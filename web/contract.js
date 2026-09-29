// 论文契约：动笔之前先把“要证明什么”写清楚——研究问题卡、核心主张（创新点、证据、什么情况下不成立）、章节地图，
// 再让 AI 以审稿人身份攻击一轮；处理完每条质疑、本人确认后契约锁定，AI 起草可以“按契约写”。
import { $, $$, esc, api, apiBg, toast, modal, closeModal, actions, fmtTime } from "/core.js";

const K = { profiles: [], cur: null, view: null, seq: 0, lib: "" };
const JOB_LABELS = ["生成论文契约", "审稿人攻击", "判定回应"];
const ST = { new: ["未处理", "warn"], answered: ["审稿人已接受", "ok"], limitation: ["写进局限", "pri"], open: ["待解决", ""] };
const SEV = { high: ["严重", "bad"], medium: ["中等", "warn"], low: ["轻微", ""] };
const REVIEW = { requested: ["等待老师审阅", "warn"], approved: ["老师已通过", "ok"], returned: ["老师退回修改", "bad"] };
const DECISION = { request: "请老师审阅", approve: "通过", return: "退回修改", comment: "评论", withdraw: "撤回审阅" };
const VERDICT = { concede: ["让步", "ok"], partial: ["部分成立", "warn"], hold: ["坚持质疑", "bad"] };

export async function renderContract(box, profiles, openJob) {
  K.profiles = profiles;
  const projects = await api("GET", "/api/projects").catch(() => []);
  box.innerHTML = `<div class="card"><h3>论文契约</h3>
    <p class="muted" style="margin-top:0">动笔之前，先把这篇论文<b>要证明什么</b>写清楚：研究问题（变量、方法、边界）、2–3 个核心主张（每个的证据，以及<b>什么情况下不成立</b>）、每一节回答什么问题。然后 AI 会以<b>审稿人</b>的身份攻击一轮。你逐条回应、写进局限或保留为待解决，确认后契约锁定；之后用“AI 起草”时可以选择按契约写，写偏了能拉回来。</p>
    <div class="ctsteps"><span>① 研究问题</span><span>② 核心主张</span><span>③ 章节地图</span><span>④ 审稿人质疑</span><span>⑤ 确认锁定</span><span>→ 按契约起草</span></div>
    <details id="ctNewBox" class="ctnew"><summary><b>＋ 新建论文契约</b></summary>
    <form data-submit="ctPlan" style="margin-top:8px">
      <div class="row"><input name="title" placeholder="题目或简称（可不填）" style="flex:1;min-width:200px">
        <select name="profile" style="width:auto">${profiles.map((x) => `<option value="${x.key}">${esc(x.name)}</option>`).join("")}</select>
        <select name="lang" style="width:auto"><option value="zh">中文</option><option value="en">English</option></select></div>
      <label class="f">你的研究想法（必填，用自己的话：想研究什么问题、打算怎么做、预期或已有的发现，每行一条）</label>
      <textarea name="idea" required style="min-height:110px" placeholder="例如：\n我想研究短视频使用时长会不会让大学生注意力变差。\n准备记录 60 名学生两周的屏幕使用时间，并做注意力测验。\n再做一个两周“关掉推荐流”的干预实验。"></textarea>
      <label class="f">已有的结果和数据（可选）</label>
      <textarea name="results" style="min-height:60px" placeholder="例如：干预组专注时长从 18 分钟增加到 24 分钟。"></textarea>
      <label class="f">参考的论文（可选，最多 20 篇）</label>
      <div class="row"><select id="ctLib" data-change="ctLib" style="width:auto"><option value="">我的论文 + 全组共享</option>${(Array.isArray(projects) ? projects : []).map((x) => `<option value="${esc(x.id)}">项目：${esc(x.name)}</option>`).join("")}</select>
        <input id="ctFind" placeholder="筛选论文…" data-input="ctFind" style="flex:1;min-width:160px"></div>
      <div id="ctMats" class="dfmats muted">加载中…</div>
      <div class="row" style="margin-top:10px"><button class="pri" type="submit">生成契约草稿（含审稿人攻击）</button><span class="muted" id="ctState"></span></div>
    </form></details></div>
    <div id="ctView"></div>
    <div id="ctReviewBox"></div>
    <div class="card"><h3>我的论文契约</h3><div id="ctList" class="muted">加载中…</div></div>`;
  loadMats();
  const [list] = await Promise.all([loadList(), loadReviewList()]);
  if (!list.length && !K.reviewN) $("#ctNewBox").open = true;
  if (window.KY_OPEN_CONTRACT) {
    const id = window.KY_OPEN_CONTRACT; window.KY_OPEN_CONTRACT = null;
    show(await api("GET", `/api/contracts?id=${encodeURIComponent(id)}`));
  } else if (openJob) {
    const r = await api("GET", `/api/jobs/${openJob.id}`);
    if (r.result) show(r.result);
  } else if (K.cur) {
    api("GET", `/api/contracts?id=${K.cur.id}`).then((r) => { if ($("#ctView")) show(r, true); }).catch(() => { K.cur = null; });
  }
}

