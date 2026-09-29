// AI 助手：图片转 LaTeX、手绘转图（所有设备可用）、本机智能体（只在运行工作台的电脑上可用）、操作记录
import { $, $$, esc, api, toast, modal, closeModal, actions, fmtTime } from "/core.js";
import { exportPDF, tectonicBtn } from "/texpdf.js";
import { renderSketch } from "/sketch.js";

const A = { side: "changes", pending: null, tab: "img", st: null, img: "", sid: "", total: 0, running: false, timer: 0, attach: [], changes: [], folders: [], pid: "" };
const TASKS = [["auto", "自动判断"], ["formula", "公式"], ["table", "表格"], ["text", "含公式的段落（论文截图）"], ["hand", "手写笔记"], ["marks", "批注修改（红笔圈改）"]];

// ---------------- KaTeX（按需加载） ----------------
let katexP = null;
export function loadKatex() {
  if (window.katex) return Promise.resolve(window.katex);
  if (!katexP) {
    katexP = new Promise((ok, fail) => {
      const l = document.createElement("link");
      l.rel = "stylesheet"; l.href = "/lib/katex/katex.min.css";
      document.head.appendChild(l);
      const s = document.createElement("script");
      s.src = "/lib/katex/katex.min.js";
      s.onload = () => ok(window.katex);
      s.onerror = () => { katexP = null; fail(new Error("公式预览组件加载失败")); };
      document.head.appendChild(s);
    });
  }
  return katexP;
}

export function renderMath(el, expr, display) {
  const span = document.createElement(display ? "div" : "span");
  if (display) span.className = "mathblock";
  window.katex.render(expr.replace(/\\label\{[^}]*\}/g, "").replace(/\\(nonumber|notag)\b/g, ""), span, { displayMode: display, throwOnError: false, strict: false, trust: false });
  el.appendChild(span);
}

const reMath = /\\begin\{(equation|align|gather|multline|eqnarray)(\*?)\}([\s\S]*?)\\end\{\1\2\}|\\\[([\s\S]*?)\\\]|\$\$([\s\S]*?)\$\$|\\\(([\s\S]*?)\\\)|\$((?:\\\$|[^$])+?)\$/g;

async function preview(el, latex, task) {
  el.innerHTML = "";
  if (task === "marks") return;
  try { await loadKatex(); } catch (e) { el.innerHTML = `<p class="muted">${esc(e.message)}</p>`; return; }
  const src = latex.trim();
  if (task === "formula" || (!/\\begin\{(tabular|table|itemize|enumerate)|\\section|\\par\b/.test(src) && !/[\u4e00-\u9fff]/.test(src.replace(/\\text\{[^}]*\}/g, "")) && !src.includes("$") && task !== "text" && task !== "table")) {
    let e = src.replace(/^\\\[|\\\]$/g, "").replace(/^\$\$|\$\$$/g, "");
    const m = e.match(/^\\begin\{(equation|align|gather|multline)(\*?)\}([\s\S]*)\\end\{\1\2\}$/);
    if (m) e = m[1] === "equation" ? m[3] : `\\begin{${m[1]}ed}${m[3]}\\end{${m[1]}ed}`.replace("multlineed", "gathered");
    renderMath(el, e, true);
    return;
  }
  if (/\\begin\{tabular/.test(src)) {
    el.innerHTML = `<p class="muted">表格需要用 LaTeX 编译后查看，这里只预览其中的公式：</p>`;
  }
  // 段落：文字原样显示，公式渲染
  const box = document.createElement("div");
  box.className = "mathtext";
  let last = 0;
  src.replace(reMath, (all, env, star, envBody, br, dd, par, inl, off) => {
    if (off > last) box.appendChild(document.createTextNode(src.slice(last, off).replace(/\\(textbf|textit|emph|section\*?|subsection\*?)\{([^}]*)\}/g, "$2")));
    if (env) renderMath(box, env === "equation" ? envBody : `\\begin{${env === "multline" ? "gather" : env}ed}${envBody}\\end{${env === "multline" ? "gather" : env}ed}`.replace("eqnarrayed", "aligned"), true);
    else if (br !== undefined || dd !== undefined) renderMath(box, br ?? dd, true);
    else renderMath(box, par ?? inl, false);
    last = off + all.length;
    return all;
  });
  if (last < src.length) box.appendChild(document.createTextNode(src.slice(last)));
  el.appendChild(box);
}

// ---------------- 图片读取与压缩 ----------------
async function fileToDataURL(file) {
  if (!file.type.startsWith("image/")) throw new Error("请选择图片文件");
  const url = URL.createObjectURL(file);
  try {
    const img = await new Promise((ok, fail) => { const i = new Image(); i.onload = () => ok(i); i.onerror = () => fail(new Error("无法读取这张图片")); i.src = url; });
    const maxSide = 2000;
    const k = Math.min(1, maxSide / Math.max(img.naturalWidth, img.naturalHeight));
    const w = Math.round(img.naturalWidth * k), h = Math.round(img.naturalHeight * k);
    const c = document.createElement("canvas");
    c.width = w; c.height = h;
    const g = c.getContext("2d");
    g.fillStyle = "#fff"; g.fillRect(0, 0, w, h);
    g.drawImage(img, 0, 0, w, h);
    let d = file.type === "image/png" && file.size < 3e6 && k === 1 ? c.toDataURL("image/png") : c.toDataURL("image/jpeg", 0.92);
    if (d.length > 7e6) d = c.toDataURL("image/jpeg", 0.8);
    return d;
  } finally { URL.revokeObjectURL(url); }
}

function imagesFromEvent(e) {
  const items = [...(e.clipboardData || e.dataTransfer || {}).items || []];
  const files = items.filter((i) => i.kind === "file" && i.type.startsWith("image/")).map((i) => i.getAsFile());
  if (!files.length && e.dataTransfer) return [...e.dataTransfer.files].filter((f) => f.type.startsWith("image/"));
  return files;
}

document.addEventListener("paste", async (e) => {
  if (!location.hash.startsWith("#/assistant")) return;
  const files = imagesFromEvent(e);
  if (!files.length) return;
  e.preventDefault();
  if (A.tab === "img") setImage(files[0]);
  else if (A.tab === "agent") addAttach(files);
  else if (A.tab === "sketch") { const inp = $('input[data-change="skPhoto"]'); if (inp) { const dt = new DataTransfer(); dt.items.add(files[0]); inp.files = dt.files; inp.dispatchEvent(new Event("change", { bubbles: true })); } }
});

