"""CanDo 可为 · 外部工具服务示例（协议 cando-tools/1）

只用 Python 标准库，不用安装任何东西：
    python example_server.py            # 默认监听 http://127.0.0.1:8765
    python example_server.py --port 9000

然后在 CanDo 可为「AI 助手 → 能力中心」里点“接入工具服务”：
    短名 fig，地址 http://127.0.0.1:8765

怎么加自己的工具：写一个函数，在上面加 @tool(...)，重启服务。CanDo 那边自动发现，不用改任何东西。
  · 参数用 JSON Schema 描述；值是文件路径的参数写 "format": "cando-file"，
    函数收到的是 InFile（文件名 + 内容），不是路径 —— 工具服务碰不到用户的磁盘，这是有意的。
  · 返回 Result(text, files=[...])。生成新文件用 OutFile(name, data)；
    交回某个输入文件的新版本用 OutFile(name, data, replaces="参数名")。
    文件都由 CanDo 交给用户确认后才保存，保存前会备份，可撤销。
  · 出错直接 raise ToolError("给人看的中文说明")。

完整约定见 docs/外部工具接口.md。
"""

import argparse
import base64
import json
import re
import traceback
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PROTOCOL = "cando-tools/1"
SERVICE = {"name": "示例工具服务", "version": "0.1"}
MAX_BODY = 120 * 1024 * 1024  # CanDo 每个文件最多 20 MB、一次最多 5 个，base64 后约 134 MB 以内

# ---------------------------------------------------------------- 小框架（一般不用改）


class ToolError(Exception):
    """工具出错时抛出，说明会原样显示给用户。"""


class InFile:
    def __init__(self, name: str, data: bytes):
        self.name, self.data = name, data

    def text(self, encoding="utf-8") -> str:
        return self.data.decode(encoding, errors="replace")


class OutFile:
    def __init__(self, name: str, data, replaces: str = ""):
        self.name = name
        self.data = data.encode("utf-8") if isinstance(data, str) else data
        self.replaces = replaces


class Result:
    def __init__(self, text: str = "", files=None):
        self.text, self.files = text, files or []


_TOOLS = {}


def tool(name, title, description="", params=None, required=None, makes_files=False, timeout=60, example=""):
    """把函数登记成工具。name 只能用小写字母、数字、下划线。"""
    assert re.fullmatch(r"[a-z][a-z0-9_]{0,39}", name), "工具名不合规：" + name

    def wrap(fn):
        _TOOLS[name] = {
            "fn": fn,
            "spec": {
                "name": name,
                "title": title,
                "description": description,
                "example": example,  # 给用户看的一句示例，显示在 CanDo 的“能力中心”里，点“用它”会填进输入框
                "params": {"type": "object", "properties": params or {}, "required": required or []},
                "makes_files": makes_files,
                "timeout": timeout,
            },
        }
        return fn

    return wrap


def _call(name, args):
    t = _TOOLS.get(name)
    if not t:
        raise ToolError("没有这个工具：" + name)
    props = t["spec"]["params"]["properties"]
    kwargs = {}
    for k, v in args.items():
        if k not in props:
            continue
        if props[k].get("format") == "cando-file" and isinstance(v, dict):
            v = InFile(v.get("name", ""), base64.b64decode(v.get("base64", "")))
        kwargs[k] = v
    r = t["fn"](**kwargs)
    if isinstance(r, str):
        r = Result(r)
    return {
        "ok": True,
        "text": r.text,
        "files": [
            {"name": f.name, "base64": base64.b64encode(f.data).decode(), **({"replaces": f.replaces} if f.replaces else {})}
            for f in r.files
        ],
    }


class _Handler(BaseHTTPRequestHandler):
    def _send(self, code, obj):
        body = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.rstrip("/") == "/tools":
            self._send(200, {"protocol": PROTOCOL, "service": SERVICE, "tools": [t["spec"] for t in _TOOLS.values()]})
        else:
            self._send(404, {"ok": False, "error": "没有这个地址"})

    def do_POST(self):
        if not self.path.startswith("/tools/"):
            return self._send(404, {"ok": False, "error": "没有这个地址"})
        n = int(self.headers.get("Content-Length") or 0)
        if n > MAX_BODY:
            return self._send(413, {"ok": False, "error": "请求太大"})
        try:
            body = json.loads(self.rfile.read(n) or b"{}")
            self._send(200, _call(self.path[len("/tools/"):], body.get("args") or {}))
        except ToolError as e:
            self._send(200, {"ok": False, "error": str(e)})
        except Exception as e:  # 程序错误：终端里打印详细信息，给用户一句话
            traceback.print_exc()
            self._send(200, {"ok": False, "error": "工具内部出错：" + type(e).__name__ + " " + str(e)[:200]})

    def log_message(self, fmt, *a):
        print("[%s] %s" % (self.log_date_time_string(), fmt % a))


def serve(port=8765):
    # 只监听本机，别的电脑连不上（CanDo 也只接受本机地址）
    srv = ThreadingHTTPServer(("127.0.0.1", port), _Handler)
    print("外部工具服务已启动：http://127.0.0.1:%d  （%d 个工具：%s）" % (port, len(_TOOLS), "、".join(_TOOLS)))
    srv.serve_forever()