async function loadMats() {
  K.lib = $("#ctLib") ? $("#ctLib").value : "";
  const ms = await api("GET", K.lib ? `/api/materials?project_id=${encodeURIComponent(K.lib)}` : "/api/materials?scope=all").catch(() => []);
  const box = $("#ctMats");
  if (!box) return;
  const ok = ms.filter((m) => m.status === "ready" || m.status === "partial");
  box.classList.remove("muted");
  box.innerHTML = ok.length ? ok.map((m) => `<label class="dfmat ctmat" data-name="${esc((m.title + " " + (m.author || "")).toLowerCase())}"><input type="checkbox" name="mids" value="${esc(m.id)}"> ${esc(m.title)}</label>`).join("")
    : '<p class="muted">论文库里还没有可用的论文。不选也可以，文献相关的依据会标为需要补充。</p>';
}
actions.ctLib = () => loadMats();
actions.ctFind = (el) => { const q = el.value.trim().toLowerCase(); $$(".ctmat").forEach((l) => l.classList.toggle("hide", !!q && !l.dataset.name.includes(q))); };

async function loadList() {
  const [list, jobs] = await Promise.all([api("GET", "/api/contracts").catch(() => []), api("GET", "/api/jobs").catch(() => ({ jobs: [] }))]);
  const box = $("#ctList");
  if (!box) return list;
  const running = jobs.jobs.filter((j) => JOB_LABELS.includes(j.label) && (j.status === "running" || j.status === "queued"));
  clearTimeout(K.timer);
  if (running.length) K.timer = setTimeout(() => { if ($("#ctList")) loadList(); }, 3000);
  box.classList.remove("muted");
  box.innerHTML = running.map((j) => `<div class="row rsrow"><span class="tag pri">${j.status === "queued" ? "排队中" : "进行中"}</span><b>${esc(j.label)}</b><span class="muted">${esc(j.title)}</span><span class="sp"></span><span class="muted">后台运行，可以离开本页</span></div>`).join("")
    + (list.length ? list.map((c) => `<div class="row rsrow"><a href="#" data-act="ctOpen" data-id="${esc(c.id)}"><b>${esc(c.title)}</b></a><span class="muted">${esc(c.profile_name)}</span>
      ${c.status === "confirmed" ? `<span class="tag ok">已确认 v${c.version}</span>` : `<span class="tag warn">草稿${c.version ? `（上次确认 v${c.version}）` : ""}</span>`}
      ${c.unhandled ? `<span class="tag bad">${c.unhandled} 条质疑未处理</span>` : ""}${c.review_status ? `<span class="tag ${REVIEW[c.review_status][1]}">${REVIEW[c.review_status][0]}${c.reviewer_name ? " · " + esc(c.reviewer_name) : ""}</span>` : ""}<span class="sp"></span><span class="muted">${fmtTime(c.updated_at)}</span></div>`).join("")
    : running.length ? "" : '<p class="muted">还没有论文契约。</p>');
  return list;
}
// 老师：请我审阅的契约
async function loadReviewList() {
  const list = await api("GET", "/api/contracts?review=1").catch(() => []);
  K.reviewN = list.length;
  const box = $("#ctReviewBox");
  if (!box) return;
  box.innerHTML = list.length ? `<div class="card"><h3>请我审阅的论文契约</h3>${list.map((c) => `<div class="row rsrow"><a href="#" data-act="ctOpen" data-id="${esc(c.id)}"><b>${esc(c.title)}</b></a><span class="muted">${esc(c.owner_name)} · ${esc(c.profile_name)} · v${c.version}</span>
    <span class="tag ${(REVIEW[c.review_status] || REVIEW.requested)[1]}">${(REVIEW[c.review_status] || REVIEW.requested)[0]}</span><span class="sp"></span><span class="muted">${fmtTime(c.updated_at)}</span></div>`).join("")}</div>` : "";
}
actions.ctOpen = async (el) => show(await api("GET", `/api/contracts?id=${el.dataset.id}`));