// ---------------- 页面 ----------------
export async function pageAssistant(main, tab) {
  A.st = await api("GET", "/api/agent/status");
  A.tab = ["sketch", "agent", "logs"].includes(tab) ? tab : "img";
  if (!A.st.local && (A.tab === "agent" || A.tab === "logs")) A.tab = "img";
  main.innerHTML = `<h2>AI 助手</h2>
    <div class="tabs">
      <button class="${A.tab === "img" ? "on" : ""}" data-act="asTab" data-t="img">图片转 LaTeX</button>
      <button class="${A.tab === "sketch" ? "on" : ""}" data-act="asTab" data-t="sketch">手绘转图</button>
      <button class="${A.tab === "agent" ? "on" : ""}" data-act="asTab" data-t="agent">本机智能体</button>
      ${A.st.local ? `<button class="${A.tab === "logs" ? "on" : ""}" data-act="asTab" data-t="logs">操作记录</button>` : ""}
    </div><div id="asBody"></div>`;
  renderTab();
}
actions.asTab = (el) => { A.tab = el.dataset.t; history.replaceState(null, "", "#/assistant/" + A.tab); $$(".tabs:not(.sub) button").forEach((b) => b.classList.toggle("on", b === el)); renderTab(); };

// 把一段任务（和图片）交给本机智能体
function goAgent(draft, attach) {
  A.attach = attach || [];
  A.draft = draft;
  A.sid = "";
  A.tab = "agent";
  history.replaceState(null, "", "#/assistant/agent");
  $$(".tabs:not(.sub) button").forEach((b) => b.classList.toggle("on", b.dataset.t === "agent"));
  renderTab();
}

function renderTab() {
  clearTimeout(A.timer);
  const b = $("#asBody");
  if (A.tab === "img") return renderImg(b);
  if (A.tab === "sketch") return renderSketch(b, { local: A.st.local, vision: A.st.vision, visionOK: A.st.vision_configured, toAgent: (t) => goAgent(t, []) });
  if (A.tab === "logs") return renderLogs(b).catch((e) => (b.innerHTML = `<div class="msg">${esc(e.message)}</div>`));
  if (!A.st.local) {
    b.innerHTML = `<div class="card"><h3>本机智能体</h3><p>为了安全，本机智能体<b>只能在运行工作台的电脑上使用</b>：它会读写这台电脑上你授权的文件夹、运行命令，手机和其他电脑不能操作。</p>
      <p class="muted">在手机上可以用“图片转 LaTeX”和“手绘转图”：拍下公式、表格、手写笔记或草图，直接得到 LaTeX。</p></div>`;
    return;
  }
  renderAgent(b).catch((e) => (b.innerHTML = `<div class="msg">${esc(e.message)}</div>`));
}

// ---------------- 图片转 LaTeX ----------------
function renderImg(b) {
  const v = A.st.vision_configured;
  b.innerHTML = `<div class="card">
    <p class="muted" style="margin-top:0">把公式、表格、论文截图、手写笔记或打印稿上的批注拍照/截图，转成可以直接用的 LaTeX。识图模型：<b>${esc(A.st.vision)}</b>${v ? "" : `<br><span style="color:var(--bad)">还没有能看图片的模型。</span>请到 <a href="#/settings">设置 → 我的 AI 模型</a> 添加能识图的模型（如通义千问 VL、智谱 GLM-4V、豆包视觉、Claude），勾选“能看图片”并通过自检。`}</p>
    <label class="drop" id="imgDrop">${A.img ? `<img src="${A.img}" alt="待识别的图片">` : `<span><b>点这里选择图片</b>，或把截图粘贴（Ctrl+V）/ 拖到这里<br><span class="muted">手机上可以直接拍照</span></span>`}
      <input type="file" accept="image/*" data-change="imgPick" hidden></label>
    <div class="row" style="margin-top:10px">
      <select id="imgTask" style="width:auto">${TASKS.map(([k, n]) => `<option value="${k}">${n}</option>`).join("")}</select>
      <input id="imgNote" placeholder="补充要求（可选），例如：用 align 环境；表格加表题“实验结果”" style="flex:1;min-width:200px">
      <button class="pri" data-act="imgGo" ${A.img ? "" : "disabled"}>识别</button>
      ${A.img ? '<button data-act="imgClear">换一张</button>' : ""}
    </div></div>
    <div id="imgOut"></div>`;
  const drop = $("#imgDrop");
  drop.addEventListener("dragover", (e) => { e.preventDefault(); drop.classList.add("over"); });
  drop.addEventListener("dragleave", () => drop.classList.remove("over"));
  drop.addEventListener("drop", (e) => { e.preventDefault(); drop.classList.remove("over"); const f = imagesFromEvent(e); if (f.length) setImage(f[0]); });
}
async function setImage(file) {
  try { A.img = await fileToDataURL(file); renderImg($("#asBody")); } catch (e) { toast(e.message, true); }
}
actions.imgPick = (el) => { if (el.files[0]) setImage(el.files[0]); };
actions.imgClear = () => { A.img = ""; renderImg($("#asBody")); };
actions.imgGo = async (el) => {
  const task = $("#imgTask").value;
  el.disabled = true;
  el.textContent = "识别中…";
  const out = $("#imgOut");
  out.innerHTML = `<div class="card muted">正在识别，通常需要 5–30 秒…</div>`;
  try {
    const r = await api("POST", "/api/latex/from-image", { image: A.img, task, note: $("#imgNote").value });
    showLatex(out, r.latex, task, r.model, r.warnings || []);
  } catch (e) {
    out.innerHTML = `<div class="card"><div class="msg">${esc(e.message)}</div></div>`;
  } finally { el.disabled = false; el.textContent = "识别"; }
};

