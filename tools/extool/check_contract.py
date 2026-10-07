"""按接口契约检查一个外部工具服务（协议 cando-tools/1）。只用 Python 标准库。

用法：
    python check_contract.py http://127.0.0.1:8765
    python check_contract.py http://127.0.0.1:8765 --call bar_chart --args "{\"values\": [3, 5, 2]}"
    python check_contract.py http://127.0.0.1:8765 --call tex_figure --file tex=main.tex --file image=fig.png

它做三件事：
  1. GET /tools：工具清单符不符合契约（docs/cando-tools.openapi.json）。
  2. POST 一个不存在的工具：出错时的返回格式对不对（不会真的运行你的任何工具）。
  3. 给了 --call 才做：真的调用一次这个工具，检查返回的格式和交回的文件。

结果分两种：“错误”是 CanDo 会拒绝或忽略的；“提醒”是 CanDo 会截短或改掉的。没有错误时退出码为 0。
"""

import argparse
import base64
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC_PLACES = [os.path.join(HERE, "cando-tools.openapi.json"), os.path.join(HERE, "..", "..", "docs", "cando-tools.openapi.json")]


def load_spec(path=""):
    for p in ([path] if path else SPEC_PLACES):
        if os.path.isfile(p):
            with open(p, encoding="utf-8") as f:
                return json.load(f)
    sys.exit("找不到契约文件 cando-tools.openapi.json（放在本脚本旁边，或用 --spec 指定）")


# ---------------------------------------------------------------- 一个够用的 JSON Schema 检查器（只支持契约里用到的写法）

class Checker:
    def __init__(self, spec):
        self.spec = spec

    def ref(self, schema):
        while "$ref" in schema:
            node = self.spec
            for part in schema["$ref"].lstrip("#/").split("/"):
                node = node[part]
            schema = node
        return schema

    def check(self, value, schema, path="", level=None):
        """返回 [(级别, 位置, 说明)]。级别来自 x-cando-enforcement：reject / ignore 算错误，truncate / clamp 算提醒。"""
        schema = self.ref(schema)
        level = schema.get("x-cando-enforcement", level)
        kind = "提醒" if level in ("truncate", "clamp") else "错误"
        out = []

        def bad(msg):
            out.append((kind, path or "（整体）", msg))

        if "oneOf" in schema:
            results = [self.check(value, s, path, level) for s in schema["oneOf"]]
            ok = [r for r in results if not any(k == "错误" for k, _, _ in r)]
            if len(ok) == 1:
                return ok[0]
            if not ok:
                best = min(results, key=len)
                return best or [("错误", path or "（整体）", "不符合任何一种约定的格式")]
            return [("错误", path or "（整体）", "同时符合多种格式，无法判断")]
        t = schema.get("type")
        types = {"object": dict, "array": list, "string": str, "boolean": bool, "integer": int, "number": (int, float)}
        if t:
            good = isinstance(value, types[t]) and not (t in ("integer", "number") and isinstance(value, bool))
            if not good:      # 类型不对一定是错误：CanDo 解析不了
                out.append(("错误", path or "（整体）", "应该是 %s，实际是 %s" % (t, type(value).__name__)))
                return out
        if "enum" in schema and value not in schema["enum"]:
            bad("应该是 %s，实际是 %s" % (" 或 ".join(json.dumps(e, ensure_ascii=False) for e in schema["enum"]), json.dumps(value, ensure_ascii=False)))
        if isinstance(value, str):
            if "maxLength" in schema and len(value) > schema["maxLength"]:
                bad("太长：%d 个字符，最多 %d" % (len(value), schema["maxLength"]))
            if "minLength" in schema and len(value) < schema["minLength"]:
                bad("不能为空")
            if "pattern" in schema and not re.search(schema["pattern"], value):
                bad("格式不对：%r 不符合 %s" % (value[:60], schema["pattern"]))
            if schema.get("format") == "byte":
                try:
                    base64.b64decode(value, validate=True)
                except Exception:
                    bad("不是有效的 base64")
        if isinstance(value, (int, float)) and not isinstance(value, bool):
            if "minimum" in schema and value < schema["minimum"]:
                bad("太小：%s，最小 %s" % (value, schema["minimum"]))
            if "maximum" in schema and value > schema["maximum"]:
                bad("太大：%s，最大 %s" % (value, schema["maximum"]))
        if isinstance(value, list):
            if "maxItems" in schema and len(value) > schema["maxItems"]:
                bad("太多：%d 个，最多 %d" % (len(value), schema["maxItems"]))
            if "items" in schema:
                for i, v in enumerate(value):
                    out += self.check(v, schema["items"], "%s[%d]" % (path, i), level)
        if isinstance(value, dict):
            for k in schema.get("required", []):
                if k not in value:
                    out.append(("错误", path or "（整体）", "缺少必需的字段 %s" % k))
            if "maxProperties" in schema and len(value) > schema["maxProperties"]:
                bad("太多：%d 项，最多 %d" % (len(value), schema["maxProperties"]))
            props = schema.get("properties", {})
            extra = schema.get("additionalProperties")
            for k, v in value.items():
                sub = "%s.%s" % (path, k) if path else k
                if k in props:
                    out += self.check(v, props[k], sub, level)
                elif isinstance(extra, dict) and extra:
                    out += self.check(v, extra, sub, level)
        return out