actions.ctPlan = async (f) => {
  const mids = $$("#ctMats input[name=mids]").filter((c) => c.checked).map((c) => c.value);
  if (mids.length > 20) return toast("最多选 20 篇论文", true);
  const my = ++K.seq;
  const title = f.title.value.trim() || f.idea.value.trim().split("\n")[0].slice(0, 30);
  const st = () => $("#ctState");
  st().textContent = "已放到后台：正在梳理研究问题和核心主张，然后审稿人会攻击一轮（通常 30–90 秒）。可以离开本页，完成后右上角“后台任务”会提示。";
  setTimeout(loadList, 300);
  try {
    const r = await apiBg("POST", "/api/contracts/plan", { title: f.title.value, profile: f.profile.value, lang: f.lang.value, idea: f.idea.value, results: f.results.value, material_ids: mids, project_id: K.lib }, { title: "论文契约 · " + title, link: "#/writing/contract" });
    if (!$("#ctView")) return;
    loadList();
    if (K.seq === my) { if (st()) st().textContent = ""; $("#ctNewBox").open = false; show(r); }
    else toast("论文契约已生成：" + title);
  } catch (e) { if (st() && K.seq === my) st().textContent = ""; if ($("#ctList")) loadList(); throw e; }
};

function show(r, quiet) {
  K.cur = r.contract; K.view = r;
  renderView();
  if (!quiet) $("#ctView").scrollIntoView({ behavior: "smooth", block: "start" });
}

// ---------- 显示与编辑 ----------
function evLinks(ids) {
  return (ids || []).map((id) => `<a href="#" class="ev ${id.startsWith("F") ? "f" : "n"}" data-act="ctEv" data-id="${esc(id)}">${esc(id)}</a>`).join("");
}
actions.ctEv = (el) => {
  const c = K.cur, id = el.dataset.id;
  const f = c.frags.find((x) => x.id === id), n = c.notes.find((x) => x.id === id);
  const m = f ? `<h3>${esc(id)} · ${esc(f.title)}</h3><p class="muted">${esc(f.location)}</p><div class="frag">${esc(f.text)}</div><a href="/api/materials/${esc(f.material_id)}/file" target="_blank">打开原文件</a>`
    : n ? `<h3>${esc(id)} · 你提供的内容</h3><div class="frag">${esc(n.text)}</div>` : `<h3>${esc(id)}</h3><p>没有这个编号</p>`;
  modal(m);
};