function showLatex(out, latex, task, model, warnings) {
  out.innerHTML = `<div class="card">
    <div class="row"><h3 style="margin:0">${task === "marks" ? "识别出的批注" : "LaTeX"}</h3><span class="muted">由 ${esc(model)} 识别，可能有误，请对照原图核对</span></div>
    <textarea id="imgLatex" class="code" spellcheck="false">${esc(latex)}</textarea>
    <div class="row" style="margin-top:8px">
      <button class="pri" data-act="imgCopy">复制</button>
      ${task !== "marks" ? '<button data-act="imgPreview">更新预览</button><button data-act="imgCheck">检查语法</button>' : ""}
      ${A.st.local && task !== "marks" ? '<button data-act="imgPDF">导出 PDF</button>' : ""}
      ${A.st.local ? '<button data-act="imgToAgent">交给智能体放进 .tex 文件</button>' : ""}
    </div>
    <div id="imgWarn">${warnHTML(warnings)}</div>
    <div id="imgPDFBox" style="margin-top:8px"></div>
    ${task !== "marks" ? '<h4>预览</h4><div id="imgPrev" class="mathprev"></div><p class="muted">预览用 KaTeX 渲染，个别命令和表格排版以实际编译为准。</p>' : ""}</div>`;
  out.dataset.task = task;
  if (task !== "marks") preview($("#imgPrev"), latex, task);
}
function warnHTML(ws) {
  return ws.length ? `<div class="msg warnbox"><b>可能的问题：</b>${ws.map((w) => `<div>${esc(w)}</div>`).join("")}</div>` : "";
}
actions.imgCopy = async () => {
  const t = $("#imgLatex").value;
  try { await navigator.clipboard.writeText(t); toast("已复制"); } catch { $("#imgLatex").select(); document.execCommand("copy"); toast("已复制"); }
};
actions.imgPreview = () => preview($("#imgPrev"), $("#imgLatex").value, $("#imgOut").dataset.task);
actions.imgCheck = async () => {
  const r = await api("POST", "/api/latex/check", { text: $("#imgLatex").value, fragment: true });
  $("#imgWarn").innerHTML = r.warnings.length ? warnHTML(r.warnings) : '<p class="muted">没有发现括号、环境或公式配对问题。</p>';
};
actions.imgPDF = () => {
  const task = $("#imgOut").dataset.task, src = $("#imgLatex").value;
  // 自动判断时：没有 $、环境、\[ 也没有中文的，按公式处理（自动加上 \[ \]）
  const formula = task === "formula" || (task === "auto" && !/\$|\\begin\{|\\\[|[\u4e00-\u9fff]/.test(src));
  exportPDF($("#imgPDFBox"), src, formula ? "formula" : "other", formula ? "formula" : "latex");
};
actions.imgToAgent = () => {
  const latex = $("#imgLatex").value;
  const task = $("#imgOut").dataset.task;
  goAgent(task === "marks"
    ? `请按图片1 上的批注修改我的论文（批注已识别如下，请核对后在对应的 .tex 文件中逐处修改）：\n${latex}\n\n文件：`
    : `请把下面这段 LaTeX（由图片1 识别）放进 .tex 文件的合适位置：\n${latex}\n\n文件和位置：`, [A.img]);
};

// ---------------- 本机智能体 ----------------
const EXAMPLES = "例如：\n· 编译 D:\\论文\\main.tex，有错误就帮我改好\n· 把“下载”文件夹里的 PDF 按年份整理到子文件夹\n· 用 Python 画出 data.csv 第二列随时间的变化，存成 PNG\n· 查一下最近关于钙钛矿电池稳定性的论文，挑 5 篇加入资料库\n· 按图片1 的红笔批注修改 chapters\\intro.tex（先附上图片）";

async function renderAgent(b) {
  const [st, sessions, projects] = await Promise.all([api("GET", "/api/agent/status"), api("GET", "/api/agent/sessions"), api("GET", "/api/projects").catch(() => [])]);
  A.st = st;
  A.folders = st.folders || [];
  if (A.sid && !sessions.find((s) => s.id === A.sid)) A.sid = "";
  if (!A.sid && sessions.length && !A.draft) A.sid = sessions[0].id;
  const plist = Array.isArray(projects) ? projects : projects.list || [];
  A.plist = plist;
  const tex = st.tex && st.tex.found ? `${esc(st.tex.dist)}` : '<a href="#" data-act="agSide" data-s="folders">未安装</a>';
  b.innerHTML = `<div class="agent">
    <div class="card agchat">
      <div class="row">
        <span class="muted">模型：<b>${esc(st.model)}</b> · 识图：<b>${esc(st.vision)}</b> · LaTeX：<b>${tex}</b></span><span class="sp"></span>
        <select data-change="agSwitch" style="width:auto;max-width:220px"><option value="">＋ 新对话</option>${sessions.map((s) => `<option value="${esc(s.id)}" ${s.id === A.sid ? "selected" : ""}>${esc(s.title || "（空对话）")} · ${fmtTime(s.updated)}</option>`).join("")}</select>
      </div>
      ${st.configured ? "" : `<div class="msg">还没有可用的 AI 模型，请先到 <a href="#/settings">设置</a> 添加。</div>`}
      <div id="agUnattended"></div>
      <div id="agSteps" class="steps"></div>
      <div id="agApprove"></div>
      <div id="agAttach" class="row"></div>
      <form data-submit="agSend">
        <textarea name="text" id="agText" placeholder="${esc(EXAMPLES)}">${esc(A.draft || "")}</textarea>
        <div class="row" style="margin-top:6px">
          <label class="btnlike">附图片<input type="file" accept="image/*" multiple data-change="agPick" hidden></label>
          <select id="agPid" style="width:auto;max-width:200px" title="检索和加入文献用哪个资料库"><option value="">论文库：我的</option>${plist.map((p) => `<option value="${esc(p.id)}" ${p.id === A.pid ? "selected" : ""}>论文库：${esc(p.name)}</option>`).join("")}</select>
          <span class="sp"></span>
          <button type="button" id="agStop" data-act="agStop" class="hide">停止</button>
          <button class="pri" type="submit" id="agGo">发送</button>
        </div>
      </form>
    </div>
    <div class="agside card">
      <div class="tabs sub">${[["changes", "待确认"], ["folders", "文件夹与权限"], ["memory", "记忆"], ["skills", "技能"], ["tasks", "定时任务"]].map(([k, n]) => `<button class="${A.side === k ? "on" : ""}" data-act="agSide" data-s="${k}">${n}<span id="agBadge-${k}"></span></button>`).join("")}</div>
      <div id="agSideBody"></div>
    </div></div>`;
  A.draft = "";
  renderAttach();
  renderSide();
  $("#agText").addEventListener("keydown", (e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); $("#agGo").click(); } });
  const drop = $(".agchat");
  drop.addEventListener("dragover", (e) => e.preventDefault());
  drop.addEventListener("drop", (e) => { const f = imagesFromEvent(e); if (f.length) { e.preventDefault(); addAttach(f); } });
  A.total = 0;
  $("#agSteps").innerHTML = A.sid ? "" : introHTML();
  if (A.sid) await poll(true);
}
function introHTML() {
  return `<div class="muted agintro"><p>告诉智能体要做什么，它会自己规划步骤：在授权文件夹里查找、阅读、修改和整理文件，运行命令和 Python 脚本，编译 LaTeX 出 PDF，读网页、查文献。</p>
    <p><b>安全：</b>改文件、移动、删除都要你在“待确认”里点应用（删除先放进回收区，可撤销）；运行命令、打开文件前会弹出确认；它只能碰你授权的文件夹。</p></div>`;
}

actions.agSide = (el) => { A.side = el.dataset.s; $$(".tabs.sub button").forEach((x) => x.classList.toggle("on", x.dataset.s === A.side)); renderSide(); };
function renderSide() {
  const box = $("#agSideBody");
  if (!box) return;
  const f = { changes: renderChanges, folders: renderFolders, memory: renderMemory, skills: renderSkills, tasks: renderTasks }[A.side] || renderChanges;
  Promise.resolve(f()).catch((e) => (box.innerHTML = `<div class="msg">${esc(e.message)}</div>`));
}