# ---------------------------------------------------------------- 访问服务

def http(method, url, body=None, timeout=10):
    data = json.dumps(body, ensure_ascii=False).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={"Content-Type": "application/json"} if data else {})
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))      # 和 CanDo 一样，不走代理
    t0 = time.time()
    try:
        with opener.open(req, timeout=timeout) as r:
            return r.status, r.read(), time.time() - t0
    except urllib.error.HTTPError as e:
        return e.code, e.read(), time.time() - t0


def check_manifest(base, spec, chk):
    lim = spec["x-cando-limits"]
    out = []
    try:
        status, raw, secs = http("GET", base + "/tools", timeout=lim["manifest_timeout_seconds"])
    except Exception as e:
        return None, [("错误", "GET /tools", "连不上，或 %d 秒内没有返回（%s）" % (lim["manifest_timeout_seconds"], type(e).__name__))]
    if status != 200:
        return None, [("错误", "GET /tools", "应该返回 200，实际是 %d" % status)]
    if len(raw) > lim["manifest_max_bytes"]:
        out.append(("错误", "GET /tools", "工具清单 %d 字节，超过 1 MB" % len(raw)))
    try:
        m = json.loads(raw.decode("utf-8"))
    except Exception:
        return None, out + [("错误", "GET /tools", "返回的不是 UTF-8 编码的 JSON")]
    out += chk.check(m, {"$ref": "#/components/schemas/Manifest"})
    if not isinstance(m, dict) or not isinstance(m.get("tools"), list):
        return m, out
    name_rx = spec["components"]["schemas"]["Tool"]["properties"]["name"]["pattern"]
    seen = set()
    for i, t in enumerate(m["tools"]):
        if not isinstance(t, dict):
            continue
        where = "tools[%d]（%s）" % (i, t.get("name", "?"))
        if t.get("name") in seen:
            out.append(("错误", where, "工具名重复，后面这个会被忽略"))
        seen.add(t.get("name"))
        if not str(t.get("description", "")).strip():
            out.append(("提醒", where, "没有 description：智能体不知道什么时候该用它"))
        params = t.get("params") if isinstance(t.get("params"), dict) else {}
        props = params.get("properties") if isinstance(params.get("properties"), dict) else {}
        files = 0
        for k, p in props.items():
            if not re.search(name_rx, k):
                out.append(("错误", where + ".params." + k, "参数名不合规，这个参数会被忽略"))
            if k in lim["reserved_param_names"]:
                out.append(("错误", where + ".params." + k, "%s 是 CanDo 保留的名字，这个参数会被忽略" % k))
            if isinstance(p, dict):
                if p.get("format") == "cando-file":
                    files += 1
                for e in (p.get("enum") or []):
                    if len(str(e)) > 20:
                        out.append(("提醒", where + ".params." + k, "可选值 %r 超过 20 个字符，会被截短" % str(e)[:30]))
        for k in (params.get("required") or []):
            if k not in props:
                out.append(("错误", where + ".params.required", "必填参数 %s 没有在 properties 里声明" % k))
        req_files = [k for k in (params.get("required") or []) if isinstance(props.get(k), dict) and props[k].get("format") == "cando-file"]
        if len(req_files) > lim["input_files_per_call"]:
            out.append(("错误", where, "必填的文件参数有 %d 个，而一次调用最多发 %d 个文件" % (len(req_files), lim["input_files_per_call"])))
    return m, out


