// 通用工具：接口调用、转义、弹窗、提示、事件委托、PDF 文字提取

export const $ = (s, r = document) => r.querySelector(s);
export const $$ = (s, r = document) => [...r.querySelectorAll(s)];

export const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

export class ApiError extends Error {
  constructor(status, msg) { super(msg); this.status = status; }
}

let onUnauthorized = () => {};
export function setUnauthorizedHandler(fn) { onUnauthorized = fn; }

export async function api(method, path, body) {
  const opt = { method, headers: { "X-KY": "1" }, credentials: "same-origin" };
  if (body instanceof FormData) opt.body = body;
  else if (body !== undefined) { opt.body = JSON.stringify(body); opt.headers["Content-Type"] = "application/json"; }
  let r;
  try { r = await fetch(path, opt); }
  catch { throw new ApiError(0, "无法连接工作台。请确认运行工作台的电脑已开机、程序在运行，且手机与电脑在同一个 Wi-Fi 下。"); }
  let data = null;
  const ct = r.headers.get("content-type") || "";
  if (ct.includes("json")) data = await r.json();
  if (r.status === 401 && path !== "/api/login") { onUnauthorized(); throw new ApiError(401, (data && data.detail) || "请先登录"); }
  if (!r.ok) throw new ApiError(r.status, (data && data.detail) || `请求失败（${r.status}）`);
  return data;
}

let toastTimer;
export function toast(msg, err = false) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast" + (err ? " err" : "");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add("hide"), err ? 5000 : 2600);
}

export function modal(html) {
  $("#modalBody").innerHTML = html;
  $("#modal").classList.remove("hide");
  const f = $("#modalBody input, #modalBody textarea, #modalBody select");
  if (f && window.innerWidth > 760) f.focus();
}
export function closeModal() { $("#modal").classList.add("hide"); $("#modalBody").innerHTML = ""; }

// 事件委托：元素上写 data-act="名称"，在 actions 中注册处理函数（页面有 CSP，不能用内联 onclick）
export const actions = {};
document.addEventListener("click", (e) => {
  const el = e.target.closest("[data-act]");
  if (!el) return;
  const fn = actions[el.dataset.act];
  if (fn) { e.preventDefault(); Promise.resolve(fn(el, e)).catch(showErr); }
});
document.addEventListener("change", (e) => {
  const el = e.target.closest("[data-change]");
  if (!el) return;
  const fn = actions[el.dataset.change];
  if (fn) Promise.resolve(fn(el, e)).catch(showErr);
});
// 边输入边响应（搜索框等）：元素上写 data-input="名称"
document.addEventListener("input", (e) => {
  const el = e.target.closest("[data-input]");
  if (!el) return;
  const fn = actions[el.dataset.input];
  if (fn) Promise.resolve(fn(el, e)).catch(showErr);
});
document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (document.querySelector(".menu.pop")) return closeMenus();
  if (!$("#modal").classList.contains("hide")) closeModal();
});

// 弹出菜单（“⋯ 更多”）：items = [{ label, act, data: {键: 值}, cls, icon }]，菜单项仍走 data-act
export function closeMenus() { document.querySelectorAll(".menu.pop").forEach((m) => m.remove()); }
export function popMenu(anchor, items) {
  const open = anchor.classList.contains("menuon");
  closeMenus();
  document.querySelectorAll(".menuon").forEach((x) => x.classList.remove("menuon"));
  if (open) return;
  const m = document.createElement("div");
  m.className = "menu pop";
  m.innerHTML = items.filter(Boolean).map((it) => it === "-" ? "<hr>" : it.href
    ? `<a href="${esc(it.href)}" ${it.target ? `target="${it.target}"` : ""} class="${it.cls || ""}">${it.icon ? `<svg class="ic sm"><use href="#${it.icon}"/></svg>` : ""}${esc(it.label)}</a>`
    : `<button type="button" class="${it.cls || ""}" data-act="${it.act}" ${Object.entries(it.data || {}).map(([k, v]) => `data-${k}="${esc(String(v))}"`).join(" ")}>${it.icon ? `<svg class="ic sm"><use href="#${it.icon}"/></svg>` : ""}${esc(it.label)}</button>`).join("");
  document.body.appendChild(m);
  anchor.classList.add("menuon");
  const r = anchor.getBoundingClientRect(), w = m.offsetWidth, h = m.offsetHeight;
  const left = Math.max(8, Math.min(r.right - w, innerWidth - w - 8));
  const top = r.bottom + h + 6 > innerHeight && r.top - h - 6 > 0 ? r.top - h - 4 : r.bottom + 4;
  Object.assign(m.style, { position: "fixed", left: left + "px", top: top + "px", right: "auto" });
}
document.addEventListener("click", (e) => {
  if (e.target.closest(".menuon")) return;
  if (document.querySelector(".menu.pop")) { closeMenus(); document.querySelectorAll(".menuon").forEach((x) => x.classList.remove("menuon")); }
}, true);
window.addEventListener("scroll", () => { if (document.querySelector(".menu.pop")) { closeMenus(); document.querySelectorAll(".menuon").forEach((x) => x.classList.remove("menuon")); } }, true);