// ----- 文件夹与权限 -----
async function renderFolders() {
  const box = $("#agSideBody");
  const [pol, eng] = await Promise.all([api("GET", "/api/agent/policy"), api("GET", "/api/latex/engine")]);
  if (A.side !== "folders") return;
  A.policy = pol;
  const opt = (v, cur, label) => `<option value="${v}" ${v === cur ? "selected" : ""}>${label}</option>`;
  box.innerHTML = `<h4 style="margin-top:0">授权文件夹</h4>
    ${A.folders.length ? A.folders.map((f, i) => `<div class="row folder">
      <code title="${esc(f.path)}">${esc(f.path)}</code>${f.exists ? "" : '<span class="tag bad">不存在</span>'}<span class="sp"></span>
      <select data-change="agMode" data-i="${i}" style="width:auto"><option value="0" ${f.write ? "" : "selected"}>只读</option><option value="1" ${f.write ? "selected" : ""}>可修改</option></select>
      <button class="sm danger" data-act="agDelFolder" data-i="${i}">移除</button></div>`).join("") : '<p class="muted">还没有授权任何文件夹，智能体现在看不到你的文件。</p>'}
    <form data-submit="agAddFolder" class="row" style="margin-top:8px">
      <input name="path" required placeholder="完整路径，例如 D:\\论文\\毕业论文" style="flex:1;min-width:180px">
      <select name="write" style="width:auto"><option value="0">只读</option><option value="1">可修改</option></select>
      <button type="submit">添加</button></form>
    <p class="muted">只授权需要的文件夹；不能授权整个磁盘、系统文件夹和工作台自己的数据文件夹。命令也只能在授权文件夹里运行。</p>
    <h4>权限</h4>
    <form data-submit="agPolicy" class="polform">
      <label>运行命令 / 脚本<select name="commands">${opt("ask", pol.commands, "每次先问我")}${opt("off", pol.commands, "不允许")}</select></label>
      <label>用默认程序打开文件、网址<select name="open">${opt("ask", pol.open, "每次先问我")}${opt("auto", pol.open, "直接打开")}${opt("off", pol.open, "不允许")}</select></label>
      <label>读取网页内容<select name="web">${opt("auto", pol.web, "允许")}${opt("off", pol.web, "不允许")}</select></label>
      <label>始终允许的命令（每行一个，按程序名，如 python、git status）<textarea name="allow" rows="3" placeholder="python">${esc((pol.allow_cmds || []).join("\n"))}</textarea></label>
      <p class="muted">删除、改注册表、下载执行、关机等危险命令<b>每次都会问</b>，不能设为始终允许。</p>
      <button type="submit" class="pri sm">保存权限</button>
    </form>
    <h4>LaTeX 编译</h4>
    ${eng.engine.found ? `<p>已检测到 <b>${esc(eng.engine.dist)}</b> ${esc(eng.engine.version || "")}（${eng.engine.tectonic ? "便携版，第一次编译会自动下载宏包，需要联网" : (eng.engine.xelatex ? "XeLaTeX" : "") + (eng.engine.xelatex && eng.engine.pdflatex ? " / " : "") + (eng.engine.pdflatex ? "pdfLaTeX" : "")}）。智能体可以编译论文、根据报错自动修改。</p>`
      : `<p class="muted">没有检测到 LaTeX。最省事的是下载便携版 <b>Tectonic</b>（约 20 MB，不用安装，第一次编译自动下载宏包）：${eng.local ? tectonicBtn() : "（请在运行工作台的电脑上操作）"}</p>
        <p class="muted">需要完整环境时，从 <a href="${esc(eng.help.texlive)}" target="_blank" rel="noopener">清华镜像</a> 安装 TeX Live，或安装 <a href="${esc(eng.help.miktex)}" target="_blank" rel="noopener">MiKTeX</a>。也可以手动下载 <a href="${esc(eng.help.tectonic_page)}" target="_blank" rel="noopener">Tectonic</a>，把 tectonic.exe 放到 <code>${esc(eng.help.tectonic_dir || "")}</code>。</p><button class="sm" data-act="agTexRefresh">重新检测</button>`}`;
}
async function saveFolders(list) {
  A.folders = await api("PUT", "/api/agent/folders", { folders: list.map((f) => ({ path: f.path, write: !!f.write })) });
  renderSide();
}
actions.agAddFolder = async (f) => {
  await saveFolders([...A.folders, { path: f.path.value.trim(), write: f.write.value === "1" }]);
  toast("已授权");
};
actions.agDelFolder = (el) => saveFolders(A.folders.filter((_, i) => i !== Number(el.dataset.i)));
actions.agMode = (el) => { const l = A.folders.map((f) => ({ ...f })); l[Number(el.dataset.i)].write = el.value === "1"; return saveFolders(l); };
actions.agPolicy = async (f) => {
  await api("PUT", "/api/agent/policy", { commands: f.commands.value, open: f.open.value, web: f.web.value, allow_cmds: f.allow.value.split(/\n+/).map((x) => x.trim()).filter(Boolean) });
  toast("已保存");
  renderSide();
};
actions.agTexRefresh = async () => {
  const d = await api("GET", "/api/latex/engine?refresh=1");
  toast(d.engine.found ? "已检测到 " + d.engine.dist : "还是没有检测到，装好后可能需要重启工作台", !d.engine.found);
  renderSide();
};

// ----- 记忆 -----
async function renderMemory() {
  const box = $("#agSideBody");
  const m = await api("GET", "/api/agent/memory");
  if (A.side !== "memory") return;
  box.innerHTML = `<p class="muted" style="margin-top:0">智能体每次对话都会先读这里，用来记住你的习惯和常用信息（例如“论文在 D:\\论文，用 XeLaTeX 编译”“参考文献用 GB/T 7714”）。对话中说“记住……”它也会自己写进来。不要写密码和密钥。</p>
    <form data-submit="agMemory"><textarea name="m" rows="10" maxlength="${m.max}">${esc(m.memory)}</textarea>
    <div class="row" style="margin-top:6px"><span class="muted">最多 ${m.max} 字</span><span class="sp"></span><button type="submit" class="pri sm">保存</button></div></form>`;
}
actions.agMemory = async (f) => { await api("PUT", "/api/agent/memory", { memory: f.m.value }); toast("已保存"); };