function renderView() {
  const c = K.cur, v = K.view, box = $("#ctView");
  if (!box || !c) return;
  const own = c.owner_id === window.KY_ME_ID;
  const lock = c.status === "confirmed" || !own, dis = lock ? "disabled" : "";
  const vt = v.var_types, ak = v.attack_kinds;
  const typeOpts = (sel) => Object.entries(vt).map(([k, n]) => `<option value="${k}" ${k === sel ? "selected" : ""}>${n}</option>`).join("");
  const claimsN = c.claims.length;
  box.innerHTML = `<div class="card ctcard ${lock ? "locked" : ""}">
    <div class="row"><input id="ctTitle" class="cttitle" value="${esc(c.title)}" ${dis}><span class="sp"></span>
      ${lock ? `<span class="tag ok">已确认 · v${c.version} · ${fmtTime(c.confirmed_at)}</span>` : `<span class="tag warn">草稿，未确认${c.version ? `（上次确认 v${c.version}）` : ""}</span>`}</div>
    <p class="muted">${own ? "" : `作者：${esc(v.owner_name)} · `}${esc(v.profile_name)} · ${c.lang === "en" ? "English" : "中文"} · ${esc(c.model)}${own && lock ? " · 已锁定：要修改请点下方“解锁修改”，改完需要重新确认" : ""}${own ? "" : " · 你是审阅老师：只能查看和给意见，不能修改契约"}</p>
    ${reviewPanel(c, v, own)}
    ${c.missing.length ? `<div class="msg warnbox"><b>还缺的材料：</b>${c.missing.map((m) => `<div>· ${esc(m)}</div>`).join("")}</div>` : ""}

    <h4 class="cth">① 研究问题</h4>
    <textarea id="ctQ" rows="2" ${dis} placeholder="一句话：研究什么问题">${esc(c.question.text)}</textarea>
    <div class="ctlabel">变量 <span class="muted">（实证研究写清自变量和因变量；理论、综述类可以不填）</span></div>
    <div id="ctVars">${c.question.variables.map((x, i) => `<div class="ctvar" data-i="${i}"><input class="vName" value="${esc(x.name)}" placeholder="变量" ${dis}><select class="vType" ${dis}>${typeOpts(x.type)}</select><input class="vMeasure" value="${esc(x.measure)}" placeholder="怎么测量 / 操作化" ${dis}>${lock ? "" : `<button type="button" class="sm ghost" data-act="ctDelVar" data-i="${i}" title="删除">×</button>`}</div>`).join("")}</div>
    ${lock ? "" : '<button type="button" class="sm" data-act="ctAddVar">＋ 变量</button>'}
    <div class="grid2" style="margin-top:8px"><div><div class="ctlabel">研究方法</div><textarea id="ctMethod" rows="3" ${dis}>${esc(c.question.method)}</textarea></div>
      <div><div class="ctlabel">研究边界 <span class="muted">（研究对象、范围，以及不研究什么）</span></div><textarea id="ctScope" rows="3" ${dis}>${esc(c.question.scope)}</textarea></div></div>

    <h4 class="cth">② 核心主张 <span class="muted">（一篇论文一般 2–3 个创新点；现在 ${claimsN} 个）</span></h4>
    <div id="ctClaims">${c.claims.map((k, i) => `<div class="ctclaim" data-i="${i}">
      <div class="row"><span class="ctid">${esc(k.id)}</span><textarea class="kText" rows="2" ${dis} placeholder="主张">${esc(k.text)}</textarea>${lock ? "" : `<button type="button" class="sm ghost" data-act="ctDelClaim" data-i="${i}" title="删除">×</button>`}</div>
      <div class="ctkgrid"><label>创新在哪里<input class="kNovelty" value="${esc(k.novelty)}" ${dis}></label>
        <label><span>依据编号 <span class="ctevs">${evLinks(k.evidence)}</span></span><input class="kEv" value="${esc(k.evidence.join(", "))}" placeholder="如 N1, F2" ${dis}></label>
        <label class="wide">什么情况下不成立 <span class="muted">（什么结果、什么数据会推翻它）</span><input class="kFalsify" value="${esc(k.falsify)}" ${dis}></label></div></div>`).join("")}</div>
    ${lock || claimsN >= 5 ? "" : '<button type="button" class="sm" data-act="ctAddClaim">＋ 核心主张</button>'}

    <h4 class="cth">③ 章节地图 <span class="muted">（每一节回答什么问题、用什么证据、得出什么结论）</span></h4>
    <div class="tw"><table class="cttable"><thead><tr><th>章节</th><th>回答什么问题</th><th>用什么证据</th><th>得出什么结论</th><th>与前后节的关系</th>${lock ? "" : "<th></th>"}</tr></thead><tbody id="ctSecs">
      ${c.sections.map((s, i) => `<tr data-i="${i}"><td><input class="sName" value="${esc(s.name)}" ${dis}></td><td><textarea class="sQ" rows="2" ${dis}>${esc(s.question)}</textarea></td><td><textarea class="sE" rows="2" ${dis}>${esc(s.evidence)}</textarea></td><td><textarea class="sC" rows="2" ${dis}>${esc(s.conclusion)}</textarea></td><td><textarea class="sL" rows="2" ${dis}>${esc(s.link)}</textarea></td>${lock ? "" : `<td><button type="button" class="sm ghost" data-act="ctDelSec" data-i="${i}" title="删除">×</button></td>`}</tr>`).join("")}
    </tbody></table></div>
    ${lock ? "" : '<button type="button" class="sm" data-act="ctAddSec">＋ 章节</button>'}

    <h4 class="cth">④ 审稿人质疑与待解决问题</h4>
    <div class="msg info ctrule"><b>让步规则：</b>只有你的回应<b>直接回答了质疑的核心</b>，并且<b>给出了证据</b>（具体数据、实验或分析设计、N/F 编号），审稿人才会撤回质疑。坚持、表态、“大家都这么认为”、“以后会补充”都不算。也可以诚实地<b>写进局限</b>，或<b>保留为待解决</b>——这些都会写进契约，起草时如实交代。</div>
    <div id="ctAtks">${c.attacks.map((q) => attackCard(q, ak, lock)).join("") || '<p class="muted">还没有质疑。点“重新发起攻击”。</p>'}</div>

    <details class="ctev"><summary><b>依据</b> <span class="muted">（你的材料 N ${c.notes.length} 条 · 论文片段 F ${c.frags.length} 段）</span></summary>
      ${c.notes.map((n) => `<div class="rditem"><span class="ev n">${esc(n.id)}</span> ${esc(n.text)}</div>`).join("")}
      ${c.frags.map((f) => `<div class="rditem"><a href="#" class="ev f" data-act="ctEv" data-id="${esc(f.id)}">${esc(f.id)}</a> <span class="muted">${esc(f.title)} · ${esc(f.location)}</span></div>`).join("")}
      ${lock ? "" : `<div class="row" style="margin-top:8px"><textarea id="ctNewNote" rows="2" placeholder="补充材料：新的数据、实验细节……（会编为新的 N 编号，回应质疑时可以引用）" style="flex:1"></textarea><button type="button" class="sm" data-act="ctAddNote">添加</button></div>`}
    </details>

    ${!own ? `<div class="row" style="margin-top:12px"><a class="btnlike" href="/api/contracts.md?id=${esc(c.id)}">导出契约文档</a></div>` : `<div class="ctgate">${v.problems.length ? `<div class="msg err"><b>确认前还需要：</b>${v.problems.map((p) => `<div>· ${esc(p)}</div>`).join("")}</div>` : lock ? "" : '<div class="msg ok">可以确认了。确认后契约锁定，AI 起草可以按它来写。</div>'}
      ${v.warnings.length ? `<div class="msg warnbox">${v.warnings.map((p) => `<div>· ${esc(p)}</div>`).join("")}</div>` : ""}
      <div class="row">${lock
        ? `<button class="pri" data-act="ctDraft">按契约起草 →</button>${c.review_status === "requested" ? "" : `<button data-act="ctAskReview">${c.review_status ? "重新请老师审阅" : "请老师审阅"}</button>`}<button data-act="ctUnlock">解锁修改</button>`
        : `<button data-act="ctSave">保存修改</button><button data-act="ctAttack">重新发起攻击</button><button class="pri" data-act="ctConfirm">确认契约并锁定</button>`}
        <a class="btnlike" href="/api/contracts.md?id=${esc(c.id)}">导出契约文档</a><span class="sp"></span><button class="danger sm" data-act="ctDel">删除</button><span class="muted" id="ctBusy"></span></div></div>`}
  </div>`;
}

