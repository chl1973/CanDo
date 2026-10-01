// 全文组装：把起草、确认过的各节拼成一篇完整论文 → 规则检查 → 逐项确认 → 导出 Word / LaTeX / PDF
// 拼装不用 AI：正文逐字来自各节草稿，参考文献按全文首次引用的顺序统一编号；题目和关键词由作者自己填。
import { $, $$, esc, api, apiBg, toast, modal, closeModal, actions, fmtTime } from "/core.js";

const F = { profiles: [], cur: null, local: false, tex: false, saving: null };
const LV = { error: "✗", warn: "!", ok: "✓", info: "i" };
const checkRow = (it) => `<div class="writem lv-${it.level}"><span class="wrico">${LV[it.level]}</span><div><b>${esc(it.cat)}</b>　${esc(it.msg)}${it.where ? `<div class="muted wrwhere">${esc(it.where)}</div>` : ""}${it.fix ? `<div class="wrfix">→ ${esc(it.fix)}</div>` : ""}</div></div>`;

export async function renderFull(box, profiles) {
  F.profiles = profiles;
  const [contracts, eng] = await Promise.all([api("GET", "/api/contracts").catch(() => []), api("GET", "/api/latex/engine").catch(() => ({}))]);
  F.local = !!eng.local;
  const ok = contracts.filter((c) => c.status === "confirmed");
  box.innerHTML = `<div class="card"><h3>全文组装</h3>
    <p class="muted" style="margin-top:0">把“AI 起草”里写好、确认过的各节拼成一篇完整的论文。<b>拼装不用 AI</b>：正文逐字来自各节草稿，参考文献按全文首次引用的顺序统一编号，题目和关键词由你自己填。导出前由规则检查：必需的章节齐不齐、是不是按同一版契约写的、核心主张的依据有没有写进去、承认的局限有没有交代、还剩多少【需补充】，再用“格式检查”同一套规则查一遍。有标红的问题不能导出，提醒要你逐项确认。</p>
    <form data-submit="fpNew" class="row">
      <select name="contract" data-change="fpNewContract" style="width:auto;min-width:200px"><option value="">不使用契约</option>${ok.map((c) => `<option value="${esc(c.id)}" data-title="${esc(c.title)}">按契约：${esc(c.title)} · v${c.version}</option>`).join("")}</select>
      <select name="profile" style="width:auto">${profiles.map((p) => `<option value="${esc(p.key)}">${esc(p.name)}</option>`).join("")}</select>
      <input name="title" placeholder="论文题目（可以稍后再填）" style="flex:1;min-width:180px">
      <button class="pri" type="submit">新建全文</button></form>
    <p class="muted">${ok.length ? "推荐按论文契约组装：会检查各节是否按同一版契约写、核心主张是否都写进了论文。" : "还没有已确认的论文契约，也可以直接按论文类型组装。"}</p></div>
    <div id="fpEdit"></div>
    <div class="card"><h3>我的全文</h3><div id="fpList" class="muted">加载中…</div></div>`;
  await loadList();
  if (F.cur) open(F.cur.paper.id, true).catch(() => { F.cur = null; });
}

actions.fpNewContract = (el) => {
  const f = el.form, on = !!el.value;
  f.profile.disabled = on;
  if (on && !f.title.value) f.title.value = el.selectedOptions[0].dataset.title || "";
};
actions.fpNew = async (f) => {
  const body = f.contract.value ? { contract_id: f.contract.value, title: f.title.value } : { profile: f.profile.value, title: f.title.value };
  show(await api("POST", "/api/writing/full", body));
  f.title.value = "";
  loadList();
  $("#fpEdit").scrollIntoView({ behavior: "smooth", block: "start" });
};