// ----- 技能 -----
const CAT_ORDER = ["通用", "写作", "投稿", "文献", "生物医药化学", "数据与图表", "诚信", "实验", "导入", "我的"];
async function renderSkills() {
  const box = $("#agSideBody");
  const list = await api("GET", "/api/agent/skills");
  if (A.side !== "skills") return;
  A.skills = list;
  const groups = {};
  for (const s of list) (groups[s.category || "我的"] ||= []).push(s);
  const cats = Object.keys(groups).sort((a, b) => (CAT_ORDER.indexOf(a) + 99) % 99 - (CAT_ORDER.indexOf(b) + 99) % 99);
  box.innerHTML = `<p class="muted" style="margin-top:0">技能是写好的“做事步骤”。智能体遇到相关的事会自动调用；也可以直接说“用‘整理文件夹’技能处理 D:\\下载”。内置技能按开源项目的思路重新编写，注明了来源和许可证。</p>
    <input id="skFind" placeholder="搜索技能…" data-change="agSkillFind" style="margin-bottom:6px">
    ${cats.map((c) => `<details class="skcat" ${c === "通用" || c === "我的" || c === "导入" ? "open" : ""}><summary><b>${esc(c)}</b> <span class="muted">${groups[c].length}</span></summary>
      ${groups[c].map((s) => `<div class="skill" data-name="${esc((s.name + s.description).toLowerCase())}"><div class="row"><b>${esc(s.name)}</b>${s.builtin ? '<span class="tag">内置</span>' : ""}${s.license ? `<span class="tag ${/nc|non-?commercial/i.test(s.license) ? "warn" : ""}">${esc(s.license)}</span>` : ""}<span class="sp"></span>
        <button class="sm" data-act="agSkillUse" data-id="${esc(s.id)}">使用</button>
        ${s.builtin ? `<button class="sm" data-act="agSkillEdit" data-id="${esc(s.id)}">复制修改</button>` : `<button class="sm" data-act="agSkillEdit" data-id="${esc(s.id)}">编辑</button><button class="sm danger" data-act="agSkillDel" data-id="${esc(s.id)}">删除</button>`}</div>
        <div class="muted">${esc(s.description)}</div>
        ${s.credit ? `<div class="muted skcredit">${esc(s.credit)}</div>` : ""}${s.source ? `<div class="muted skcredit">来源：<a href="${esc(s.source)}" target="_blank" rel="noopener">${esc(s.source)}</a></div>` : ""}</div>`).join("")}</details>`).join("")}
    <div class="row" style="margin-top:10px"><button class="sm pri" data-act="agSkillEdit">＋ 新建技能</button><label class="btnlike sm">导入 SKILL.md 文件<input type="file" accept=".md,text/markdown,text/plain" data-change="agSkillImport" hidden></label></div>
    <form data-submit="agSkillGH" class="row" style="margin-top:8px"><input name="url" required placeholder="从 GitHub 导入：粘贴技能文件夹或 SKILL.md 的网址" style="flex:1;min-width:180px"><button type="submit" class="sm">导入</button></form>
    <p class="muted">例如 https://github.com/Yuan1z0825/nature-skills/tree/main/skills/nature-polishing 。只导入步骤说明（不下载脚本）；请留意许可证，标了 NC 的只能用于非商业用途。</p>`;
}
actions.agSkillFind = (el) => {
  const q = el.value.trim().toLowerCase();
  $$("#agSideBody .skill").forEach((d) => d.classList.toggle("hide", !!q && !d.dataset.name.includes(q)));
  if (q) $$("#agSideBody .skcat").forEach((d) => (d.open = true));
};
actions.agSkillGH = async (f) => {
  const r = await api("POST", "/api/agent/skills/import", { url: f.url.value.trim() });
  toast("已导入：" + r.skill.name);
  if (r.warning) alert(r.warning);
  renderSide();
};
actions.agSkillEdit = (el) => {
  const s = (A.skills || []).find((x) => x.id === el.dataset.id);
  const copy = s && s.builtin;
  modal(`<h3>${s && !copy ? "编辑技能" : "新建技能"}</h3><form data-submit="agSkillSave">
    <input type="hidden" name="id" value="${s && !copy ? esc(s.id) : ""}">
    <label>名称<input name="name" required maxlength="40" value="${esc(s ? (copy ? s.name + "（我的）" : s.name) : "")}"></label>
    <label>什么时候用（一句话）<input name="description" maxlength="200" value="${esc(s ? s.description : "")}" placeholder="例如：整理实验数据并出图时"></label>
    <label>步骤（写清楚先做什么、再做什么、注意什么）<textarea name="body" rows="12" required>${esc(s ? s.body : "1. \n2. \n3. ")}</textarea></label>
    <div class="row"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button type="submit" class="pri">保存</button></div></form>`);
};
actions.agSkillSave = async (f) => {
  A.skills = await api("POST", "/api/agent/skills", { id: f.id.value, name: f.name.value, description: f.description.value, body: f.body.value });
  closeModal();
  toast("已保存");
  renderSide();
};
actions.agSkillDel = async (el) => {
  if (!confirm("删除这个技能？")) return;
  await api("DELETE", "/api/agent/skills?id=" + encodeURIComponent(el.dataset.id));
  renderSide();
};
actions.agSkillImport = async (el) => {
  const f = el.files[0];
  el.value = "";
  if (!f) return;
  if (f.size > 40000) return toast("文件太大", true);
  await api("POST", "/api/agent/skills", { skill_md: await f.text() });
  toast("已导入");
  renderSide();
};
actions.agSkillUse = (el) => {
  const s = (A.skills || []).find((x) => x.id === el.dataset.id);
  if (!s) return;
  const t = $("#agText");
  t.value = `用“${s.name}”技能：` + (t.value ? "\n" + t.value : "");
  t.focus();
};

