// 手绘转图：在画板上随手画（鼠标、手指、手写笔都行）或拍一张纸上的草图，AI 理解意图后生成规范的 TikZ 图，
// 可以一句话调整、导出 PDF，或交给本机智能体放进论文。
import { $, esc, api, toast, actions } from "/core.js";
import { exportPDF } from "/texpdf.js";

const TARGETS = [["auto", "自动判断"], ["flow", "流程图 / 框图"], ["function", "函数图像"], ["geometry", "几何示意图"], ["chart", "数据图表"], ["device", "实验装置 / 结构图"], ["circuit", "电路图"], ["network", "网络 / 架构图"]];
const COLORS = ["#14161F", "#1E2AB0", "#b42318", "#157f47"];
const S = { strokes: [], cur: null, color: COLORS[0], width: 3, eraser: false, bg: null, out: null, ctx: null, local: false, toAgent: null };

export function renderSketch(b, opts) {
  S.local = !!opts.local;
  S.toAgent = opts.toAgent;
  b.innerHTML = `<div class="card">
    <p class="muted" style="margin-top:0">随手画个草图：流程图、函数曲线、几何图、实验装置、电路……线画歪了、没闭合都没关系，AI 会理解你想画什么，生成规范的 LaTeX（TikZ）图。也可以<b>拍一张纸上的草图</b>。识图模型：<b>${esc(opts.vision)}</b>${opts.visionOK ? "" : `<br><span style="color:var(--bad)">还没有能看图片的模型</span>，请到 <a href="#/settings">设置 → 我的 AI 模型</a> 添加并通过自检。`}</p>
    <div class="row sktools">
      <button class="sm ${S.eraser ? "" : "on"}" data-act="skPen">笔</button><button class="sm ${S.eraser ? "on" : ""}" data-act="skEraser">橡皮</button>
      ${COLORS.map((c) => `<button class="sm swatch ${c === S.color && !S.eraser ? "on" : ""}" data-act="skColor" data-c="${c}" style="background:${c}" title="颜色"></button>`).join("")}
      <select data-change="skWidth" style="width:auto">${[2, 3, 5, 8].map((w) => `<option value="${w}" ${w === S.width ? "selected" : ""}>粗细 ${w}</option>`).join("")}</select>
      <button class="sm" data-act="skUndo">撤销</button><button class="sm" data-act="skClear">清空</button>
      <label class="btnlike sm">拍照 / 上传草图<input type="file" accept="image/*" data-change="skPhoto" hidden></label>
    </div>
    <canvas id="skCanvas" class="skcanvas" width="1200" height="720" aria-label="手绘画板"></canvas>
    <div class="row" style="margin-top:10px">
      <select id="skTarget" style="width:auto">${TARGETS.map(([k, n]) => `<option value="${k}">${n}</option>`).join("")}</select>
      <input id="skNote" placeholder="补充说明（可选），例如：三个方框从左到右，最后一个是“输出”；曲线是 y=e^{-x}" style="flex:1;min-width:200px">
      <button class="pri" data-act="skGo">生成图</button>
    </div></div><div id="skOut"></div>`;
  const cv = $("#skCanvas");
  S.ctx = cv.getContext("2d");
  cv.addEventListener("pointerdown", down);
  cv.addEventListener("pointermove", move);
  cv.addEventListener("pointerup", up);
  cv.addEventListener("pointercancel", up);
  cv.addEventListener("pointerleave", up);
  redraw();
  if (S.out) showOut();
}

function pos(e, cv = e.currentTarget) {
  const r = cv.getBoundingClientRect();
  return [((e.clientX - r.left) / r.width) * cv.width, ((e.clientY - r.top) / r.height) * cv.height];
}
function down(e) {
  e.preventDefault();
  e.currentTarget.setPointerCapture?.(e.pointerId);
  const k = e.pointerType === "pen" && e.pressure ? 0.6 + e.pressure : 1;
  S.cur = { color: S.eraser ? "#ffffff" : S.color, width: (S.eraser ? S.width * 5 : S.width) * k * 1.5, pts: [pos(e)] };
  S.strokes.push(S.cur);
  redraw();
}
function move(e) {
  if (!S.cur) return;
  e.preventDefault();
  const evs = e.getCoalescedEvents ? e.getCoalescedEvents() : [e];
  for (const ev of evs) S.cur.pts.push(pos(ev, e.currentTarget));
  redraw();
}
function up() { S.cur = null; }

