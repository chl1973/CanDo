// AI 读文献：速读卡、概念讲解（资料库里使用）
import { $, esc, api, toast, modal, actions, fmtTime, apiBg } from "/core.js";
import { CITES } from "/kb.js";
import { loadKatex, renderMath } from "/assistant.js";

const R = { mid: "", title: "", brief: null };

function rdChip(cid) {
  const c = CITES[cid];
  if (!c) return "";
  if (c.deleted) return `<span class="cite del">${esc(c.title)} · 已删除</span>`;
  return `<span class="cite" data-act="rdCite" data-cid="${esc(cid)}" title="点击查看原文">${esc(c.location)}</span>`;
}
const chips = (ids) => (ids || []).map(rdChip).join("");
// 在速读卡里就地展开原文，不离开当前页面
actions.rdCite = (el) => {
  const c = CITES[el.dataset.cid];
  const box = el.closest(".rditem");
  const old = box.querySelector(".frag");
  if (old) { old.remove(); return; }
  const page = c.ftype === "pdf" && c.page_index ? `#page=${c.page_index}` : "";
  box.insertAdjacentHTML("beforeend", `<div class="frag"><span class="muted">${esc(c.title)} v${c.version} · ${esc(c.location)}</span>\n${esc(c.text)}\n<a href="/api/materials/${esc(c.material_id)}/file${page}" target="_blank">打开原文件</a></div>`);
};
const item = (x) => x ? `<div class="rditem">${esc(x.text)} ${chips(x.chunk_ids)}${x.note ? `<div class="rdnote">⚠ ${esc(x.note)}</div>` : ""}</div>` : "";

function head(d) {
  return `<p class="muted" style="margin:0 0 8px">由 ${esc(d.model)} 生成${d.route ? `（${esc(d.route)}）` : ""} · ${fmtTime(d.at)}${d.cached ? ` · ${esc(d.by || "")}之前生成，已直接复用，没有再次调用模型` : ""}</p>`;
}

export async function openBrief(mid, title, refresh = false, effort = "") {
  R.mid = mid;
  R.title = title;
  modal(`<h3>AI 速读：${esc(title)}</h3><p class="muted">${refresh ? "正在重新生成" : "正在读取（已生成过的会直接显示）"}，通常需要 10–40 秒。在工作台后台运行，可以关掉这个窗口，稍后再点“AI 速读”会直接显示结果。</p>`);
  const token = (R.token = (R.token || 0) + 1);
  let d;
  try {
    d = await apiBg("POST", "/api/read/brief", { material_id: mid, refresh, effort }, { title, link: location.hash });
  } catch (e) {
    if (R.token === token && !$("#modal").classList.contains("hide")) modal(`<h3>AI 速读：${esc(title)}</h3><div class="msg">${esc(e.message)}</div>`);
    return;
  }
  if (R.token !== token || $("#modal").classList.contains("hide")) return; // 已关闭或打开了别的内容
  R.brief = d;
  showBrief(d);
}

async function showBrief(d) {
  Object.assign(CITES, d.citations || {});
  const r = d.result;
  const cov = r.coverage || {};
  const cards = await api("GET", `/api/read/cards?material_id=${encodeURIComponent(R.mid)}`).catch(() => []);
  const concepts = cards.filter((c) => c.kind === "concept");
  modal(`<div class="rd">
    <div class="row" style="padding-right:34px"><h3 style="margin:0">AI 速读：${esc(R.title)}</h3><span class="sp"></span>
      <button class="sm" data-act="rdRedo" data-effort="deep" title="用难题模型重新生成（更准确，也更贵）">深度重读</button></div>
    ${head(d)}
    ${cov.picked < cov.total ? `<div class="msg info">资料较长，本次读的是全文 ${cov.total} 段中挑出的 ${cov.picked} 段（开头、各章节开头和均匀抽样）。细节请用“问答”或打开原文核对。</div>` : ""}
    <div class="rdsum">${esc(r.summary)}</div>
    <h4>研究问题</h4>${item(r.question) || '<p class="muted">摘录中没有明确写出</p>'}
    <h4>方法与数据</h4>${item(r.method) || '<p class="muted">摘录中没有明确写出</p>'}
    <h4>主要发现</h4>${(r.findings || []).map(item).join("") || '<p class="muted">没有得到有原文出处的发现</p>'}
    ${(r.limits || []).length ? `<h4>局限与未解决的问题</h4>${r.limits.map(item).join("")}` : ""}
    ${(r.gaps || []).length ? `<p class="muted">摘录中缺少：${esc(r.gaps.join("；"))}</p>` : ""}
    <h4>关键术语 <span class="muted" style="font-weight:normal">（点一下讲解）</span></h4>
    <div>${(r.terms || []).map((t) => `<button class="sm termchip" data-act="rdConcept" data-term="${esc(t.term)}" title="${esc(t.note)}">${esc(t.term)}</button>`).join(" ")}</div>
    <h4>读前需要的知识 <span class="tag">AI 通用知识</span></h4>
    ${(r.prerequisites || []).map((p) => `<div class="rditem"><b>${esc(p.concept)}</b>：${esc(p.why)} <button class="sm" data-act="rdConcept" data-term="${esc(p.concept)}">讲解</button></div>`).join("") || '<p class="muted">无</p>'}
    <form data-submit="rdAsk" class="row" style="margin-top:12px"><input name="term" required maxlength="60" placeholder="还有看不懂的词或概念？输入后点“讲解”" style="flex:1;min-width:180px"><button class="pri" type="submit">讲解</button></form>
    ${concepts.length ? `<h4>已讲解过的概念</h4><div>${concepts.map((c) => `<button class="sm" data-act="rdConcept" data-term="${esc(c.term)}">${esc(c.term)}</button>`).join(" ")}</div>` : ""}
    <p class="muted" style="margin-top:12px">带出处的内容可点击查看原文；没有出处的标了 ⚠。“读前需要的知识”是 AI 的通用知识，不是出自这份资料。</p>
  </div>`);
}