// ----- 定时任务 -----
const WEEK = ["日", "一", "二", "三", "四", "五", "六"];
function taskWhen(t) {
  if (t.kind === "daily") return `每天 ${t.time}`;
  if (t.kind === "weekly") return `每周${WEEK[t.weekday]} ${t.time}`;
  if (t.kind === "hours") return `每 ${t.every} 小时`;
  return `一次：${fmtTime(t.once_at)}`;
}
const isZero = (s) => !s || s.startsWith("0001-");
async function renderTasks() {
  const box = $("#agSideBody");
  const list = await api("GET", "/api/agent/tasks");
  if (A.side !== "tasks") return;
  A.tasks = list;
  box.innerHTML = `<p class="muted" style="margin-top:0">让智能体定时自动做事，例如每周一早上追踪新文献、每天晚上检查论文能否编译。工作台开着才会执行（错过的会在开机后补做一次）。无人值守时<b>不会</b>运行需要确认的命令，改文件也只会放进“待确认”等你回来处理。</p>
    ${list.length ? list.map((t) => `<div class="skill"><div class="row"><b>${esc(t.name)}</b><span class="tag ${t.enabled ? "ok" : ""}">${t.enabled ? taskWhen(t) : "已暂停"}</span><span class="sp"></span>
      <button class="sm" data-act="agTaskRun" data-id="${esc(t.id)}">立即执行</button><button class="sm" data-act="agTaskEdit" data-id="${esc(t.id)}">编辑</button><button class="sm danger" data-act="agTaskDel" data-id="${esc(t.id)}">删除</button></div>
      <div class="muted pre">${esc(t.prompt)}</div>
      ${t.enabled && !isZero(t.next_run) ? `<div class="muted">下次：${fmtTime(t.next_run)}</div>` : ""}
      ${(t.runs || []).length ? `<details><summary class="muted">最近执行（${t.runs.length}）</summary>${t.runs.slice().reverse().map((r) => `<div class="taskrun"><span class="tag ${r.status === "ok" ? "ok" : "bad"}">${r.status === "ok" ? "完成" : "出错"}</span> ${fmtTime(r.at)}${r.pending ? ` <span class="tag warn">${r.pending} 处修改待确认</span>` : ""}
        ${r.session_id ? `<a href="#" data-act="agOpenSession" data-id="${esc(r.session_id)}">查看过程 →</a>` : ""}<div class="muted">${esc(r.summary || "")}</div></div>`).join("")}</details>` : ""}</div>`).join("") : '<p class="muted">还没有定时任务。</p>'}
    <button class="sm pri" data-act="agTaskEdit" style="margin-top:8px">＋ 新建定时任务</button>`;
}
function localInput(iso) {
  const d = isZero(iso) ? new Date(Date.now() + 3600e3) : new Date(iso);
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}
actions.agTaskEdit = (el) => {
  const t = (A.tasks || []).find((x) => x.id === el.dataset.id) || { kind: "weekly", time: "08:30", weekday: 1, every: 6, enabled: true, prompt: "", name: "", project_id: A.pid };
  const k = (v) => (t.kind === v ? "selected" : "");
  modal(`<h3>${t.id ? "编辑定时任务" : "新建定时任务"}</h3><form data-submit="agTaskSave">
    <input type="hidden" name="id" value="${esc(t.id || "")}">
    <label>名称<input name="name" required maxlength="40" value="${esc(t.name)}" placeholder="例如：每周文献追踪"></label>
    <label>要做的事（和平时对智能体说的话一样）<textarea name="prompt" rows="5" required maxlength="2000" placeholder="例如：用“文献追踪”技能，查最近一周关于 xxx 的新论文，挑最相关的 5 篇加入资料库，并总结要点">${esc(t.prompt)}</textarea></label>
    <div class="row">
      <label>频率<select name="kind" data-change="agTaskKind"><option value="daily" ${k("daily")}>每天</option><option value="weekly" ${k("weekly")}>每周</option><option value="hours" ${k("hours")}>每隔几小时</option><option value="once" ${k("once")}>只执行一次</option></select></label>
      <label class="tk tk-weekly">星期<select name="weekday">${WEEK.map((w, i) => `<option value="${i}" ${i === t.weekday ? "selected" : ""}>周${w}</option>`).join("")}</select></label>
      <label class="tk tk-daily tk-weekly">时间<input type="time" name="time" value="${esc(t.time || "08:30")}"></label>
      <label class="tk tk-hours">间隔（小时）<input type="number" name="every" min="1" max="168" value="${t.every || 6}"></label>
      <label class="tk tk-once">时间<input type="datetime-local" name="once" value="${localInput(t.once_at)}"></label>
    </div>
    <label>资料库<select name="project_id"><option value="">我的资料</option>${(A.plist || []).map((p) => `<option value="${esc(p.id)}" ${p.id === t.project_id ? "selected" : ""}>${esc(p.name)}</option>`).join("")}</select></label>
    <label class="inline"><input type="checkbox" name="enabled" ${t.enabled ? "checked" : ""}> 启用</label>
    <div class="row"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button type="submit" class="pri">保存</button></div></form>`);
  taskKindShow($("#modalBody select[name=kind]"));
};
function taskKindShow(sel) {
  $$("#modalBody .tk").forEach((l) => l.classList.toggle("hide", !l.classList.contains("tk-" + sel.value)));
}
actions.agTaskKind = (el) => taskKindShow(el);
actions.agTaskSave = async (f) => {
  const body = { id: f.id.value, name: f.name.value, prompt: f.prompt.value, kind: f.kind.value, time: f.time.value, weekday: Number(f.weekday.value), every: Number(f.every.value), project_id: f.project_id.value, enabled: f.enabled.checked };
  if (body.kind === "once") body.once_at = new Date(f.once.value).toISOString();
  await api("POST", "/api/agent/tasks", body);
  closeModal();
  toast("已保存");
  renderSide();
};
actions.agTaskDel = async (el) => {
  if (!confirm("删除这个定时任务？")) return;
  await api("DELETE", "/api/agent/tasks?id=" + encodeURIComponent(el.dataset.id));
  renderSide();
};
actions.agTaskRun = async (el) => {
  const r = await api("POST", `/api/agent/tasks/${encodeURIComponent(el.dataset.id)}/run`, {});
  toast("已开始执行");
  if (r.session_id) openSession(r.session_id);
};
actions.agOpenSession = (el) => openSession(el.dataset.id);
async function openSession(id) {
  A.sid = id;
  A.side = "changes";
  await renderAgent($("#asBody"));
}

// ----- 对话 -----
async function addAttach(files) {
  for (const f of files) {
    if (A.attach.length >= 4) { toast("一次最多附 4 张图片", true); break; }
    try { A.attach.push(await fileToDataURL(f)); } catch (e) { toast(e.message, true); }
  }
  renderAttach();
}
function renderAttach() {
  const box = $("#agAttach");
  if (!box) return;
  box.innerHTML = A.attach.map((d, i) => `<span class="thumb"><img src="${d}" alt=""><button type="button" class="sm" data-act="agUnattach" data-i="${i}">×</button></span>`).join("");
}
actions.agPick = (el) => { addAttach([...el.files]); el.value = ""; };
actions.agUnattach = (el) => { A.attach.splice(Number(el.dataset.i), 1); renderAttach(); };
actions.agSwitch = async (el) => {
  A.sid = el.value; A.total = 0; A.changes = []; A.pending = null;
  $("#agSteps").innerHTML = A.sid ? "" : introHTML();
  $("#agApprove").innerHTML = ""; $("#agUnattended").innerHTML = "";
  renderSide();
  if (A.sid) await poll(true);
};

actions.agSend = async (f) => {
  const text = f.text.value.trim();
  if (!text && !A.attach.length) return toast("请输入要做的事", true);
  A.pid = $("#agPid").value;
  if (!A.sid) {
    A.sid = (await api("POST", "/api/agent/sessions", { project_id: A.pid })).id;
    A.total = 0;
    $("#agSteps").innerHTML = "";
  }
  await api("POST", `/api/agent/sessions/${A.sid}/messages`, { text, images: A.attach });
  A.attach = [];
  renderAttach();
  f.text.value = "";
  $("#agGo").textContent = "处理中…";
  setTimeout(() => poll(false), 0); // 等表单提交处理结束（它会重新启用按钮）后再按运行状态设置
};
actions.agStop = async () => { if (A.sid) await api("POST", `/api/agent/sessions/${A.sid}/stop`, {}); toast("会在当前这一步完成后停止"); };