// 老师审阅：作者看到状态和意见；老师看到意见框
function reviewPanel(c, v, own) {
  const rs = c.reviews || [];
  if (own && !c.review_status && !rs.length) return "";
  const [rn, rc] = REVIEW[c.review_status] || ["", ""];
  const thread = rs.map((x) => `<div class="ctturn ${x.decision === "request" || x.decision === "withdraw" ? "me" : "ai"}"><div class="row"><b>${esc(x.name)}</b><span class="tag ${x.decision === "approve" ? "ok" : x.decision === "return" ? "bad" : ""}">${DECISION[x.decision] || esc(x.decision)}</span><span class="muted">契约 v${x.version} · ${fmtTime(x.at)}</span></div>${x.text ? `<div class="pre">${esc(x.text)}</div>` : ""}</div>`).join("");
  const stale = c.review_status && c.review_ver && c.review_ver !== c.version ? `<div class="muted">审阅请求针对 v${c.review_ver}，契约已更新到 v${c.version}</div>` : "";
  const form = !own && c.review_status ? `<textarea id="ctRvText" rows="3" placeholder="写下你的意见：研究问题是否清楚、主张是否站得住、局限是否诚实……（退回或评论时必填）"></textarea>
    <div class="row"><button class="sm pri" data-act="ctReview" data-d="approve">通过</button><button class="sm" data-act="ctReview" data-d="return">退回修改</button><button class="sm" data-act="ctReview" data-d="comment">仅评论</button></div>` : "";
  return `<div class="ctreview"><div class="row"><b>老师审阅</b>${c.review_status ? `<span class="tag ${rc}">${rn}</span>` : ""}${v.reviewer_name ? `<span class="muted">审阅老师：${esc(v.reviewer_name)}</span>` : ""}</div>${stale}${thread}${form}</div>`;
}
actions.ctAskReview = async () => {
  const ts = await api("GET", "/api/contracts/reviewers");
  if (!ts.length) return toast("组里还没有老师账号，请管理员在“设置 → 成员”里添加", true);
  modal(`<h3>请老师审阅论文契约</h3><p class="muted">老师可以查看这份契约（只读），给出“通过”“退回修改”或评论。老师不能改你的契约。</p>
    <form data-submit="ctAskReviewGo"><label class="f">审阅老师</label><select name="reviewer">${ts.map((t) => `<option value="${t.id}" ${t.id === K.cur.reviewer ? "selected" : ""}>${esc(t.name)}</option>`).join("")}</select>
    <label class="f">想请老师重点看什么（可选）</label><textarea name="note" rows="3" placeholder="例如：第二个主张的证据够不够？"></textarea>
    <div class="row" style="margin-top:12px"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button class="pri" type="submit">发送</button></div></form>`);
};
actions.ctAskReviewGo = async (f) => {
  const r = await api("POST", `/api/contracts/${K.cur.id}/review-request`, { reviewer: Number(f.reviewer.value), note: f.note.value });
  closeModal(); show(r, true); loadList(); toast("已请老师审阅，老师首页的“待我处理”里会看到");
};
actions.ctReview = async (el) => {
  const d = el.dataset.d, text = ($("#ctRvText") || {}).value || "";
  if (d !== "approve" && !text.trim()) return toast("退回或评论时请写下意见", true);
  if (d === "approve" && !confirm("通过这份论文契约？")) return;
  const r = await api("POST", `/api/contracts/${K.cur.id}/review`, { decision: d, text });
  show(r, true); loadReviewList(); toast(d === "approve" ? "已通过" : d === "return" ? "已退回，学生会在首页看到" : "已发送评论");
};