def check_failure_shape(base, chk):
    name = "contract_probe_no_such_tool"
    try:
        status, raw, _ = http("POST", base + "/tools/" + name, {"args": {}, "call_id": "contract-check"})
    except Exception as e:
        return [("错误", "POST /tools/" + name, "连不上（%s）" % type(e).__name__)]
    try:
        r = json.loads(raw.decode("utf-8"))
    except Exception:
        return [("错误", "POST /tools/<不存在的工具>", "出错时返回的不是 JSON（HTTP %d）。用户只能看到“没有说明原因”" % status)]
    out = chk.check(r, {"$ref": "#/components/schemas/CallFailure"}, "出错时的返回")
    if isinstance(r, dict) and r.get("ok") is False and not str(r.get("error", "")).strip():
        out.append(("提醒", "出错时的返回", "没有 error 字段：用户只能看到“没有说明原因”"))
    return out


def check_call(base, spec, chk, manifest, name, args, files):
    lim = spec["x-cando-limits"]
    tool = next((t for t in (manifest or {}).get("tools", []) if isinstance(t, dict) and t.get("name") == name), None)
    if not tool:
        return [("错误", "--call " + name, "工具清单里没有这个工具")], None
    out = []
    props = ((tool.get("params") or {}).get("properties") or {})
    send = dict(args)
    sent_files = []
    for k, path in files.items():
        with open(path, "rb") as f:
            data = f.read()
        if len(data) > lim["input_file_max_bytes"]:
            out.append(("错误", "--file " + k, "文件超过 20 MB，CanDo 不会发送"))
        send[k] = {"name": os.path.basename(path), "base64": base64.b64encode(data).decode()}
        sent_files.append(k)
    for k in send:
        if k not in props:
            out.append(("错误", "--call " + name, "参数 %s 没有在清单里声明，CanDo 不会发送它" % k))
    timeout = min(max(int(tool.get("timeout") or 0) or lim["timeout_default_seconds"], 1), lim["timeout_max_seconds"])
    try:
        status, raw, secs = http("POST", base + "/tools/" + name, {"args": send, "call_id": "contract-check"}, timeout=timeout)
    except Exception as e:
        return out + [("错误", "POST /tools/" + name, "连不上，或超过声明的 %d 秒没有返回（%s）" % (timeout, type(e).__name__))], None
    if len(raw) > lim["response_max_bytes"]:
        out.append(("错误", "POST /tools/" + name, "返回内容超过 80 MB"))
    try:
        r = json.loads(raw.decode("utf-8"))
    except Exception:
        return out + [("错误", "POST /tools/" + name, "返回的不是 JSON（HTTP %d）" % status)], None
    if not isinstance(r, dict) or not isinstance(r.get("ok"), bool):
        return out + [("错误", "返回.ok", "必须有 ok 字段，值是 true 或 false")], None
    out += chk.check(r, {"$ref": "#/components/schemas/" + ("CallSuccess" if r["ok"] else "CallFailure")}, "返回")
    if status != 200:
        out.append(("提醒", "POST /tools/" + name, "HTTP 状态码是 %d：CanDo 会按失败处理" % status))
    if isinstance(r, dict) and r.get("ok") is True:
        total = 0
        for i, f in enumerate(r.get("files") or []):
            if not isinstance(f, dict):
                continue
            where = "返回.files[%d]（%s）" % (i, f.get("name", "?"))
            base_name = str(f.get("name", "")).replace("\\", "/").rsplit("/", 1)[-1]
            if base_name.startswith(".") or not base_name:
                out.append(("错误", where, "文件名为空或以 . 开头，CanDo 不保存"))
            if os.path.splitext(base_name)[1].lower() in lim["rejected_output_extensions"]:
                out.append(("错误", where, "可执行文件或脚本，CanDo 不保存，整次调用按失败处理"))
            if f.get("replaces") and f["replaces"] not in sent_files:
                out.append(("错误", where, "replaces 写的“%s”不是这次发送的文件参数" % f["replaces"]))
            try:
                total += len(base64.b64decode(f.get("base64", ""), validate=True))
            except Exception:
                pass
        if total > lim["output_files_total_max_bytes"]:
            out.append(("错误", "返回.files", "交回的文件合计 %.1f MB，超过 50 MB" % (total / 1048576)))
        if (r.get("files") or []) and not tool.get("makes_files") and not any(f.get("replaces") for f in r["files"] if isinstance(f, dict)):
            out.append(("提醒", "返回.files", "交回了新文件，但清单里 makes_files 不是 true：智能体不知道可以用 save_to 指定保存位置"))
    return out, r