async function poll(reset) {
  clearTimeout(A.timer);
  if (!A.sid || !$("#agSteps")) return;
  const sid = A.sid;
  let r;
  try { r = await api("GET", `/api/agent/sessions/${sid}?since=${reset ? 0 : A.total}`); } catch (e) { toast(e.message, true); return; }
  if (sid !== A.sid || !$("#agSteps")) return;
  if (reset) $("#agSteps").innerHTML = "";
  $("#agSteps").insertAdjacentHTML("beforeend", r.steps.map((s) => stepHTML(s, sid)).join(""));
  if (r.steps.length) $("#agSteps").scrollTop = $("#agSteps").scrollHeight;
  A.total = r.total;
  A.running = r.running;
  const before = JSON.stringify(A.changes.map((c) => c.id + c.status));
  A.changes = r.changes || [];
  const nPending = A.changes.filter((c) => c.status === "pending").length;
  const badge = $("#agBadge-changes");
  if (badge) badge.innerHTML = nPending ? ` <span class="badge">${nPending}</span>` : "";
  if (JSON.stringify(A.changes.map((c) => c.id + c.status)) !== before || reset) {
    if (nPending && A.side !== "changes" && !reset) { A.side = "changes"; $$(".tabs.sub button").forEach((x) => x.classList.toggle("on", x.dataset.s === "changes")); }
    if (A.side === "changes") renderChanges();
  }
  $("#agUnattended").innerHTML = r.unattended ? '<div class="msg warnbox">这是定时任务在无人值守时的对话：需要确认的操作已跳过，提出的修改在“待确认”里等你处理。</div>' : "";
  renderApproval(r.pending);
  $$("#agSteps .approvalstep").forEach((x) => x.classList.toggle("live", !!r.pending && x.dataset.id === r.pending.id));
  $("#agStop").classList.toggle("hide", !r.running);
  $("#agGo").disabled = r.running;
  $("#agGo").textContent = r.running ? "处理中…" : "发送";
  if (r.running) A.timer = setTimeout(() => poll(false), r.pending ? 1500 : 800);
}

function renderApproval(p) {
  const box = $("#agApprove");
  if (!box) return;
  if (!p) { A.pending = null; box.innerHTML = ""; return; }
  if (A.pending && A.pending.id === p.id) return;
  A.pending = p;
  box.innerHTML = `<div class="approve ${p.danger ? "danger" : ""}">
    <div class="row"><b>${p.kind === "open" ? "🗂 智能体想打开" : "⌨ 智能体想运行命令"}：${esc(p.title)}</b></div>
    <pre>${esc(p.detail)}</pre>
    ${p.danger ? `<div class="dangertext">⚠ ${esc(p.danger)}，请看清楚再决定</div>` : ""}
    <div class="row"><button class="pri sm" data-act="agApprove" data-d="once">允许一次</button>
      ${p.can_always ? `<button class="sm" data-act="agApprove" data-d="always" title="以后运行 ${esc(p.always_key)} 开头的命令不再询问">始终允许“${esc(p.always_key)}”</button>` : ""}
      <button class="sm danger" data-act="agApprove" data-d="deny">拒绝</button><span class="sp"></span><span class="muted">10 分钟内不处理会自动拒绝</span></div></div>`;
  box.scrollIntoView({ block: "nearest", behavior: "smooth" });
}
actions.agApprove = async (el) => {
  if (!A.pending) return;
  $$("#agApprove button").forEach((b) => (b.disabled = true));
  try {
    await api("POST", `/api/agent/sessions/${A.sid}/approve`, { id: A.pending.id, decision: el.dataset.d });
  } catch (e) { toast(e.message, true); }
  A.pending = null;
  $("#agApprove").innerHTML = "";
  poll(false);
};

const TOOL_ICON = { list_folders: "📁", list_dir: "📁", search_files: "🔍", read_file: "📄", edit_file: "✏️", write_file: "✏️", check_latex: "✔", recognize_image: "🖼", search_library: "📚",
  run_command: "⌨", file_op: "🗂", open: "↗", fetch_url: "🌐", search_papers: "🔎", add_paper: "➕", compile_latex: "📑", remember: "🧠", forget: "🧠", use_skill: "🧩" };
function stepHTML(s, sid) {
  switch (s.kind) {
    case "user":
      return `<div class="bubble me"><div class="pre">${esc(s.text)}</div>${(s.images || []).map((n) => `<img class="thumbimg" src="/api/agent/sessions/${esc(sid)}/images/${n}" alt="图片${n}" title="图片${n}">`).join("")}</div>`;
    case "say":
      return `<div class="agsay">${esc(s.text)}</div>`;
    case "tool":
      return `<div class="agtool">${TOOL_ICON[s.tool] || "▸"} ${esc(s.text)}</div>`;
    case "approval":
      return `<div class="agtool approvalstep" data-id="${esc(s.approval || "")}">⏸ 等待你确认：${esc(s.text)}</div>`;
    case "result":
      return `<details class="agres"><summary>${s.text.startsWith("失败") ? `<span style="color:var(--bad)">${esc(s.text)}</span>` : esc(s.text)}${s.change ? ' <a href="#" data-act="agGoChange" data-id="' + esc(s.change) + '">查看 →</a>' : ""}</summary><pre>${esc(s.detail || "")}</pre></details>`;
    case "reply":
      return `<div class="bubble ai"><div class="pre">${esc(s.text)}</div></div>`;
    case "error":
      return `<div class="msg">${esc(s.text)}</div>`;
    default:
      return `<div class="aginfo">${esc(s.text)}</div>`;
  }
}
actions.agGoChange = (el) => {
  if (A.side !== "changes") { A.side = "changes"; $$(".tabs.sub button").forEach((x) => x.classList.toggle("on", x.dataset.s === "changes")); renderChanges(); }
  const c = document.getElementById("ch-" + el.dataset.id);
  if (c) { c.scrollIntoView({ behavior: "smooth" }); c.classList.add("flash"); setTimeout(() => c.classList.remove("flash"), 1500); }
};

