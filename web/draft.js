// AI 起草：你的想法和结果 + 论文库里的论文 → 论证骨架（可修改）→ 按写作规范起草 → 逐句校验依据 → 导出
import { $, $$, esc, api, apiBg, toast, modal, closeModal, actions, fmtTime } from "/core.js";

const D = { profiles: [], lib: "", mats: [], cur: null, rule: null, roles: {}, busy: false, seq: 0 };
const JOB_LABELS = ["生成论证骨架", "AI 起草"];
const SKIP = /^(题目|题名|项目名称|Title|关键词|参考文献|References|目录|附录|致谢|Figure legends|Data availability|Code availability|Author contributions|Competing interests)$/;

export async function renderDraft(box, profiles, key, openJob) {
  D.profiles = profiles;
  const [projects, contracts] = await Promise.all([api("GET", "/api/projects").catch(() => []), api("GET", "/api/contracts").catch(() => [])]);
  const plist = Array.isArray(projects) ? projects : projects.list || [];
  const p = profiles.find((x) => x.key === key) || profiles[0];
  box.innerHTML = `<div class="card"><h3>AI 起草</h3>
    <p class="muted" style="margin-top:0">写下你自己的想法和结果，选几篇论文库里的论文，AI 按论文写作规范先帮你搭<b>论证骨架</b>（每个论点、它的作用和依据），你确认或修改后再<b>起草成文</b>。每句话都标出依据：<span class="ev n">N</span> 是你提供的内容，<span class="ev f">F</span> 是论文原文；没有依据的句子会标黄，缺的内容留【需补充】。</p>
    <details class="muted"><summary>产出逻辑与依据</summary>
      <ol style="margin:6px 0 0 18px;padding:0;line-height:1.8">
        <li>定位：论文类型 × 章节 × 语言，选用对应的章节写作规则。</li>
        <li>取证：从所选论文中检索与你的想法最相关的原文片段（F），把你的想法和结果拆成条目（N）。</li>
        <li>论证骨架：按章节的论证步骤列出论点，标明作用（背景 / 缺口 / 问题 / 做法 / 发现 / 支撑 / 边界 / 意义）和依据；研究发现只能来自你的材料，文献背景只能来自论文原文。</li>
        <li>起草：只按确认后的骨架写，逐句标注依据，不新增事实、数据和文献。</li>
        <li>校验：检查引用是否真实存在、有没有无依据的陈述、字数、夸大和空泛用词。</li>
        <li>输出：引用换成 [1] [2]… 并生成参考文献，可以复制或下载 Word。</li>
      </ol>
      <p style="margin:6px 0 0">章节规则参考了 Nature 官方“How to construct a Nature summary paragraph”、nature-skills（Apache-2.0）的 Nature 写作规则（摘要的论证顺序、引言的漏斗结构、结果的逐步升级、讨论的顺序、正文精简），以及中文摘要“目的、方法、结果、结论”的要求，按思路重新编写。</p></details>
    <form data-submit="dfPlan" style="margin-top:10px">
      <div class="row dfct"><label class="inline"><b>按论文契约写</b></label><select id="dfContract" data-change="dfContract" style="width:auto;min-width:220px"><option value="">不使用契约（自由写）</option>${contracts.filter((c) => c.status === "confirmed").map((c) => `<option value="${esc(c.id)}" data-profile="${esc(c.profile)}" data-lang="${esc(c.lang)}">${esc(c.title)} · v${c.version}</option>`).join("")}</select>
        <span class="muted" id="dfCtHint">${contracts.some((c) => c.status === "confirmed") ? "推荐：骨架和成文都不会超出契约" : "还没有已确认的论文契约，可以先到“论文契约”里建一份"}</span></div>
      <div class="row">
        <select name="profile" data-change="dfProfile" style="width:auto">${profiles.map((x) => `<option value="${x.key}" ${x.key === p.key ? "selected" : ""}>${esc(x.name)}</option>`).join("")}</select>
        <select name="section" id="dfSection" style="width:auto"></select>
        <select name="lang" id="dfLang" style="width:auto"><option value="zh">中文</option><option value="en">English</option></select>
      </div>
      <label class="f">你想在这一节表达什么（必填，用自己的话：研究问题、做法、主要发现、观点，每行一条）</label>
      <textarea name="idea" required style="min-height:110px" data-ph="例如：\n短视频使用时间越长，大学生注意力越难集中，但原因不清楚。\n我们认为算法推荐带来的频繁切换是主要原因。\n我们设计了两周的“无推荐流”干预，对照组照常使用。" placeholder="例如：\n短视频使用时间越长，大学生注意力越难集中，但原因不清楚。\n我们认为算法推荐带来的频繁切换是主要原因。\n我们设计了两周的“无推荐流”干预，对照组照常使用。"></textarea>
      <label class="f">你的结果和数据（可选，写得越具体，AI 越不用留空）</label>
      <textarea name="results" style="min-height:70px" placeholder="例如：\n干预组 32 人，对照组 30 人。\n干预后专注时长从 18 分钟增加到 24 分钟（p=0.01）。"></textarea>
      <label class="f">从论文库里选参考的论文（最多 20 篇）</label>
      <div class="row"><select id="dfLib" data-change="dfLib" style="width:auto"><option value="">我的论文 + 全组共享</option>${plist.map((x) => `<option value="${esc(x.id)}">项目：${esc(x.name)}</option>`).join("")}</select>
        <input id="dfFind" placeholder="筛选论文…" data-change="dfFind" style="flex:1;min-width:160px"></div>
      <div id="dfMats" class="dfmats muted">加载中…</div>
      <div class="row" style="margin-top:10px"><button class="pri" type="submit">① 生成论证骨架</button><span class="muted" id="dfState"></span></div>
    </form></div>
    <div id="dfPlanBox"></div><div id="dfDraftBox"></div>
    <div class="card"><h3>我的草稿</h3><div id="dfList" class="muted">加载中…</div></div>`;
  fillSections(p);
  if (window.KY_DRAFT_CONTRACT) {
    const sel = $("#dfContract");
    sel.value = window.KY_DRAFT_CONTRACT; window.KY_DRAFT_CONTRACT = null;
    if (sel.value) actions.dfContract(sel);
  }
  if (window.KY_OPEN_DRAFT) { D.cur = { id: window.KY_OPEN_DRAFT }; window.KY_OPEN_DRAFT = null; }
  await loadMats();
  loadList();
  if (openJob) {
    const r = await api("GET", `/api/jobs/${openJob.id}`);
    if (r.result) show(r.result);
  } else if (D.cur && $("#dfPlanBox")) {
    // 回到本页时显示上次看的草稿（重新读取，后台可能已经更新）
    api("GET", `/api/writing/drafts?id=${D.cur.id}`).then((r) => { if ($("#dfPlanBox")) show(r, true); }).catch(() => {});
  }
}