def main(argv=None):
    ap = argparse.ArgumentParser(description="按接口契约检查一个外部工具服务（cando-tools/1）")
    ap.add_argument("url", help="服务地址，例如 http://127.0.0.1:8765")
    ap.add_argument("--spec", default="", help="契约文件，默认找 cando-tools.openapi.json")
    ap.add_argument("--call", default="", help="真的调用一次这个工具")
    ap.add_argument("--args", default="{}", help="调用时的参数（JSON）")
    ap.add_argument("--file", action="append", default=[], metavar="参数名=文件路径", help="调用时的文件参数，可以给多次")
    a = ap.parse_args(argv)
    base = a.url.rstrip("/")
    spec = load_spec(a.spec)
    chk = Checker(spec)
    print("契约：%s %s（%s）" % (spec["info"]["title"], spec["info"]["version"], spec["x-cando-protocol"]))
    print("服务：%s\n" % base)

    manifest, problems = check_manifest(base, spec, chk)
    if manifest and isinstance(manifest.get("tools"), list):
        print("1. 工具清单：%d 个工具（%s）" % (len(manifest["tools"]), "、".join(str(t.get("name")) for t in manifest["tools"] if isinstance(t, dict))))
    else:
        print("1. 工具清单：读不到")
    if manifest is not None:
        p2 = check_failure_shape(base, chk)
        print("2. 出错时的返回格式：%s" % ("符合" if not p2 else "有问题"))
        problems += p2
    if a.call and manifest is not None:
        try:
            args = json.loads(a.args)
            files = dict(x.split("=", 1) for x in a.file)
        except Exception:
            sys.exit("--args 要是 JSON；--file 的写法是 参数名=文件路径")
        p3, r = check_call(base, spec, chk, manifest, a.call, args, files)
        if r is not None:
            if r.get("ok") is True:
                print("3. 调用 %s：成功。text %d 字，交回 %d 个文件" % (a.call, len(str(r.get("text", ""))), len(r.get("files") or [])))
            else:
                print("3. 调用 %s：服务返回失败：%s" % (a.call, str(r.get("error", ""))[:200]))
        problems += p3

    errors = [p for p in problems if p[0] == "错误"]
    notes = [p for p in problems if p[0] == "提醒"]
    print()
    for kind, where, msg in errors + notes:
        print("[%s] %s：%s" % (kind, where, msg))
    print("结果：%d 处错误，%d 条提醒。%s" % (len(errors), len(notes), "符合契约。" if not errors else "请先改掉错误。"))
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