function redraw() {
  const g = S.ctx;
  if (!g) return;
  const cv = g.canvas;
  g.fillStyle = "#fff";
  g.fillRect(0, 0, cv.width, cv.height);
  if (S.bg) {
    const k = Math.min(cv.width / S.bg.naturalWidth, cv.height / S.bg.naturalHeight);
    const w = S.bg.naturalWidth * k, h = S.bg.naturalHeight * k;
    g.drawImage(S.bg, (cv.width - w) / 2, (cv.height - h) / 2, w, h);
  }
  g.lineCap = "round"; g.lineJoin = "round";
  for (const s of S.strokes) {
    g.strokeStyle = s.color; g.lineWidth = s.width;
    g.beginPath();
    s.pts.forEach(([x, y], i) => (i ? g.lineTo(x, y) : g.moveTo(x, y)));
    if (s.pts.length === 1) g.lineTo(s.pts[0][0] + 0.1, s.pts[0][1]);
    g.stroke();
  }
}

function toolbarOn(el) { el.parentElement.querySelectorAll("button").forEach((b) => b.classList.toggle("on", b === el || (b.dataset.c === S.color && !S.eraser && b.dataset.act === "skColor"))); }
actions.skPen = (el) => { S.eraser = false; toolbarOn(el); };
actions.skEraser = (el) => { S.eraser = true; toolbarOn(el); };
actions.skColor = (el) => { S.color = el.dataset.c; S.eraser = false; toolbarOn(el); el.parentElement.querySelector('[data-act="skPen"]').classList.add("on"); };
actions.skWidth = (el) => { S.width = Number(el.value); };
actions.skUndo = () => { S.strokes.pop(); if (!S.strokes.length && S.bg && confirm("要把照片也去掉吗？")) S.bg = null; redraw(); };
actions.skClear = () => { S.strokes = []; S.bg = null; redraw(); };
actions.skPhoto = (el) => {
  const f = el.files[0];
  el.value = "";
  if (!f || !f.type.startsWith("image/")) return;
  const url = URL.createObjectURL(f);
  const img = new Image();
  img.onload = () => { S.bg = img; redraw(); };
  img.onerror = () => toast("无法读取这张图片", true);
  img.src = url;
};

function isBlank() {
  if (S.bg) return false;
  return !S.strokes.some((s) => s.color !== "#ffffff");
}
function canvasImage() {
  // 裁掉四周空白，缩小后发给模型，省流量也省 tokens
  const cv = S.ctx.canvas, g = S.ctx;
  const { data, width: W, height: H } = g.getImageData(0, 0, cv.width, cv.height);
  let x0 = W, y0 = H, x1 = -1, y1 = -1;
  for (let y = 0; y < H; y += 2) for (let x = 0; x < W; x += 2) {
    const i = (y * W + x) * 4;
    if (data[i] < 245 || data[i + 1] < 245 || data[i + 2] < 245) { if (x < x0) x0 = x; if (x > x1) x1 = x; if (y < y0) y0 = y; if (y > y1) y1 = y; }
  }
  if (x1 < 0) return cv.toDataURL("image/png");
  const pad = 30;
  x0 = Math.max(0, x0 - pad); y0 = Math.max(0, y0 - pad); x1 = Math.min(W, x1 + pad); y1 = Math.min(H, y1 + pad);
  const c = document.createElement("canvas");
  c.width = x1 - x0; c.height = y1 - y0;
  const cg = c.getContext("2d");
  cg.fillStyle = "#fff"; cg.fillRect(0, 0, c.width, c.height);
  cg.drawImage(cv, x0, y0, c.width, c.height, 0, 0, c.width, c.height);
  return S.bg ? c.toDataURL("image/jpeg", 0.9) : c.toDataURL("image/png");
}

