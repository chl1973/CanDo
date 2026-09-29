// OCR：扫描件 / 图片型 PDF 没有文字时，把缺字的页渲染成图片，逐页交给识图模型识别，再加入资料库。
import { $, esc, api, toast, modal, closeModal, actions, openPdf } from "/core.js";

const O = { stop: false, running: false };

// startOCR 对一份资料做 OCR；完成后调用 done(结果)
export async function startOCR(mat, done) {
  if (O.running) return toast("已经有一个 OCR 在进行", true);
  modal(`<h3>OCR 识别：${esc(mat.title)}</h3><div id="ocrBody" class="muted">正在打开 PDF…</div>`);
  let pdf;
  try {
    const r = await fetch(`/api/materials/${encodeURIComponent(mat.id)}/file`, { credentials: "same-origin" });
    if (!r.ok) throw new Error("无法读取原文件");
    pdf = await openPdf(await r.arrayBuffer());
  } catch (e) { $("#ocrBody").innerHTML = `<div class="msg">${esc(e.message || e)}</div>`; return; }
  // 已经有文字片段的页（文字层或之前 OCR 过的）不再识别
  const done0 = new Set((await api("GET", `/api/materials/${encodeURIComponent(mat.id)}/chunks`).catch(() => [])).map((c) => c.page_index));
  const need = [];
  for (let i = 1; i <= pdf.numPages; i++) if (!done0.has(i)) need.push(i);
  const body = $("#ocrBody");
  if (!body) { pdf.close(); return; }
  if (!need.length) { body.innerHTML = `<p>每一页都已经有文字，不需要 OCR。</p>`; pdf.close(); return; }
  const st = await api("GET", "/api/agent/status").catch(() => ({}));
  body.innerHTML = `<p>共 ${pdf.numPages} 页，其中 <b>${need.length} 页</b>没有文字层（${need.length > 12 ? need.slice(0, 12).join("、") + "…" : need.join("、")}），需要用 AI 识别。</p>
    <p class="muted">使用识图模型：<b>${esc(st.vision || "")}</b>。每页调用一次，大约 5–20 秒；页数多时会产生一定费用，可在“用量与花费”查看。识别出的文字会标注“OCR 识别”，可能有错字，引用前请对照原文件。</p>
    ${st.vision_configured === false ? `<div class="msg">还没有能看图片的模型，请先到 <a href="#/settings">设置 → 我的 AI 模型</a> 添加。</div>` : ""}
    <label class="inline" style="margin:6px 0"><span>主要语言</span><select id="ocrLang" style="width:auto"><option value="">自动</option><option value="zh">中文</option><option value="en">英文</option></select></label>
    <div class="row"><span class="sp"></span><button data-act="closeModal">取消</button><button class="pri" data-act="ocrGo" ${st.vision_configured === false ? "disabled" : ""}>开始识别 ${need.length} 页</button></div>`;
  O.job = { mat, pdf, need, done };
}

actions.ocrGo = async () => {
  const { mat, pdf, need, done } = O.job || {};
  if (!pdf) return;
  const lang = $("#ocrLang").value;
  O.stop = false; O.running = true;
  $("#ocrBody").innerHTML = `<div class="meter"><div id="ocrBar" style="width:0%"></div></div><p id="ocrMsg">准备中…</p>
    <div id="ocrPrev" class="ocrprev"></div><div class="row"><span class="sp"></span><button id="ocrStopBtn" data-act="ocrStop">停止（保留已识别的页）</button></div>`;
  const pages = [];
  let model = "", fails = 0, lastErr = "";
  try {
    for (let k = 0; k < need.length && !O.stop; k++) {
      const i = need[k];
      const msg = $("#ocrMsg");
      if (msg) msg.textContent = `正在识别第 ${i} 页（${k + 1}/${need.length}）…`;
      let text = "";
      try {
        const img = await pdf.imageOf(i);
        const r = await api("POST", "/api/ocr/page", { image: img, project_id: mat.project_id || "", lang });
        text = r.text; model = r.model;
      } catch (e) {
        fails++; lastErr = e.message || String(e);
        if (/没有能看图片|额度|模型未接入/.test(lastErr) || fails >= 3 && pages.length === 0) { O.stop = true; break; }
        continue;
      }
      if (text) pages.push({ page_index: i, page_label: pdf.label(i), text });
      const bar = $("#ocrBar");
      if (bar) bar.style.width = `${((k + 1) / need.length) * 100}%`;
      const prev = $("#ocrPrev");
      if (prev) prev.innerHTML = `<div class="muted">第 ${i} 页识别结果（预览）：</div><div class="frag">${esc(text ? text.slice(0, 600) + (text.length > 600 ? "…" : "") : "（空白页）")}</div>`;
    }
  } finally { O.running = false; pdf.close(); }
  if (!pages.length) {
    $("#ocrBody").innerHTML = `<div class="msg">没有识别出文字${lastErr ? "：" + esc(lastErr) : "。"}</div><div class="row"><span class="sp"></span><button data-act="closeModal">关闭</button></div>`;
    return;
  }
  $("#ocrMsg").textContent = "正在保存到论文库…";
  const r = await api("POST", `/api/materials/${encodeURIComponent(mat.id)}/ocr`, { total_pages: pdf.numPages, pages, model });
  $("#ocrBody").innerHTML = `<p><span class="tag ${r.status === "ready" ? "ok" : r.status === "partial" ? "warn" : "bad"}">${esc(r.status_text)}</span> 已识别并加入 ${r.ocr_pages.length} 页的文字${fails ? `，${fails} 页识别出错（${esc(lastErr)}），可以稍后再点“OCR 识别”补做` : ""}。</p>
    <p class="muted">${esc(r.parse_note || r.error || "")}</p>
    <div class="row"><span class="sp"></span><button class="pri" data-act="closeModal">完成</button></div>`;
  if (done) done(r);
};
actions.ocrStop = (el) => { O.stop = true; el.disabled = true; el.textContent = "正在停止…"; };

// 扫描件（没有文字层）判断：上传、检索下载全文、Zotero 导入后都用它来提示 OCR
export const needsOCR = (m) => m && m.ftype === "pdf" && (m.status === "failed" || m.status === "partial") && /未提取到文字/.test((m.error || "") + (m.parse_note || ""));
const KNOWN = {};
export function rememberMat(m) { KNOWN[m.id] = m; }
actions.ocrStartId = (el) => { const m = KNOWN[el.dataset.id]; if (m) startOCR(m, null); };