function fillSections(p) {
  $("#dfSection").innerHTML = p.sections.filter((s) => !SKIP.test(s.name)).map((s) => `<option>${esc(s.name)}</option>`).join("");
  $("#dfLang").value = p.limits.lang === "en" ? "en" : "zh";
}
// 选择论文契约：论文类型和语言跟随契约；想法可以不写
actions.dfContract = (el) => {
  const f = el.form, o = el.selectedOptions[0], on = !!el.value;
  if (on) {
    f.profile.value = o.dataset.profile;
    fillSections(D.profiles.find((x) => x.key === o.dataset.profile) || D.profiles[0]);
    f.lang.value = o.dataset.lang === "en" ? "en" : "zh";
  }
  f.profile.disabled = on; f.lang.disabled = on;
  f.idea.required = !on;
  f.idea.placeholder = on ? "（可选）这一节想特别强调的内容。不写也可以，AI 会按契约里这一节要回答的问题、证据和结论来搭骨架。" : f.idea.dataset.ph;
  $("#dfCtHint").textContent = on ? "按契约写：骨架和成文都不会超出契约；契约里的局限会如实写出" : "推荐：先建论文契约，骨架和成文都不会超出契约";
};
actions.dfProfile = (el) => fillSections(D.profiles.find((x) => x.key === el.value));

async function loadMats() {
  D.lib = $("#dfLib").value;
  const ms = await api("GET", D.lib ? `/api/materials?project_id=${encodeURIComponent(D.lib)}` : "/api/materials?scope=all");
  D.mats = ms.filter((m) => m.status === "ready" || m.status === "partial");
  const box = $("#dfMats");
  box.classList.remove("muted");
  box.innerHTML = D.mats.length ? D.mats.map((m) => `<label class="dfmat" data-name="${esc((m.title + " " + (m.author || "")).toLowerCase())}"><input type="checkbox" name="mids" value="${esc(m.id)}"> ${esc(m.title)}${m.owner_id !== window.KY_ME_ID && !D.lib ? `<span class="muted">（${esc(m.uploader)} 共享）</span>` : ""}</label>`).join("")
    : '<p class="muted">这个论文库里还没有可用的论文。不选论文也可以起草，文献背景部分会留【需补充】。</p>';
}
actions.dfLib = () => loadMats();
actions.dfFind = (el) => { const q = el.value.trim().toLowerCase(); $$(".dfmat").forEach((l) => l.classList.toggle("hide", !!q && !l.dataset.name.includes(q))); };