function attackCard(q, ak, lock) {
  const [sn, sc] = ST[q.status] || ST.new, [vn, vc] = SEV[q.severity] || SEV.medium;
  const canAct = !lock && q.status !== "answered";
  return `<div class="ctatk st-${q.status}" id="atk-${esc(q.id)}">
    <div class="row"><span class="ctid">${esc(q.id)}</span><span class="tag">${esc(ak[q.kind] || q.kind)}</span><span class="tag ${vc}">${vn}</span>${q.target ? `<span class="muted">针对 ${esc(q.target)}</span>` : ""}${q.forced ? '<span class="muted">· 因果类主张必问</span>' : ""}<span class="sp"></span><span class="tag ${sc}">${sn}</span></div>
    <div class="ctq">${esc(q.text)}</div>
    ${q.hint ? `<div class="muted">怎样才能回应：${esc(q.hint)}</div>` : ""}
    ${q.thread.map((t) => t.who === "student"
      ? `<div class="ctturn me"><b>你的回应</b><div class="pre">${esc(t.text)}</div></div>`
      : `<div class="ctturn ai"><div class="row"><b>审稿人</b><span class="tag ${(VERDICT[t.verdict] || VERDICT.hold)[1]}">${(VERDICT[t.verdict] || VERDICT.hold)[0]}</span>${(t.evidence || []).length ? `<span class="muted">认可的证据：${t.evidence.map((e) => /^[NF]\d+$/.test(e) ? evLinks([e]) : "“" + esc(e) + "”").join("、")}</span>` : ""}</div><div class="pre">${esc(t.text)}</div>${t.note && !t.text.startsWith(t.note) ? `<div class="ctnote">${esc(t.note)}</div>` : ""}</div>`).join("")}
    ${q.status === "limitation" ? `<div class="ctturn lim"><b>写进局限：</b>${esc(q.limit || q.text)}</div>` : ""}
    ${canAct && (q.status === "new") ? `<textarea class="ctReply" rows="2" placeholder="回应这条质疑：拿出证据（数据、实验设计、N/F 编号）。也可以在这里写局限的表述，再点“写进局限”。"></textarea>
      <div class="row"><button class="sm pri" data-act="ctRebut" data-q="${esc(q.id)}">提交回应</button><button class="sm" data-act="ctResolve" data-s="limitation" data-q="${esc(q.id)}">写进局限</button><button class="sm" data-act="ctResolve" data-s="open" data-q="${esc(q.id)}">保留为待解决</button></div>`
      : canAct ? `<div class="row"><button class="sm ghost" data-act="ctResolve" data-s="new" data-q="${esc(q.id)}">撤销，重新处理</button></div>` : ""}
  </div>`;
}