actions.skGo = async (el) => {
  if (isBlank()) return toast("先在画板上画点什么，或上传一张草图", true);
  el.disabled = true; el.textContent = "识别中…";
  $("#skOut").innerHTML = `<div class="card muted">AI 正在理解你的草图并画成规范的图，通常需要 10–40 秒…</div>`;
  try {
    S.out = await api("POST", "/api/sketch", { image: canvasImage(), target: $("#skTarget").value, note: $("#skNote").value });
    showOut();
  } catch (e) {
    $("#skOut").innerHTML = `<div class="card"><div class="msg">${esc(e.message)}</div></div>`;
  } finally { el.disabled = false; el.textContent = "生成图"; }
};

function svgURL(svg) { return "data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg); }

function showOut() {
  const o = S.out, box = $("#skOut");
  if (!box) return;
  box.innerHTML = `<div class="card">
    <div class="row"><h3 style="margin:0">AI 理解的意图</h3><span class="muted">由 ${esc(o.model)} 生成</span></div>
    <p>${esc(o.intent || "（未说明）")}</p>${o.note ? `<p class="muted">说明：${esc(o.note)}</p>` : ""}
    <div class="skres">
      <div><h4>预览</h4>${o.svg ? `<img class="skprev" src="${svgURL(o.svg)}" alt="生成的图（预览）">` : '<p class="muted">模型没有给出预览图，可以直接导出 PDF 查看实际效果。</p>'}
        <p class="muted">预览是示意，以导出的 PDF 为准。</p></div>
      <div><h4>TikZ 代码</h4><textarea id="skTikz" class="code" spellcheck="false">${esc(o.tikz)}</textarea></div>
    </div>
    ${o.lint && o.lint.length ? `<div class="msg warnbox"><b>可能的问题：</b>${o.lint.map((w) => `<div>${esc(w)}</div>`).join("")}</div>` : ""}
    <div class="row" style="margin-top:8px">
      <button class="pri" data-act="skCopy">复制代码</button>
      ${S.local ? '<button data-act="skPDF">导出 PDF</button><button data-act="skAgent">交给智能体放进论文</button>' : ""}
    </div>
    <form data-submit="skRefine" class="row" style="margin-top:10px">
      <input name="ins" placeholder="一句话调整，例如：箭头改成虚线；把“处理”框改成菱形判断；坐标轴加上刻度" style="flex:1;min-width:200px" required>
      <button type="submit">调整</button></form>
    <div id="skPDF" style="margin-top:10px"></div>
    ${S.local ? "" : '<p class="muted">导出 PDF 需要在运行工作台的电脑上操作（用那台电脑上的 LaTeX 编译）。</p>'}</div>`;
}

actions.skCopy = async () => {
  const t = $("#skTikz").value;
  try { await navigator.clipboard.writeText(t); } catch { $("#skTikz").select(); document.execCommand("copy"); }
  toast("已复制。使用时导言区需要 \\usepackage{tikz}（按需加 pgfplots、circuitikz）");
};
actions.skPDF = () => exportPDF($("#skPDF"), $("#skTikz").value, "tikz", "sketch");
actions.skRefine = async (f) => {
  const o = S.out;
  const r = await api("POST", "/api/sketch/refine", { intent: o.intent, tikz: $("#skTikz").value, instruction: f.ins.value });
  S.out = r;
  showOut();
  toast("已调整");
};
actions.skAgent = () => {
  if (!S.toAgent) return;
  S.toAgent(`请把下面这张图（TikZ，意图：${S.out.intent || ""}）放进我的论文合适的位置，用 figure 环境包起来并加上图题；如果导言区缺少需要的宏包（tikz、pgfplots、circuitikz 等）请一并补上，改完编译一次确认没有错误：\n${$("#skTikz").value}\n\n文件和位置：`);
};