async function loadList() {
  const [list, jobs] = await Promise.all([api("GET", "/api/writing/drafts"), api("GET", "/api/jobs").catch(() => ({ jobs: [] }))]);
  const box = $("#dfList");
  if (!box) return;
  box.classList.remove("muted");
  const running = jobs.jobs.filter((j) => JOB_LABELS.includes(j.label) && (j.status === "running" || j.status === "queued"));
  clearTimeout(D.listTimer);
  if (running.length) D.listTimer = setTimeout(() => { if ($("#dfList")) loadList(); }, 3000);
  box.innerHTML = running.map((j) => `<div class="row rsrow"><span class="tag pri">${j.status === "queued" ? "排队中" : "进行中"}</span><b>${esc(j.label)}</b><span class="muted">${esc(j.title)}</span><span class="sp"></span><span class="muted">后台运行，可以离开本页</span></div>`).join("") + (list.length ? list.map((d) => `<div class="row rsrow"><a href="#" data-act="dfOpen" data-id="${esc(d.id)}">${esc(d.profile)} · ${esc(d.section)}</a><span class="muted">${esc(d.idea)}</span>
      <span class="tag ${d.stage === "draft" ? "ok" : ""}">${d.stage === "draft" ? "已起草" : "骨架"}</span><span class="muted">${fmtTime(d.updated_at)}</span><span class="sp"></span><button class="sm danger" data-act="dfDel" data-id="${esc(d.id)}">删除</button></div>`).join("")
    : running.length ? "" : '<p class="muted">还没有草稿。</p>');
}
actions.dfOpen = async (el) => { show(await api("GET", `/api/writing/drafts?id=${el.dataset.id}`)); };
actions.dfDel = async (el) => {
  if (!confirm("删除这份草稿？")) return;
  await api("DELETE", `/api/writing/drafts?id=${el.dataset.id}`);
  if (D.cur && D.cur.id === el.dataset.id) { D.cur = null; $("#dfPlanBox").innerHTML = ""; $("#dfDraftBox").innerHTML = ""; }
  loadList();
};

actions.dfPlan = async (f) => {
  const mids = $$("#dfMats input[name=mids]").filter((c) => c.checked).map((c) => c.value);
  if (mids.length > 20) return toast("最多选 20 篇论文", true);
  const my = ++D.seq;
  const ctSel = $("#dfContract"), ctId = ctSel ? ctSel.value : "";
  const title = `${f.section.value} · ${ctId ? "按契约：" + ctSel.selectedOptions[0].textContent : f.idea.value.trim().split("\n")[0].slice(0, 40)}`;
  const st = () => $("#dfState");
  st().textContent = "已放到后台：正在检索论文、搭建论证骨架（通常 20–60 秒）。可以离开本页，或者接着发起另一份起草，完成后右上角“后台任务”会提示。";
  const body = { profile: f.profile.value, section: f.section.value, lang: f.lang.value, idea: f.idea.value, results: f.results.value, material_ids: mids, project_id: D.lib, contract_id: ctId };
  setTimeout(loadList, 300);
  try {
    const r = await apiBg("POST", "/api/writing/plan", body, { title, link: "#/writing/draft" });
    if (!$("#dfPlanBox")) return; // 已离开本页
    loadList();
    if (D.seq === my) { if (st()) st().textContent = ""; show(r); }
    else toast(`论证骨架已生成：${title}，在“我的草稿”里打开`);
  } catch (e) { if (st() && D.seq === my) st().textContent = ""; if ($("#dfList")) loadList(); throw e; }
};

function show(r, quiet) {
  D.cur = r.draft; D.rule = r.rule; D.roles = r.roles; D.contract = r.contract || null;
  D.seq++;
  renderPlan();
  renderDraftText();
  if (!quiet) $("#dfPlanBox").scrollIntoView({ behavior: "smooth", block: "start" });
}