function collect() {
  const c = K.cur;
  const val = (sel, root = document) => { const e = root.querySelector(sel); return e ? e.value : ""; };
  return {
    title: val("#ctTitle"),
    question: { text: val("#ctQ"), method: val("#ctMethod"), scope: val("#ctScope"),
      variables: $$("#ctVars .ctvar").map((r) => ({ name: val(".vName", r), type: val(".vType", r), measure: val(".vMeasure", r) })) },
    claims: $$("#ctClaims .ctclaim").map((r) => ({ text: val(".kText", r), novelty: val(".kNovelty", r), falsify: val(".kFalsify", r), evidence: val(".kEv", r).split(/[,，、\s]+/).map((x) => x.trim().toUpperCase()).filter(Boolean) })),
    sections: $$("#ctSecs tr").map((r) => ({ name: val(".sName", r), question: val(".sQ", r), evidence: val(".sE", r), conclusion: val(".sC", r), link: val(".sL", r) })),
    _c: c,
  };
}
// 本地改动（加行、删行）先放进 K.cur，不立即保存
function local(fn) {
  const d = collect();
  Object.assign(K.cur, { title: d.title, question: d.question, claims: d.claims.map((k, i) => ({ ...k, id: "K" + (i + 1) })), sections: d.sections });
  fn(K.cur);
  renderView();
}
actions.ctAddVar = () => local((c) => c.question.variables.push({ name: "", type: "other", measure: "" }));
actions.ctDelVar = (el) => local((c) => c.question.variables.splice(Number(el.dataset.i), 1));
actions.ctAddClaim = () => local((c) => c.claims.push({ id: "", text: "", novelty: "", evidence: [], falsify: "" }));
actions.ctDelClaim = (el) => local((c) => c.claims.splice(Number(el.dataset.i), 1));
actions.ctAddSec = () => local((c) => c.sections.push({ name: "", question: "", evidence: "", conclusion: "", link: "" }));
actions.ctDelSec = (el) => local((c) => c.sections.splice(Number(el.dataset.i), 1));

