// LaTeX → PDF：用这台电脑上的 TeX Live / MiKTeX 编译，在页面里预览、下载；出错时指出行号和原因。
import { $, esc, api, apiBg, toast, actions } from "/core.js";

let engine = null;
export async function texEngine(refresh) {
  if (!engine || refresh) engine = await api("GET", "/api/latex/engine" + (refresh ? "?refresh=1" : ""));
  return engine;
}

function installHTML(d) {
  const h = d.help || {};
  return `<div class="msg warnbox"><b>这台电脑还没有检测到 LaTeX。</b>导出 PDF 需要一个 LaTeX 编译器（只需装一次）：
    <ol style="margin:6px 0 6px 18px;padding:0">
      <li><b>最省事：便携版 Tectonic</b>（约 20 MB，不用安装；第一次编译会自动下载需要的宏包，需要联网）${tectonicBtn()}</li>
      <li><b>TeX Live（推荐）</b>：从 <a href="${esc(h.texlive)}" target="_blank" rel="noopener">清华镜像</a> 下载 texlive.iso，双击打开后运行 install-tl-windows.bat，一路默认，约 30–60 分钟。</li>
      <li><b>MiKTeX（体积小）</b>：从 <a href="${esc(h.miktex)}" target="_blank" rel="noopener">miktex.org</a> 下载安装，编译时缺少的宏包会自动下载。</li>
    </ol>装好后点 <button class="sm" data-act="texRedetect">重新检测</button></div>`;
}

// exportPDF 把 source 编译成 PDF，结果显示在 box 里。kind：formula（公式片段）/ tikz（图）/ other（段落、表格或完整文档）
export async function exportPDF(box, source, kind, name) {
  box.dataset.src = source; box.dataset.kind = kind; box.dataset.name = name || "";
  let d;
  try { d = await texEngine(); } catch (e) { box.innerHTML = `<div class="msg">${esc(e.message)}</div>`; return; }
  if (!d.local) { box.innerHTML = `<div class="msg">导出 PDF 要用运行工作台那台电脑上的 LaTeX，只能在那台电脑上操作。可以先复制代码。</div>`; return; }
  if (!d.engine.found) { box.innerHTML = installHTML(d); return; }
  box.innerHTML = `<div class="muted">正在用 ${esc(d.engine.dist)} 编译，第一次可能要十几秒${d.engine.dist === "MiKTeX" ? "（MiKTeX 可能在下载缺少的宏包）" : ""}…</div>`;
  let r;
  try { r = await api("POST", "/api/latex/compile", { source, kind, name }); } catch (e) { box.innerHTML = `<div class="msg">${esc(e.message)}</div>`; return; }
  const res = r.result;
  const issues = (list, cls) => list.map((x) => `<div class="${cls}">${x.line ? `<b>第 ${x.line} 行</b>${x.file && x.file !== "main.tex" ? `（${esc(x.file)}）` : ""}：` : ""}${esc(x.msg)}${x.hint ? `<div class="muted">→ ${esc(x.hint)}</div>` : ""}</div>`).join("");
  const docLink = `<details><summary class="muted">查看实际编译的完整文档（自动补上了导言区）</summary><pre class="code">${esc(r.document)}</pre></details>`;
  if (!res.ok || !res.pdf_id) {
    box.innerHTML = `<div class="msg"><b>编译没有成功</b>（${esc(res.engine)}，${res.seconds.toFixed(1)} 秒）${issues(res.errors || [], "texerr")}</div>
      ${docLink}${res.log_tail ? `<details><summary class="muted">编译日志末尾</summary><pre class="code">${esc(res.log_tail)}</pre></details>` : ""}
      <p class="muted">行号是上面“完整文档”里的行号。改好代码后再点一次导出。</p>`;
    return;
  }
  const url = `/api/latex/pdf/${encodeURIComponent(res.pdf_id)}`;
  box.innerHTML = `<div class="row"><span class="tag ok">已生成 PDF</span><span class="muted">${esc(res.engine)} · ${res.pages || "?"} 页 · ${res.seconds.toFixed(1)} 秒</span><span class="sp"></span>
      <a class="btnlike pri" href="${url}?download=1">下载 PDF</a><a class="btnlike" href="${url}" target="_blank" rel="noopener">新窗口打开</a></div>
    ${(res.warnings || []).length ? `<div class="msg warnbox"><b>提示：</b>${issues(res.warnings, "")}</div>` : ""}
    <iframe class="pdfview" src="${url}" title="PDF 预览"></iframe>${docLink}
    <p class="muted">PDF 在工作台里保存 30 分钟，请及时下载。</p>`;
}

// 一键下载便携版 Tectonic（从官方 GitHub 发布页下载）
export function tectonicBtn() {
  return ` <button class="sm pri" data-act="texTectonic">一键下载</button><span class="muted texTecState"></span>`;
}
actions.texTectonic = async (el) => {
  const st = el.parentElement.querySelector(".texTecState");
  el.disabled = true;
  if (st) st.textContent = " 正在从 GitHub 下载（约 20 MB，网络慢时要几分钟，可以离开本页）…";
  try {
    const r = await apiBg("POST", "/api/latex/tectonic", {}, { title: "Tectonic", link: location.hash });
    engine = null;
    toast("已下载 Tectonic " + (r.version || "") + "，可以编译了");
    if (st) st.textContent = " 已下载，放在：" + r.path;
    const box = el.closest("[data-src]");
    if (box) exportPDF(box, box.dataset.src, box.dataset.kind, box.dataset.name);
    else if (actions.agTexRefresh) actions.agTexRefresh();
  } catch (e) {
    el.disabled = false;
    if (st) st.textContent = "";
    throw e;
  }
};

actions.texRedetect = async (el) => {
  const box = el.closest("[data-src]");
  el.disabled = true;
  const d = await texEngine(true);
  if (!d.engine.found) { el.disabled = false; toast("还是没有检测到 LaTeX。装好后可能需要重启工作台", true); return; }
  toast("已检测到 " + d.engine.dist);
  if (box) exportPDF(box, box.dataset.src, box.dataset.kind, box.dataset.name);
};