// ---------- 证据显示 ----------
function evChip(id) {
  const d = D.cur;
  if (id.startsWith("F")) {
    const f = d.frags.find((x) => x.id === id);
    return f ? `<a href="#" class="ev f" data-act="dfEv" data-id="${id}" title="${esc(f.title + " · " + f.location)}">${id}</a>` : "";
  }
  const n = d.notes.find((x) => x.id === id);
  return n ? `<a href="#" class="ev n" data-act="dfEv" data-id="${id}" title="${esc(n.text)}">${id}</a>` : "";
}
actions.dfEv = (el) => {
  const d = D.cur, id = el.dataset.id;
  if (id.startsWith("F")) {
    const f = d.frags.find((x) => x.id === id);
    modal(`<h3>${esc(id)} · ${esc(f.title)}</h3><p class="muted">${esc(f.location)}</p><div class="frag">${esc(f.text)}</div><a href="/api/materials/${esc(f.material_id)}/file" target="_blank">打开原文件</a>`);
  } else {
    const n = d.notes.find((x) => x.id === id);
    modal(`<h3>${esc(id)} · 你提供的内容</h3><div class="frag">${esc(n.text)}</div>`);
  }
};

// ---------- 论证骨架 ----------
function renderPlan() {
  const d = D.cur, box = $("#dfPlanBox");
  const roleOpts = (sel) => Object.entries(D.roles).map(([k, v]) => `<option value="${k}" ${k === sel ? "selected" : ""}>${v}</option>`).join("");
  box.innerHTML = `<div class="card"><div class="row"><h3 style="margin:0">论证骨架 · ${esc(d.section)}</h3><span class="sp"></span><span class="muted">${esc(d.model)}</span></div>
    <details><summary class="muted">这一节的论证步骤和写作规则（${esc(D.rule.name)}）</summary>
      <ol style="margin:6px 0 0 18px;padding:0">${D.rule.moves.map((m) => `<li>${esc(m)}</li>`).join("")}</ol>
      ${D.rule.rules.map((x) => `<div class="rditem muted">· ${esc(x)}</div>`).join("")}</details>
    ${D.contract ? `<div class="msg info">按论文契约「${esc(D.contract.title)}」v${d.contract_ver} 搭建：论点不会超出契约。${D.contract.version > d.contract_ver || D.contract.status !== "confirmed" ? `<b>契约已${D.contract.status !== "confirmed" ? "解锁修改" : "更新到 v" + D.contract.version}，建议按新契约重新搭骨架。</b>` : ""}</div>` : ""}
    ${d.advice ? `<p class="rdsum">${esc(d.advice)}</p>` : ""}
    <p class="muted">可以改论点的文字、作用，删掉不需要的，或加上新的；“需要补充”的论点起草时会留【需补充】。</p>
    <div id="dfClaims">${d.claims.map((c, i) => `<div class="dfclaim ${c.status === "needs_evidence" ? "need" : ""}" data-i="${i}">
      <div class="row"><select class="dfRole" style="width:auto">${roleOpts(c.role)}</select><span class="sp"></span>${c.evidence.map(evChip).join(" ")}
        ${c.status === "needs_evidence" ? '<span class="tag warn">需要补充</span>' : '<span class="tag ok">有依据</span>'}<button type="button" class="sm danger" data-act="dfDelClaim" data-i="${i}">删除</button></div>
      <textarea class="dfText" rows="2">${esc(c.text)}</textarea>${c.need ? `<div class="muted">需要：${esc(c.need)}</div>` : ""}</div>`).join("")}</div>
    <div class="row" style="margin-top:6px"><button type="button" class="sm" data-act="dfAddClaim">＋ 加一个论点</button></div>
    ${(d.missing || []).length ? `<div class="msg warnbox"><b>还缺的材料：</b>${d.missing.map((m) => `<div>· ${esc(m)}</div>`).join("")}</div>` : ""}
    <div class="row" style="margin-top:10px"><select id="dfLen" style="width:auto"><option value="short">精简</option><option value="" selected>适中</option><option value="long">充分展开</option></select>
      <button class="pri" data-act="dfWrite">② 按骨架起草</button><span class="muted" id="dfWState"></span></div></div>`;
}