document.addEventListener("submit", (e) => {
  const el = e.target.closest("form[data-submit]");
  if (!el) return;
  e.preventDefault();
  const fn = actions[el.dataset.submit];
  if (fn) {
    const btn = el.querySelector("button[type=submit]");
    if (btn) btn.disabled = true;
    Promise.resolve(fn(el, e)).catch(showErr).finally(() => { if (btn) btn.disabled = false; });
  }
});
$("#modal").addEventListener("click", (e) => { if (e.target.id === "modal") closeModal(); });
actions.closeModal = closeModal;

export function showErr(e) {
  if (e && e.status === 401) return;
  toast((e && e.message) || String(e), true);
}

export const fmtTime = (s) => {
  if (!s) return "";
  const d = new Date(s);
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
};

export const formData = (form) => Object.fromEntries(new FormData(form).entries());

export const ROLE = { admin: "管理员", teacher: "老师", student: "学生" };
export const STAGE_ST = { todo: ["未开始", ""], doing: ["进行中", "pri"], review: ["待审核", "warn"], done: ["已通过", "ok"], returned: ["已退回", "bad"] };

// ---- PDF：在浏览器中用 pdf.js 逐页提取文字（服务器不需要额外组件） ----
let pdfjs;
async function loadPdfjs() {
  if (!pdfjs) {
    pdfjs = await import("/lib/pdf.min.mjs");
    pdfjs.GlobalWorkerOptions.workerSrc = "/lib/pdf.worker.min.mjs";
  }
  return pdfjs;
}

export async function extractPdf(file, onProgress) {
  const lib = await loadPdfjs();
  const data = new Uint8Array(await file.arrayBuffer());
  const doc = await lib.getDocument({ data, cMapUrl: "/lib/cmaps/", cMapPacked: true, standardFontDataUrl: "/lib/standard_fonts/", isEvalSupported: false }).promise;
  let labels = null;
  try { labels = await doc.getPageLabels(); } catch { labels = null; }
  const pages = [];
  for (let i = 1; i <= doc.numPages; i++) {
    const page = await doc.getPage(i);
    const tc = await page.getTextContent();
    let out = "", lastY = null, lastH = 0;
    for (const it of tc.items) {
      if (!("str" in it)) continue;
      const y = it.transform ? it.transform[5] : null;
      const h = Math.abs(it.height || (it.transform ? it.transform[3] : 10)) || 10;
      if (lastY !== null && y !== null && Math.abs(y - lastY) > Math.max(2, h * 0.4)) {
        const gap = Math.abs(lastY - y);
        if (gap > Math.max(lastH, h) * 1.8) out = out.replace(/\n*$/, "\n\n");
        else if (!out.endsWith("\n")) out += "\n";
      }
      out += it.str;
      if (it.hasEOL) out += "\n";
      if (y !== null) { lastY = y; lastH = h; }
    }
    out = out.replace(/\n{3,}/g, "\n\n");
    pages.push({ page_index: i, page_label: labels ? (labels[i - 1] || "") : "", text: out });
    if (onProgress) onProgress(i, doc.numPages);
    page.cleanup();
  }
  await doc.destroy();
  return pages;
}