async function save(extra = {}) {
  if (K.cur.status === "confirmed") return;
  const { _c, ...d } = collect();
  const r = await api("PUT", `/api/contracts/${K.cur.id}`, { ...d, ...extra });
  K.cur = r.contract; K.view = r;
}
function busy(t) { const b = $("#ctBusy"); if (b) b.textContent = t || ""; }

actions.ctSave = async () => { await save(); renderView(); toast("已保存"); loadList(); };
actions.ctAddNote = async () => {
  const t = $("#ctNewNote").value.trim();
  if (!t) return toast("先写下要补充的材料", true);
  await save({ add_notes: [t] });
  renderView();
  const det = $(".ctev"); if (det) det.open = true;
  toast("已添加为 " + K.cur.notes[K.cur.notes.length - 1].id);
};
actions.ctAttack = async (el) => {
  await save();
  const id = K.cur.id;
  el.disabled = true; busy("审稿人正在重新审读（后台运行，可以离开本页）…");
  try {
    const r = await apiBg("POST", `/api/contracts/${id}/attack`, {}, { title: "审稿人攻击 · " + K.cur.title, link: "#/writing/contract" });
    if ($("#ctView") && K.cur && K.cur.id === id) { show(r, true); $("#ctAtks").scrollIntoView({ behavior: "smooth" }); toast("审稿人又提出了新的质疑"); }
  } finally { busy(""); const b = $("button[data-act=ctAttack]"); if (b) b.disabled = false; }
};
actions.ctRebut = async (el) => {
  const qid = el.dataset.q, card = $("#atk-" + qid), text = card.querySelector(".ctReply").value.trim();
  if (text.length < 8) return toast("请写下你的回应（至少 8 个字）", true);
  await save();
  const id = K.cur.id;
  el.disabled = true; busy("审稿人正在判断你的回应…");
  try {
    const r = await apiBg("POST", `/api/contracts/${id}/rebut`, { qid, text }, { title: `判定回应 · ${qid}`, link: "#/writing/contract" });
    if ($("#ctView") && K.cur && K.cur.id === id) {
      show(r, true);
      const q = r.contract.attacks.find((x) => x.id === qid), last = q.thread[q.thread.length - 1];
      toast(last.verdict === "concede" ? "审稿人接受了你的回应" : "审稿人没有撤回质疑，看看还缺什么", last.verdict !== "concede");
      const c2 = $("#atk-" + qid); if (c2) c2.scrollIntoView({ behavior: "smooth", block: "center" });
    }
  } finally { busy(""); }
};
actions.ctResolve = async (el) => {
  const qid = el.dataset.q, s = el.dataset.s, card = $("#atk-" + qid);
  const limit = s === "limitation" ? ((card.querySelector(".ctReply") || {}).value || "").trim() : "";
  await save();
  const r = await api("POST", `/api/contracts/${K.cur.id}/resolve`, { qid, status: s, limit });
  show(r, true);
  const c2 = $("#atk-" + qid); if (c2) c2.scrollIntoView({ block: "center" });
};
actions.ctConfirm = async () => {
  await save();
  if (K.view.problems.length) { renderView(); return toast("还有需要处理的地方", true); }
  if (!confirm("确认这份论文契约？\n确认后契约锁定：AI 起草会按它来写，不能超出它。以后要改，需要先解锁，改完再重新确认（版本号 +1）。")) { renderView(); return; }
  const r = await api("POST", `/api/contracts/${K.cur.id}/confirm`);
  show(r, true); toast(`已确认，契约 v${r.contract.version}`); loadList();
};
actions.ctUnlock = async () => {
  if (!confirm("解锁后可以修改契约，改完需要重新确认。已经按旧版本写的草稿不受影响。继续？")) return;
  show(await api("POST", `/api/contracts/${K.cur.id}/unlock`), true); loadList();
};
actions.ctDraft = () => { window.KY_DRAFT_CONTRACT = K.cur.id; location.hash = "#/writing/draft"; };
actions.ctDel = async () => {
  if (!confirm("删除这份论文契约？已经按它写的草稿会保留。")) return;
  await api("DELETE", `/api/contracts?id=${K.cur.id}`);
  K.cur = null; $("#ctView").innerHTML = ""; loadList(); toast("已删除");
};