async function loadList() {
  const list = await api("GET", "/api/writing/full");
  const box = $("#fpList");
  if (!box) return;
  box.classList.remove("muted");
  box.innerHTML = list.length ? list.map((p) => `<div class="row rsrow"><a href="#" data-act="fpOpen" data-id="${esc(p.id)}">${esc(p.title || "（未填题目）")}</a><span class="muted">${esc(p.profile)} · ${p.sections} 节${p.contract ? " · 按契约" : ""}</span><span class="sp"></span><span class="muted">${fmtTime(p.updated_at)}</span><button class="sm danger" data-act="fpDel" data-id="${esc(p.id)}">删除</button></div>`).join("")
    : '<p class="muted">还没有全文。先在“AI 起草”里把各节写好、确认，再回来新建。</p>';
}
async function open(id, quiet) { show(await api("GET", `/api/writing/full?id=${encodeURIComponent(id)}`), quiet); }
actions.fpOpen = (el) => open(el.dataset.id);
actions.fpDel = async (el) => {
  if (!confirm("删除这篇全文？各节草稿不会被删除。")) return;
  await api("DELETE", `/api/writing/full?id=${encodeURIComponent(el.dataset.id)}`);
  if (F.cur && F.cur.paper.id === el.dataset.id) { F.cur = null; $("#fpEdit").innerHTML = ""; }
  loadList();
};

function show(r, quiet) {
  F.cur = r;
  render();
  if (!quiet) $("#fpEdit").scrollIntoView({ behavior: "smooth", block: "start" });
}

function partRow(x) {
  const d = x.draft;
  const opts = [];
  if (!x.must) opts.push(`<option value="__skip" ${x.skip ? "selected" : ""}>不写这一节</option>`);
  if (!x.candidates.length) opts.push(`<option value="" ${!x.skip ? "selected" : ""}>（还没有这一节的草稿）</option>`);
  else if (!x.draft_id && !x.skip) opts.push('<option value="" selected>（请选择草稿）</option>');
  x.candidates.forEach((c) => opts.push(`<option value="${esc(c.id)}" ${c.id === x.draft_id ? "selected" : ""}>${esc(c.label)} · ${esc(fmtTime(c.updated_at))}${c.contract ? " · 契约 " + esc(c.contract) : ""}${c.confirmed ? "" : " · 未确认"}</option>`));
  const tags = d ? [d.confirmed ? '<span class="tag ok">已确认</span>' : '<span class="tag bad">未做导出前确认</span>', d.gaps ? `<span class="tag warn">需补充 ${d.gaps}</span>` : "", d.flags ? `<span class="tag warn">无依据 ${d.flags}</span>` : "", d.drift ? `<span class="tag warn">契约问题 ${d.drift}</span>` : "", d.contract_ver ? `<span class="tag">契约 v${d.contract_ver}</span>` : ""].join("") : "";
  return `<div class="fprow">
    <div class="fpsec"><b>${esc(x.section)}</b>${x.must ? ' <span class="tag">必需</span>' : ""}<div class="muted fpwhat">${esc(x.what || "")}</div></div>
    <div class="fppick"><select data-change="fpPick" data-s="${esc(x.section)}">${opts.join("")}</select>
      <div class="row fptags">${tags}<span class="sp"></span>${d ? `<a href="#" data-act="fpOpenDraft" data-id="${esc(d.id)}">打开草稿</a>` : x.skip ? "" : `<a href="#" data-act="fpGoDraft">去起草</a>`}</div></div></div>`;
}