actions.rdRedo = (el) => openBrief(R.mid, R.title, true, el.dataset.effort);
actions.rdAsk = (f) => openConcept(f.term.value.trim());
actions.rdConcept = (el) => openConcept(el.dataset.term);
actions.rdBack = () => (R.brief ? showBrief(R.brief) : null);

export async function openConcept(term, ctx = "", mid = R.mid) {
  if (!term) return;
  if (mid !== R.mid) { R.mid = mid; R.brief = null; }
  modal(`<h3>讲解：${esc(term)}</h3><p class="muted">正在讲解…（在工作台后台运行，关掉窗口也会继续，讲解结果会保存）</p>`);
  const token = (R.token = (R.token || 0) + 1);
  let d;
  try {
    d = await apiBg("POST", "/api/read/concept", { material_id: mid, term, context: ctx }, { title: term, link: location.hash });
    if (R.token !== token || $("#modal").classList.contains("hide")) return;
  } catch (e) {
    if (R.token !== token || $("#modal").classList.contains("hide")) return;
    modal(`<h3>讲解：${esc(term)}</h3><div class="msg">${esc(e.message)}</div>${R.brief ? '<button data-act="rdBack">← 返回速读</button>' : ""}`);
    return;
  }
  Object.assign(CITES, d.citations || {});
  const r = d.result;
  modal(`<div class="rd">
    ${R.brief ? '<a href="#" data-act="rdBack">← 返回速读</a>' : ""}
    <h3 style="margin:6px 0">讲解：${esc(d.term)}</h3>
    ${head(d)}
    <h4>通俗解释 <span class="tag">AI 通用知识</span></h4><div class="rditem">${esc(r.plain)}</div>
    ${r.formula ? `<div id="rdFormula" class="mathprev"></div>` : ""}
    ${r.example ? `<h4>举个例子</h4><div class="rditem">${esc(r.example)}</div>` : ""}
    ${r.confusions ? `<h4>容易误解的地方</h4><div class="rditem">${esc(r.confusions)}</div>` : ""}
    <h4>在这份资料中 <span class="tag ok">有原文出处</span></h4>
    ${r.appears ? "" : '<p class="muted">资料中没有直接出现这个词，以下是相关段落中的说法。</p>'}
    ${(r.in_paper || []).map(item).join("") || '<p class="muted">资料中没有找到相关说法。</p>'}
    ${(r.prerequisites || []).length ? `<h4>需要先懂的知识</h4>${r.prerequisites.map((p) => `<div class="rditem"><b>${esc(p.concept)}</b>：${esc(p.why)} <button class="sm" data-act="rdConcept" data-term="${esc(p.concept)}">讲解</button></div>`).join("")}` : ""}
    ${(r.learn_next || []).length ? `<h4>想深入可以搜</h4><div>${r.learn_next.map((k) => `<span class="tag">${esc(k)}</span>`).join(" ")}</div>` : ""}
    <p class="muted" style="margin-top:12px">${esc(r.note)}</p></div>`);
  if (r.formula) {
    try { await loadKatex(); const el = $("#rdFormula"); if (el) renderMath(el, r.formula.replace(/^\$+|\$+$/g, ""), true); } catch { /* 预览失败不影响 */ }
  }
}

actions.readBrief = (el) => openBrief(el.dataset.id, el.dataset.title);