function collectClaims() {
  return $$("#dfClaims .dfclaim").map((el) => {
    const c = D.cur.claims[Number(el.dataset.i)] || { evidence: [], status: "needs_evidence", need: "作者新增的论点，请补充依据" };
    return { ...c, role: el.querySelector(".dfRole").value, text: el.querySelector(".dfText").value };
  });
}
actions.dfDelClaim = (el) => {
  D.cur.claims = collectClaims().filter((_, i) => i !== Number(el.dataset.i));
  renderPlan();
};
actions.dfAddClaim = () => {
  D.cur.claims = [...collectClaims(), { id: "", role: "other", text: "", evidence: [], status: "needs_evidence", need: "作者新增的论点，请补充依据" }];
  renderPlan();
};
actions.dfWrite = async (el) => {
  const claims = collectClaims().filter((c) => c.text.trim());
  if (!claims.length) return toast("骨架是空的", true);
  const id = D.cur.id, my = ++D.seq;
  el.disabled = true;
  $("#dfWState").textContent = "已放到后台起草（通常 20–60 秒），可以离开本页或处理别的草稿。";
  setTimeout(loadList, 300);
  try {
    const r = await apiBg("POST", "/api/writing/draft", { id, claims, length: $("#dfLen").value }, { title: `${D.cur.section} · ${D.cur.idea.split("\n")[0].slice(0, 40)}`, link: "#/writing/draft" });
    if (!$("#dfPlanBox")) return;
    loadList();
    if (D.seq === my && D.cur && D.cur.id === id) {
      D.cur = r.draft; D.rule = r.rule;
      renderPlan();
      renderDraftText();
      $("#dfDraftBox").scrollIntoView({ behavior: "smooth", block: "start" });
    } else toast("起草完成：" + r.draft.section + "，在“我的草稿”里打开");
  } finally { const s = $("#dfWState"); if (s && D.seq === my) s.textContent = ""; const b = $("button[data-act=dfWrite]"); if (b) b.disabled = false; }
};

// ---------- 起草稿 ----------
const LV = { error: "✗", warn: "!", ok: "✓", info: "i" };
function renderDraftText() {
  const d = D.cur, box = $("#dfDraftBox");
  if (d.stage !== "draft") { box.innerHTML = ""; return; }
  const refNum = {};
  (d.ref_order || []).forEach((mid, i) => (refNum[mid] = i + 1));
  box.innerHTML = `<div class="card"><div class="row"><h3 style="margin:0">起草稿 · ${esc(d.section)}</h3><span class="tag warn">AI 辅助起草</span><span class="sp"></span>
      ${d.contract_id ? `<button class="sm" data-act="dfDrift">${d.drift_at ? "重新对照契约检查" : "对照契约检查"}</button>` : ""}
      <button class="sm pri" data-act="dfCopy">复制正文</button><button class="sm" data-act="dfDocx">下载 Word</button></div>
    ${d.confirm ? `<p class="muted" style="margin:4px 0 0">✓ 已在 ${esc(fmtTime(d.confirm.at))} 确认过导出要求。各节都写好后，可以到 <a href="#/writing/full">全文组装</a> 拼成完整论文</p>` : ""}
    <p class="muted">黄色：没有依据的陈述，需要补充出处或删改；橙色：需要你补充的内容。点 <span class="ev n">N</span>/<span class="ev f">F</span> 看依据原文。</p>
    <div class="dftext ${d.lang === "en" ? "en" : ""}">${d.paragraphs.map((ps) => `<p>${ps.map((s) => `<span class="dfs ${s.kind === "gap" ? "gap" : s.flag ? "flag" : ""}" title="${esc(s.flag || "")}">${esc(s.text)}${s.cites.map(evChip).join("")}</span>`).join(d.lang === "en" ? " " : "")}</p>`).join("")}</div>
    ${(d.refs || []).length ? `<h4>参考文献</h4>${d.refs.map((r) => `<div class="rditem">${esc(r)}</div>`).join("")}` : ""}
    <h4>检查</h4>${(d.checks || []).map(checkRow).join("")}
    ${d.contract_id ? `<h4>对照契约检查${d.drift_at ? `<span class="muted" style="font-weight:400"> · 对照契约 v${d.drift_ver} · ${esc(fmtTime(d.drift_at))}</span>` : ""}</h4>
      ${d.drift_at ? (d.drift || []).map(checkRow).join("") : `<p class="muted">这份草稿是按论文契约写的。导出前请点“对照契约检查”，看看有没有超出契约的新主张、把相关写成因果、漏写承认的局限等。</p>`}` : ""}
    ${(d.draft_notes || []).length ? `<h4>给你的提醒</h4>${d.draft_notes.map((n) => `<div class="rditem">· ${esc(n)}</div>`).join("")}` : ""}
    <p class="muted">这是依据你的想法、结果和所选论文生成的起草稿，不是定稿：请逐句核对事实和引用，补全【需补充】，用自己的话修改，并按学校或期刊要求声明使用了 AI。由 ${esc(d.model)} 生成。</p></div>`;
}
function checkRow(it) {
  return `<div class="writem lv-${it.level}"><span class="wrico">${LV[it.level]}</span><div><b>${esc(it.cat)}</b>　${esc(it.msg)}${it.where ? `<div class="muted wrwhere">${esc(it.where)}</div>` : ""}${it.fix ? `<div class="wrfix">→ ${esc(it.fix)}</div>` : ""}</div></div>`;
}
actions.dfDrift = async (el) => {
  const id = D.cur.id, my = D.seq;
  el.disabled = true; el.textContent = "检查中…";
  try {
    const r = await apiBg("POST", "/api/writing/drift", { id }, { title: `对照契约检查 · ${D.cur.section}`, link: "#/writing/draft" });
    if (!$("#dfDraftBox")) return;
    if (D.seq === my && D.cur && D.cur.id === id) { D.cur = r.draft; renderDraftText(); toast("检查完成"); }
    else toast("对照契约检查完成：" + r.draft.section);
  } finally { if (el.isConnected) { el.disabled = false; el.textContent = "重新对照契约检查"; } }
};