function render() {
  const r = F.cur, p = r.paper, box = $("#fpEdit");
  if (!box) return;
  const errs = r.checks.filter((c) => c.level === "error");
  const ct = r.contract;
  box.innerHTML = `<div class="card fpcard"><div class="row"><h3 style="margin:0">${esc(p.title || "（未填题目）")}</h3><span class="tag warn">AI 辅助起草</span><span class="sp"></span><span class="muted">${esc(r.profile.name)} · ${p.lang === "en" ? "English" : "中文"}</span></div>
    ${ct ? `<div class="msg info">按论文契约「${esc(ct.title)}」v${ct.version} 组装${ct.status !== "confirmed" ? "：<b>契约已解锁修改，请先重新确认</b>" : ""}</div>` : ""}
    <form data-submit="fpMeta" class="fpmeta">
      <label class="f">题目（由你自己定）</label><input name="title" value="${esc(p.title)}" data-change="fpMetaSave" placeholder="论文题目">
      <label class="f">关键词${r.profile.kw_min ? `（${r.profile.kw_min}–${r.profile.kw_max} 个，` : "（"}用“；”隔开）</label><input name="keywords" value="${esc((p.keywords || []).join("；"))}" data-change="fpMetaSave" placeholder="${p.lang === "en" ? "keyword1; keyword2; keyword3" : "关键词1；关键词2；关键词3"}">
    </form>
    <h4>各节</h4><div class="fprows">${r.parts.map(partRow).join("")}</div>
    <p class="muted">共 ${r.stats.sections} 节、约 ${r.stats.units} ${esc(r.stats.unit)}、${r.stats.refs} 条参考文献${r.stats.gaps ? `，还有 ${r.stats.gaps} 处【需补充】` : ""}。各节的内容要改，请到“AI 起草”里打开那份草稿修改或重新起草，回到这里会自动更新。</p>
    <h4>检查${errs.length ? ` <span class="tag bad">${errs.length} 个问题要先解决</span>` : ""}</h4>
    ${r.checks.map(checkRow).join("")}
    ${r.format.length ? `<details><summary><b>格式检查</b> <span class="muted">（与“格式与投稿规范检查”同一套规则，检查生成的 Word；${r.format.length} 条提醒）</span></summary>${r.format.map(checkRow).join("")}</details>` : ""}
    <details class="fpprev"><summary><b>预览全文</b></summary>${preview(r)}</details>
    <div class="row fpexport">
      ${r.confirmed ? `<span class="muted">✓ 已在 ${esc(fmtTime(p.confirm.at))} 确认导出要求</span>` : p.confirm ? '<span class="muted">内容变了，导出前要重新确认</span>' : ""}
      <span class="sp"></span>
      <button class="pri" data-act="fpDocx" ${errs.length ? "disabled" : ""}>导出 Word</button>
      <button data-act="fpTex" ${errs.length ? "disabled" : ""}>导出 LaTeX</button>
      ${F.local && r.tex_engine ? `<button data-act="fpPdf" ${errs.length ? "disabled" : ""}>编译 PDF</button>` : ""}
    </div>
    ${errs.length ? '<p class="muted" style="text-align:right;margin:4px 0 0">先解决标红的问题才能导出</p>' : ""}
    <p class="muted">这是依据你的想法、结果和所选论文起草、再拼装成的全文，不是定稿：请逐句核对，补全【需补充】，用自己的话修改，并按学校、竞赛或期刊要求声明使用了 AI。</p></div>`;
}

function preview(r) {
  const en = r.paper.lang === "en";
  const sent = (s) => {
    let t = esc(s.text);
    if (s.nums && s.nums.length) {
      const mark = `<sup class="fpcite">[${s.nums.join(",")}]</sup>`;
      t = /[。.；;！!？?]$/.test(s.text) ? t.slice(0, -1) + mark + t.slice(-1) : t + mark;
    }
    return `<span class="dfs ${s.gap ? "gap" : s.flag ? "flag" : ""}">${t}</span>`;
  };
  const kw = (r.paper.keywords || []).length ? `<p><b>${en ? "Keywords: " : "关键词："}</b>${esc(r.paper.keywords.join(en ? "; " : "；"))}</p>` : "";
  let kwDone = false;
  let n = 0;
  const body = r.sections.map((s) => {
    let pre = "";
    if (!s.abstract && !kwDone) { kwDone = true; pre = kw; }
    const head = s.abstract || en ? s.name : `${++n} ${s.name}`;
    const out = `${pre}<h4>${esc(head)}</h4>${s.paras.map((ps) => `<p>${ps.map(sent).join(en ? " " : "")}</p>`).join("")}`;
    if (s.abstract && !kwDone) { kwDone = true; return out + kw; }
    return out;
  }).join("");
  return `<div class="dftext fpdoc ${en ? "en" : ""}"><h3 style="text-align:center">${esc(r.paper.title || "")}</h3>${body}${kwDone ? "" : kw}
    ${r.refs.length ? `<h4>${en ? "References" : "参考文献"}</h4>${r.refs.map((x, i) => `<div class="rditem">[${i + 1}] ${esc(x)}</div>`).join("")}` : ""}</div>`;
}