const ST = { pending: ["待确认", "warn"], applied: ["已完成", "ok"], rejected: ["未采纳", ""], undone: ["已撤销", ""], conflict: ["未执行（有冲突）", "bad"] };
const KIND = { create: "新建", modify: "修改", move: "移动", copy: "复制", mkdir: "新建文件夹", delete: "删除", pdf: "生成 PDF", files: "保存图片" };
const kindName = (c) => (c.kind === "office" ? (c.sub === "create" ? "新建 Word" : "修改 Word") : KIND[c.kind] || c.kind);
const base = (p) => (p || "").split(/[\\/]/).pop();
function changeBody(c) {
  if (c.kind === "pdf") {
    return `${c.pdf_id ? `<div class="row"><a class="btnlike sm" href="/api/latex/pdf/${esc(c.pdf_id)}" target="_blank" rel="noopener">预览 PDF</a><a class="btnlike sm" href="/api/latex/pdf/${esc(c.pdf_id)}?download=1">下载</a></div>` : ""}
      ${c.readonly ? '<div class="muted">这个文件夹是只读的，PDF 不能保存到文件夹里，请用上面的按钮下载。</div>' : `<div class="muted">点“保存”会把 PDF 写到：${esc(c.path)}${c.status === "pending" ? "（如有同名文件，会先备份）" : ""}</div>`}`;
  }
  if (c.kind === "move" || c.kind === "copy") return `<div class="fromto"><code>${esc(c.from)}</code><span>→</span><code>${esc(c.path)}</code></div>`;
  if (c.kind === "mkdir") return `<div class="fromto"><code>${esc(c.path)}</code></div>`;
  if (c.kind === "delete") return `<div class="fromto"><code>${esc(c.path)}</code></div><div class="muted">不会直接删掉：会移到工作台的回收区，点“撤销”可以放回原处。</div>`;
  return `<div class="muted" style="word-break:break-all">${esc(c.path)}</div>
    <details ${c.status === "pending" ? "open" : ""}><summary class="muted">查看改动</summary><div class="diff">${(c.diff || []).map((d) => `<div class="${{ "+": "a", "-": "d", "~": "s", " ": "" }[d.op]}"><span class="ln">${d.op === "~" ? "" : d.op === "+" ? d.new : d.old}</span>${esc(d.op === "~" ? d.text : d.op + " " + d.text)}</div>`).join("")}</div></details>`;
}
function renderChanges() {
  const box = $("#agSideBody");
  if (!box || A.side !== "changes") return;
  if (!A.changes.length) {
    box.innerHTML = '<p class="muted" style="margin-top:0">智能体提出的修改、移动、删除和生成的 PDF 会显示在这里，你点“应用”后才会真正执行；执行前自动备份，可以撤销。</p>';
    return;
  }
  const order = { pending: 0, conflict: 1, applied: 2, rejected: 3, undone: 3 };
  const list = [...A.changes].sort((a, b) => order[a.status] - order[b.status] || ((a.at < b.at ? 1 : -1) * (a.status === "pending" ? -1 : 1)));
  const np = list.filter((c) => c.status === "pending" && !(c.kind === "pdf" && c.readonly)).length;
  box.innerHTML = (np > 1 ? `<div class="row" style="margin-bottom:6px"><span class="muted">${np} 处待确认</span><span class="sp"></span><button class="sm" data-act="agApplyAll">全部应用</button></div>` : "") +
    list.map((c) => `<div class="change" id="ch-${esc(c.id)}">
    <div class="row"><b class="path" title="${esc(c.path)}">${esc(base(c.path))}</b><span class="tag">${kindName(c)}</span><span class="tag ${ST[c.status][1]}">${ST[c.status][0]}</span>${c.kind === "create" || c.kind === "modify" || c.kind === "office" ? `<span class="muted">+${c.added} −${c.removed}</span>` : ""}</div>
    ${c.reason ? `<div>${esc(c.reason)}</div>` : ""}
    ${c.note ? `<div class="msg">${esc(c.note)}</div>` : ""}
    ${(c.warnings || []).length ? warnHTML(c.warnings) : ""}
    ${changeBody(c)}
    <div class="row" style="margin-top:6px">${c.status === "pending" && !(c.kind === "pdf" && c.readonly) ? `<button class="sm pri" data-act="agChange" data-id="${esc(c.id)}" data-a="apply">${c.kind === "pdf" ? "保存到文件夹" : c.kind === "delete" ? "确认删除" : "应用"}</button><button class="sm" data-act="agChange" data-id="${esc(c.id)}" data-a="reject">不要</button>` : c.status === "applied" ? `<button class="sm" data-act="agChange" data-id="${esc(c.id)}" data-a="undo">撤销</button>` : ""}</div>
  </div>`).join("");
}
actions.agChange = async (el) => {
  const a = el.dataset.a;
  const c = A.changes.find((x) => x.id === el.dataset.id);
  if (a === "apply" && c?.warnings?.length && !confirm(c.kind === "office" ? "这处 Word 修改有需要注意的地方（见黄色提示），仍然写入吗？" : "LaTeX 检查发现这处修改可能引入问题，仍然写入吗？")) return;
  el.disabled = true;
  try {
    await api("POST", `/api/agent/sessions/${A.sid}/changes/${el.dataset.id}/${a}`, {});
    toast({ apply: c?.kind === "pdf" ? "PDF 已保存" : "已完成（原文件已备份）", reject: "已放弃", undo: "已撤销，已恢复原样" }[a]);
  } finally { await poll(false); renderChanges(); }
};
actions.agApplyAll = async (el) => {
  const list = A.changes.filter((c) => c.status === "pending" && !(c.kind === "pdf" && c.readonly));
  if (list.some((c) => c.warnings?.length) && !confirm("有的修改标出了需要注意的地方（见黄色提示），仍然全部应用吗？")) return;
  if (list.some((c) => c.kind === "delete") && !confirm(`其中有 ${list.filter((c) => c.kind === "delete").length} 处删除（会移到回收区，可撤销），继续吗？`)) return;
  el.disabled = true;
  let fail = 0;
  // 按提出的先后顺序执行（例如先建文件夹再移动进去）
  for (const c of list.sort((a, b) => (a.at < b.at ? -1 : 1))) {
    try { await api("POST", `/api/agent/sessions/${A.sid}/changes/${c.id}/apply`, {}); } catch { fail++; }
  }
  toast(fail ? `${list.length - fail} 处完成，${fail} 处没有成功` : "全部完成", !!fail);
  await poll(false);
  renderChanges();
};

// ---------------- 操作记录 ----------------
async function renderLogs(b) {
  const logs = await api("GET", "/api/agent/logs");
  b.innerHTML = `<div class="card"><h3>本机智能体操作记录</h3><p class="muted">智能体每次查看、读取、运行命令、提交修改，以及你的确认、应用、放弃、撤销，都记录在这里（最近 300 条）。</p>
    ${logs.length ? `<div class="tw"><table><tr><th>时间</th><th>操作</th><th>文件</th><th>说明</th></tr>
    ${logs.map((l) => `<tr><td style="white-space:nowrap">${fmtTime(l.at)}</td><td style="white-space:nowrap">${esc(l.action)}</td><td style="word-break:break-all">${esc(l.path || "")}</td><td class="muted">${esc(l.detail || "")}</td></tr>`).join("")}</table></div>` : '<p class="muted">还没有记录。</p>'}</div>`;
}