// 导出前确认：复制正文或下载 Word 之前逐项勾选
function counts(d) {
  let gaps = 0, flags = 0;
  (d.paragraphs || []).forEach((ps) => ps.forEach((s) => { if (s.kind === "gap") gaps++; else if (s.flag) flags++; }));
  const drift = (d.drift || []).filter((x) => x.level === "warn" || x.level === "error").length;
  return { gaps, flags, drift };
}
function ensureConfirmed(then) {
  const d = D.cur;
  if (d.confirm) return then();
  if (d.contract_id && !d.drift_at) {
    modal(`<h3>导出前先对照契约检查</h3><p>这份草稿是按论文契约写的。导出前请先点“对照契约检查”，确认没有超出契约或写偏的地方。</p><div class="row"><span class="sp"></span><button class="pri" data-act="closeModal">知道了</button></div>`);
    return;
  }
  const c = counts(d);
  const item = (k, t) => `<label class="cfitem"><input type="checkbox" name="${k}"> <span>${t}</span></label>`;
  D.afterConfirm = then;
  modal(`<h3>导出前确认</h3><p class="muted">AI 帮你组织和起草，内容要由你负责。请逐项确认：</p>
    <form data-submit="dfConfirm" class="cflist">
      ${item("facts", "我会逐句核对事实、数据和引用，确认每条参考文献真实存在、说法和原文一致")}
      ${c.gaps ? item("gaps", `还有 <b>${c.gaps}</b> 处【需补充】，我会补全后再使用`) : ""}
      ${c.flags || c.drift ? item("issues", `还有 ${c.flags ? `<b>${c.flags}</b> 句没有依据的陈述（标黄）` : ""}${c.flags && c.drift ? "、" : ""}${c.drift ? `<b>${c.drift}</b> 条对照契约发现的问题` : ""}，我会逐条处理`) : ""}
      ${item("ai", "我会用自己的话修改，并按学校或期刊的要求声明使用了 AI 辅助")}
      <div class="row" style="margin-top:12px"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button class="pri" type="submit">确认并继续</button></div>
    </form>`);
}
actions.dfConfirm = async (f) => {
  const checks = {};
  $$("input[type=checkbox]", f).forEach((x) => (checks[x.name] = x.checked));
  if (Object.values(checks).some((v) => !v)) return toast("请逐项勾选", true);
  const r = await api("POST", "/api/writing/confirm", { id: D.cur.id, checks });
  D.cur = r.draft;
  closeModal();
  renderDraftText();
  const then = D.afterConfirm; D.afterConfirm = null;
  if (then) then();
};
actions.dfDocx = () => ensureConfirmed(() => { location.href = `/api/writing/draft.docx?id=${encodeURIComponent(D.cur.id)}`; });
actions.dfCopy = () => ensureConfirmed(copyText);
async function copyText() {
  const r = await api("GET", `/api/writing/draft.txt?id=${D.cur.id}`);
  const t = r.paragraphs.join("\n\n") + (r.refs.length ? "\n\n" + (D.cur.lang === "en" ? "References" : "参考文献") + "\n" + r.refs.join("\n") : "");
  try { await navigator.clipboard.writeText(t); toast("已复制（引用已换成 [1] [2]…）"); } catch { toast("复制失败，请用“下载 Word”", true); }
};