# ---------------------------------------------------------------- 示例工具（照着写自己的）


@tool(
    "bar_chart",
    "画柱状图",
    "把一组数字画成 SVG 柱状图（矢量图，可直接插进 Word 或 LaTeX）",
    params={
        "values": {"type": "array", "description": "数值列表，例如 [3, 5, 2]"},
        "labels": {"type": "array", "description": "每根柱子的名字，数量和 values 一样"},
        "title": {"type": "string", "description": "图标题"},
        "filename": {"type": "string", "description": "文件名，默认 chart.svg"},
    },
    required=["values"],
    makes_files=True,
    example="把 3、5、2 画成柱状图，三根柱子叫甲、乙、丙，标题“三组对比”",
)
def bar_chart(values, labels=None, title="", filename="chart.svg"):
    try:
        vals = [float(v) for v in values]
    except (TypeError, ValueError):
        raise ToolError("values 必须是数字列表")
    if not vals or len(vals) > 50:
        raise ToolError("values 要有 1 到 50 个数")
    labels = [str(x) for x in (labels or [])] + [""] * len(vals)
    w, h, pad, top = 60 * len(vals) + 80, 320, 40, 40 if title else 20
    mx = max(max(vals), 0) or 1
    esc = lambda s: s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" font-family="SimSun, serif" font-size="12">']
    if title:
        parts.append(f'<text x="{w / 2}" y="22" text-anchor="middle" font-size="14">{esc(title)}</text>')
    base = h - pad
    parts.append(f'<line x1="{pad}" y1="{base}" x2="{w - 20}" y2="{base}" stroke="#000"/>')
    for i, v in enumerate(vals):
        bh = max(v, 0) / mx * (base - top)
        x = pad + 10 + i * 60
        parts.append(f'<rect x="{x}" y="{base - bh:.1f}" width="36" height="{bh:.1f}" fill="#4a5bd4"/>')
        parts.append(f'<text x="{x + 18}" y="{base - bh - 4:.1f}" text-anchor="middle">{v:g}</text>')
        parts.append(f'<text x="{x + 18}" y="{base + 16}" text-anchor="middle">{esc(labels[i])}</text>')
    parts.append("</svg>")
    name = filename if filename.endswith(".svg") else filename + ".svg"
    return Result(f"画好了 {len(vals)} 根柱子的柱状图 {name}", [OutFile(name, "\n".join(parts))])


@tool(
    "tex_outline",
    "LaTeX 提纲",
    "列出 .tex 文件的章节结构、图表和引用数量",
    params={"tex": {"type": "string", "format": "cando-file", "description": ".tex 文件"}},
    required=["tex"],
    example="列出 main.tex 的章节结构，看看有几个图、几个表",
)
def tex_outline(tex: InFile):
    src = tex.text()
    lines = []
    for m in re.finditer(r"\\(chapter|section|subsection|subsubsection)\*?\{([^}]*)\}", src):
        indent = {"chapter": 0, "section": 1, "subsection": 2, "subsubsection": 3}[m.group(1)]
        lines.append("  " * indent + m.group(2))
    stats = "图 %d 个，表 %d 个，引用 %d 处" % (
        len(re.findall(r"\\begin\{figure", src)),
        len(re.findall(r"\\begin\{table", src)),
        len(re.findall(r"\\cite\w*\{", src)),
    )
    return "%s 的结构：\n%s\n%s" % (tex.name, "\n".join(lines) or "（没有找到章节命令）", stats)


@tool(
    "tex_insert_figure",
    "在 LaTeX 里插图",
    "在 .tex 的指定行后插入一个 figure 环境（交回修改后的 .tex，由用户确认）",
    params={
        "tex": {"type": "string", "format": "cando-file", "description": "要修改的 .tex 文件"},
        "image": {"type": "string", "description": "图片文件名，例如 chart.svg 或 fig1.png"},
        "caption": {"type": "string", "description": "图题"},
        "after_line": {"type": "integer", "description": "插在第几行之后"},
        "label": {"type": "string", "description": "引用标签，例如 fig:chart"},
    },
    required=["tex", "image", "caption", "after_line"],
    example="在 main.tex 的“实验结果”一节后面插入 chart.svg，图题“三组对比”",
)
def tex_insert_figure(tex: InFile, image, caption, after_line, label=""):
    lines = tex.text().split("\n")
    n = int(after_line)
    if not 0 <= n <= len(lines):
        raise ToolError("after_line 超出范围（文件共 %d 行）" % len(lines))
    label = label or "fig:" + re.sub(r"\W+", "_", image.rsplit(".", 1)[0])
    block = [
        r"\begin{figure}[htbp]",
        r"  \centering",
        r"  \includegraphics[width=0.8\textwidth]{%s}" % image,
        r"  \caption{%s}" % caption,
        r"  \label{%s}" % label,
        r"\end{figure}",
    ]
    new = "\n".join(lines[:n] + block + lines[n:])
    return Result("已在第 %d 行后插入图“%s”，标签 %s" % (n, caption, label), [OutFile(tex.name, new, replaces="tex")])


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=8765)
    serve(ap.parse_args().port)