export async function fileToText(file) {
  const name = file.name.toLowerCase();
  if (name.endsWith(".pdf")) {
    const pages = await extractPdf(file);
    return pages.map((p) => p.text).join("\n\n");
  }
  return await file.text();
}

// 打开 PDF（用于 OCR）：返回 { numPages, textOf(i), imageOf(i), close() }
export async function openPdf(data) {
  const lib = await loadPdfjs();
  const doc = await lib.getDocument({ data: new Uint8Array(data), cMapUrl: "/lib/cmaps/", cMapPacked: true, standardFontDataUrl: "/lib/standard_fonts/", isEvalSupported: false }).promise;
  let labels = null;
  try { labels = await doc.getPageLabels(); } catch { labels = null; }
  return {
    numPages: doc.numPages,
    label: (i) => (labels ? labels[i - 1] || "" : ""),
    async textOf(i) {
      const page = await doc.getPage(i);
      const tc = await page.getTextContent();
      page.cleanup();
      return tc.items.map((it) => it.str || "").join("");
    },
    // 渲染成图片：长边约 maxSide 像素（扫描件文字识别需要足够清晰）
    async imageOf(i, maxSide = 2000) {
      const page = await doc.getPage(i);
      const v1 = page.getViewport({ scale: 1 });
      const scale = Math.min(3, maxSide / Math.max(v1.width, v1.height));
      const vp = page.getViewport({ scale });
      const c = document.createElement("canvas");
      c.width = Math.round(vp.width); c.height = Math.round(vp.height);
      const g = c.getContext("2d");
      g.fillStyle = "#fff"; g.fillRect(0, 0, c.width, c.height);
      await page.render({ canvasContext: g, viewport: vp }).promise;
      page.cleanup();
      let d = c.toDataURL("image/jpeg", 0.88);
      if (d.length > 6e6) d = c.toDataURL("image/jpeg", 0.7);
      c.width = c.height = 0;
      return d;
    },
    close: () => doc.destroy(),
  };
}

// ---- 后台任务：耗时的 AI 操作放到后台运行，关掉或离开页面也会继续 ----
export const WATCHED = new Set(); // 当前页面正在等待结果的任务（完成时由页面自己显示，不再弹通知）
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export async function waitJob(id) {
  for (;;) {
    let r;
    try { r = await api("GET", `/api/jobs/${id}`); } catch (e) { if (e.status === 404) throw e; await sleep(3000); continue; }
    if (r.job.status === "done") return r.result;
    if (r.job.status === "error") throw new ApiError(400, r.job.error || "处理失败");
    await sleep(1500);
  }
}

// apiBg：像 api() 一样调用，但请求在工作台后台运行。meta.title 显示在“后台任务”列表里，meta.link 是查看结果的页面。
export async function apiBg(method, path, body, meta = {}) {
  const opt = { method, headers: { "X-KY": "1", "X-KY-Async": "1", "X-KY-Title": encodeURIComponent(meta.title || ""), "X-KY-Link": encodeURIComponent(meta.link || location.hash) }, credentials: "same-origin" };
  if (body instanceof FormData) opt.body = body;
  else if (body !== undefined) { opt.body = JSON.stringify(body); opt.headers["Content-Type"] = "application/json"; }
  let r;
  try { r = await fetch(path, opt); } catch { throw new ApiError(0, "无法连接工作台。请确认运行工作台的电脑已开机、程序在运行。"); }
  const data = await r.json().catch(() => null);
  if (!r.ok) throw new ApiError(r.status, (data && data.detail) || `请求失败（${r.status}）`);
  if (!data || !data.job_id) return data; // 服务端不支持后台运行时直接返回结果
  WATCHED.add(data.job_id);
  window.dispatchEvent(new CustomEvent("ky-jobs"));
  if (meta.onStart) meta.onStart(data.job_id);
  try { return await waitJob(data.job_id); } finally { WATCHED.delete(data.job_id); }
}

// 从“后台任务”点“查看结果”跳转过来时，页面用它取得要显示的任务
export function takeOpenJob(label) {
  const j = window.KY_OPEN_JOB;
  if (j && (!label || [].concat(label).includes(j.label))) { window.KY_OPEN_JOB = null; return j; }
  return null;
}