// ---------- 修改 ----------
async function save(body) {
  const id = F.cur.paper.id;
  const r = await api("PUT", `/api/writing/full/${encodeURIComponent(id)}`, body);
  if (F.cur && F.cur.paper.id === id) { F.cur = r; render(); }
  loadList();
}
actions.fpMetaSave = (el) => {
  const f = el.form;
  return save({ title: f.title.value, keywords: [f.keywords.value] });
};
actions.fpMeta = (f) => save({ title: f.title.value, keywords: [f.keywords.value] });
actions.fpPick = (el) => {
  const parts = F.cur.parts.map((x) => ({ section: x.section, draft_id: x.draft_id, skip: x.skip }));
  const x = parts.find((p) => p.section === el.dataset.s);
  if (el.value === "__skip") { x.skip = true; x.draft_id = ""; } else { x.skip = false; x.draft_id = el.value; }
  return save({ parts });
};
actions.fpOpenDraft = (el) => { window.KY_OPEN_DRAFT = el.dataset.id; location.hash = "#/writing/draft"; };
actions.fpGoDraft = () => {
  if (F.cur.paper.contract_id) window.KY_DRAFT_CONTRACT = F.cur.paper.contract_id;
  location.hash = "#/writing/draft";
};

// ---------- 导出前确认 ----------
function ensureConfirmed(then) {
  const r = F.cur;
  if (r.errors) return toast("先解决标红的问题才能导出", true);
  if (r.confirmed) return then();
  const item = (k, t) => `<label class="cfitem"><input type="checkbox" name="${k}"> <span>${t}</span></label>`;
  F.afterConfirm = then;
  modal(`<h3>导出全文前确认</h3><p class="muted">全文由 AI 辅助起草的各节拼装而成，内容要由你负责。请逐项确认：</p>
    <form data-submit="fpConfirm" class="cflist">
      ${item("facts", "我会通读全文，逐句核对事实、数据和引用，确认每条参考文献真实存在、信息完整")}
      ${r.need.includes("issues") ? item("issues", `检查里还有 <b>${r.warn_count}</b> 条提醒${r.stats.gaps ? `（含 <b>${r.stats.gaps}</b> 处【需补充】）` : ""}，我会逐条处理`) : ""}
      ${item("ai", "我会用自己的话修改，并按学校、竞赛或期刊的要求声明使用了 AI 辅助")}
      <div class="row" style="margin-top:12px"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button class="pri" type="submit">确认并继续</button></div>
    </form>`);
}
actions.fpConfirm = async (f) => {
  const checks = {};
  $$("input[type=checkbox]", f).forEach((x) => (checks[x.name] = x.checked));
  if (Object.values(checks).some((v) => !v)) return toast("请逐项勾选", true);
  F.cur = await api("POST", `/api/writing/full/${encodeURIComponent(F.cur.paper.id)}/confirm`, { checks });
  closeModal();
  render();
  const then = F.afterConfirm; F.afterConfirm = null;
  if (then) then();
};
actions.fpDocx = () => ensureConfirmed(() => { location.href = `/api/writing/full/${encodeURIComponent(F.cur.paper.id)}/docx`; });
actions.fpTex = () => ensureConfirmed(() => { location.href = `/api/writing/full/${encodeURIComponent(F.cur.paper.id)}/tex`; });
actions.fpPdf = (el) => ensureConfirmed(async () => {
  const id = F.cur.paper.id;
  el.disabled = true; el.textContent = "编译中…";
  try {
    const r = await apiBg("POST", "/api/writing/full/pdf", { id }, { title: `编译全文 · ${F.cur.paper.title || ""}`, link: "#/writing/full" });
    const res = r.result;
    if (res.pdf_id) { window.open(`/api/latex/pdf/${encodeURIComponent(res.pdf_id)}`, "_blank"); toast("PDF 已生成（保存 30 分钟，请及时下载）"); }
    else modal(`<h3>编译没有成功</h3>${(res.errors || []).slice(0, 8).map((e) => `<div class="rditem">${e.line ? "第 " + e.line + " 行：" : ""}${esc(e.msg)}${e.hint ? `<div class="muted">${esc(e.hint)}</div>` : ""}</div>`).join("") || `<pre class="pre">${esc(res.log_tail || "")}</pre>`}<p class="muted">可以先导出 LaTeX，用 TeXstudio 等编辑器打开修改。</p><div class="row"><span class="sp"></span><button class="pri" data-act="closeModal">知道了</button></div>`);
  } finally { if (el.isConnected) { el.disabled = false; el.textContent = "编译 PDF"; } }
});
