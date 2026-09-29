// 写论文：新进组同学的论文向导（选类型 → 看结构和格式要求 → 下载模板 → AI 带列提纲）+ 格式 / 投稿规范检查
import { $, $$, esc, api, apiBg, toast, actions, takeOpenJob } from "/core.js";
import { renderDraft } from "/draft.js";
import { renderContract } from "/contract.js";

const W = { tab: "guide", profiles: null, key: "", check: null };

export async function pageWriting(main, tab) {
  W.tab = ["check", "draft", "contract"].includes(tab) ? tab : "guide";
  W.open = takeOpenJob(["AI 列提纲", "格式检查"]);
  if (!W.profiles) W.profiles = await api("GET", "/api/writing/profiles");
  main.innerHTML = `<h2>写论文</h2>
    <p class="muted">刚进组、第一次写论文？先在“新手向导”看结构和格式要求、下载模板；动笔前用“论文契约”把要证明什么想清楚，并经审稿人视角的质疑；再用“AI 起草”按契约和写作规范起草，每句都标出依据；写完用“格式检查”找出格式和规范问题。</p>
    <div class="tabs" id="wrTabs"><button class="${W.tab === "guide" ? "on" : ""}" data-act="wrTab" data-t="guide">新手向导</button><button class="${W.tab === "contract" ? "on" : ""}" data-act="wrTab" data-t="contract">论文契约</button><button class="${W.tab === "draft" ? "on" : ""}" data-act="wrTab" data-t="draft">AI 起草</button><button class="${W.tab === "check" ? "on" : ""}" data-act="wrTab" data-t="check">格式与投稿规范检查</button></div>
    <div id="wrBody"></div>`;
  render();
}
actions.wrTab = (el) => { W.tab = el.dataset.t; history.replaceState(null, "", "#/writing/" + W.tab); $$("#wrTabs button").forEach((b) => b.classList.toggle("on", b === el)); render(); };

function render() {
  if (W.tab === "check") return renderCheck();
  if (W.tab === "contract") return renderContract($("#wrBody"), W.profiles, takeOpenJob(["生成论文契约", "审稿人攻击", "判定回应"])).catch((e) => ($("#wrBody").innerHTML = `<div class="msg">${esc(e.message)}</div>`));
  if (W.tab === "draft") return renderDraft($("#wrBody"), W.profiles, W.key, takeOpenJob(["生成论证骨架", "AI 起草"])).catch((e) => ($("#wrBody").innerHTML = `<div class="msg">${esc(e.message)}</div>`));
  renderGuide();
  // 从“后台任务”打开的提纲
  const j = W.open && W.open.label === "AI 列提纲" ? W.open : null;
  if (j) {
    W.open = null;
    api("GET", `/api/jobs/${j.id}`).then((r) => {
      W.key = r.result.profile;
      renderGuide();
      $("#wrOutline").innerHTML = "";
      const t = $("form[data-submit=wrOutline] input[name=topic]");
      if (t) t.value = r.result.topic || "";
      showOutline(r.result);
      $("#wrOutline").scrollIntoView({ behavior: "smooth" });
    }).catch((e) => toast(e.message, true));
  }
}
const prof = () => W.profiles.find((p) => p.key === W.key);

// ---------------- 新手向导 ----------------
function renderGuide() {
  const b = $("#wrBody");
  const p = prof();
  b.innerHTML = `<div class="card"><h3>① 你要写哪种论文？</h3>
    <div class="wizgrid">${W.profiles.map((x) => `<button class="wizcard ${x.key === W.key ? "on" : ""}" data-act="wrPick" data-k="${x.key}"><b>${esc(x.name)}</b><span class="muted">${esc(x.for)}</span></button>`).join("")}</div></div>
    ${p ? guideHTML(p) : ""}`;
}
actions.wrPick = (el) => { W.key = el.dataset.k; renderGuide(); $("#wrSteps")?.scrollIntoView({ behavior: "smooth" }); };

function guideHTML(p) {
  const en = p.limits.lang === "en";
  return `<div id="wrSteps"></div>
  <div class="card"><h3>② ${esc(p.name)}：结构和每一节写什么</h3>
    <p class="muted" style="margin-top:0">依据：${esc(p.basis)}。<b>${esc(p.note)}。</b></p>
    <div class="wrsecs">${p.sections.map((s, i) => `<details ${i < 2 ? "open" : ""}><summary><b>${esc(s.name)}</b>${s.must ? ' <span class="tag">必需</span>' : ""}${s.len ? ` <span class="muted">${esc(s.len)}</span>` : ""}</summary>
      <div class="wrsec"><div>${esc(s.what)}</div>${(s.tips || []).map((t) => `<div class="muted">· ${esc(t)}</div>`).join("")}</div></details>`).join("")}</div>
    <h4>排版要求</h4>${p.format.map((f) => `<div class="rditem">· ${esc(f)}</div>`).join("")}
    <h4>交稿前自查</h4>${p.checklist.map((f) => `<label class="inline wrcheck"><input type="checkbox"> ${esc(f)}</label>`).join("")}</div>
  <div class="card"><h3>③ 下载模板</h3>
    <p class="muted" style="margin-top:0">模板已经设好${en ? "页面、字体、标题层级、图表题注和参考文献格式" : "A4 页面、页边距 2.5 cm、宋体小四 1.5 倍行距、标题样式（可自动生成目录）、图题表题位置、三线表和 GB/T 7714 参考文献示例"}。灰色文字是每一节的写作提示，写的时候删掉。</p>
    <div class="row"><a class="btnlike pri" href="/api/writing/template?profile=${esc(p.key)}&format=docx">下载 Word 模板（.docx）</a><a class="btnlike" href="/api/writing/template?profile=${esc(p.key)}&format=tex">下载 LaTeX 模板（.tex）</a></div>
    <p class="muted">LaTeX 模板${en ? "" : "用 XeLaTeX 编译；"}可以在“AI 助手 → 本机智能体”里让它帮你放到论文文件夹并编译。</p></div>
  <div class="card"><h3>④ 让 AI 带你列提纲</h3>
    <p class="muted" style="margin-top:0">AI 不会替你写正文：它按这种论文的结构，给每一节列出要回答的问题、建议的小标题和需要准备的材料。</p>
    <form data-submit="wrOutline">
      <input name="topic" required placeholder="论文题目或研究问题，例如：校园共享单车的潮汐调度优化">
      <textarea name="notes" style="min-height:70px;margin-top:6px" placeholder="已经有的材料和进展（可选）：做了什么实验/调查、有哪些数据、读过哪些文献、老师的要求……"></textarea>
      <div class="row" style="margin-top:6px"><button class="pri" type="submit">生成提纲</button><span class="muted" id="wrOutState"></span></div>
    </form><div id="wrOutline"></div></div>
  <div class="card"><h3>⑤ 写完之后</h3>
    <div class="rditem">· 有了想法和结果后，到“AI 起草”让 AI 按这种论文的规范帮你起草某一节（每句标出依据，缺的地方留空给你补）。</div>
    <div class="rditem">· 到 <a href="#" data-act="wrGoCheck">格式与投稿规范检查</a> 上传 Word 或 LaTeX 文件，自动检查结构、摘要、关键词、图表题注、参考文献和引用是否对应${p.limits.no_identity ? "、有没有身份信息、附录有没有代码" : ""}。</div>
    <div class="rditem">· 在“论文库 → 问答”里核对引用的原文；在“引用核验”里检查参考文献是否真实存在。</div>
    <div class="rditem">· 提交前可以让本机智能体用“提交前诚信检查”技能再过一遍。</div></div>`;
}

actions.wrOutline = async (f) => {
  $("#wrOutState").textContent = "已放到后台生成（通常 10–30 秒），可以离开本页，完成后右上角“后台任务”里可以查看…";
  try {
    const r = await apiBg("POST", "/api/writing/outline", { profile: W.key, topic: f.topic.value, notes: f.notes.value }, { title: f.topic.value, link: "#/writing/guide" });
    if (!$("#wrOutline") || W.key !== r.profile) return;
    showOutline(r);
    $("#wrOutState").textContent = "";
  } catch (e) { if ($("#wrOutState")) $("#wrOutState").textContent = ""; throw e; }
};
function showOutline(r) {
    $("#wrOutline").innerHTML = `${r.sections.map((s) => `<div class="wrout"><h4>${esc(s.name)}</h4>
      ${s.questions.length ? `<div class="muted">这一节要回答：</div>${s.questions.map((q) => `<div class="rditem">· ${esc(q)}</div>`).join("")}` : ""}
      ${s.subheadings.length ? `<div class="muted">建议小标题：</div><div class="rditem">${s.subheadings.map(esc).join(" / ")}</div>` : ""}
      ${s.materials.length ? `<div class="muted">需要准备：</div>${s.materials.map((q) => `<div class="rditem">· ${esc(q)}</div>`).join("")}` : ""}
      ${s.pitfall ? `<div class="rdnote">⚠ ${esc(s.pitfall)}</div>` : ""}</div>`).join("")}
      ${r.next.length ? `<h4>接下来三步</h4>${r.next.map((q, i) => `<div class="rditem">${i + 1}. ${esc(q)}</div>`).join("")}` : ""}
      <p class="muted">由 ${esc(r.model)} 生成，仅供参考。写作时引用的每个观点都要有真实文献。</p>`;
}
actions.wrGoCheck = () => { W.tab = "check"; history.replaceState(null, "", "#/writing/check"); $$("#wrTabs button").forEach((b) => b.classList.toggle("on", b.dataset.t === "check")); render(); };

// ---------------- 格式检查 ----------------
function renderCheck() {
  const b = $("#wrBody");
  b.innerHTML = `<div class="card"><h3>格式与投稿规范检查</h3>
    <p class="muted" style="margin-top:0">上传 Word（.docx）或 LaTeX（.tex）源文件，也可以直接粘贴文字。规则检查在本机完成，不调用 AI；勾选“AI 看内容”时，会把题名、摘要、章节标题和引言、结论的开头发给你的模型，请它指出内容和结构上的问题。</p>
    <form data-submit="wrCheck">
      <div class="row"><span class="muted">论文类型</span><select name="profile" style="width:auto">${W.profiles.map((p) => `<option value="${p.key}" ${p.key === W.key ? "selected" : ""}>${esc(p.name)}</option>`).join("")}</select></div>
      <div class="row" style="margin-top:8px"><input type="file" name="file" accept=".docx,.tex,.txt,.md" style="flex:1"></div>
      <details style="margin-top:6px"><summary class="muted">或者粘贴文字</summary><textarea name="text" style="min-height:120px" placeholder="从摘要一直粘贴到参考文献"></textarea></details>
      <div class="row" style="margin-top:8px"><label class="inline"><input type="checkbox" name="ai" value="1"> AI 看内容（摘要、引言、结构，会调用模型）</label><span class="sp"></span><button class="pri" type="submit">开始检查</button></div>
    </form></div><div id="wrReport"></div>`;
  const j = W.open && W.open.label === "格式检查" ? W.open : null;
  if (j) {
    W.open = null;
    api("GET", `/api/jobs/${j.id}`).then((r) => { W.check = r.result; showReport(W.check); }).catch((e) => toast(e.message, true));
  } else if (W.check) showReport(W.check);
}

actions.wrCheck = async (f) => {
  const fd = new FormData();
  fd.append("profile", f.profile.value);
  if (f.file.files[0]) fd.append("file", f.file.files[0]);
  else if (f.text.value.trim()) fd.append("text", f.text.value);
  else return toast("请选择文件或粘贴文字", true);
  if (f.ai.checked) fd.append("ai", "1");
  W.key = f.profile.value;
  $("#wrReport").innerHTML = `<div class="card muted">正在检查${f.ai.checked ? "（已放到后台，AI 看内容通常需要 10–40 秒；可以离开本页，完成后在右上角“后台任务”里查看）" : ""}…</div>`;
  try {
    const r = await apiBg("POST", "/api/writing/check", fd, { title: f.file.files[0] ? f.file.files[0].name : "粘贴的文字", link: "#/writing/check" });
    W.check = r;
    if ($("#wrReport")) showReport(W.check);
  } catch (e) { if ($("#wrReport")) $("#wrReport").innerHTML = ""; throw e; }
};

const LV = { error: ["需要修改", "bad", "✗"], warn: ["请检查", "warn", "!"], ok: ["通过", "ok", "✓"], info: ["提示", "", "i"] };
function itemHTML(it) {
  return `<div class="writem lv-${it.level}"><span class="wrico">${LV[it.level][2]}</span><div><b>${esc(it.cat)}</b>　${esc(it.msg)}
    ${it.where ? `<div class="muted wrwhere">${esc(it.where)}</div>` : ""}${it.fix ? `<div class="wrfix">→ ${esc(it.fix)}</div>` : ""}</div></div>`;
}
function showReport(r) {
  const box = $("#wrReport");
  if (!box) return;
  const n = (lv) => r.items.filter((i) => i.level === lv).length;
  const order = { error: 0, warn: 1, info: 2, ok: 3 };
  const items = r.items.slice().sort((a, b) => order[a.level] - order[b.level]);
  const s = r.stats;
  box.innerHTML = `<div class="card"><div class="row"><h3 style="margin:0">检查结果${r.name ? "：" + esc(r.name) : ""}</h3><span class="sp"></span>
      <span class="tag bad">需要修改 ${n("error")}</span><span class="tag warn">请检查 ${n("warn")}</span><span class="tag ok">通过 ${n("ok")}</span></div>
    <p class="muted">依据：${esc(r.basis)}。${esc(r.note)}。识别出 ${s.headings} 个标题、${s.refs} 条参考文献、${s.figures || 0} 个图、${s.tables || 0} 个表；正文约 ${s.body_units} ${esc(s.unit)}${s.pages ? `；Word 记录 ${s.pages} 页` : ""}。</p>
    ${items.map(itemHTML).join("")}
    ${(s.heading_list || []).length ? `<details><summary class="muted">识别出的标题（检查是否漏了或认错）</summary>${s.heading_list.map((h) => `<div class="rditem">${esc(h)}</div>`).join("")}</details>` : ""}
    <p class="muted">规则检查是自动识别的，可能有误判；学校和期刊的细则以官方要求为准。图片里的文字、页眉页脚请人工再看一遍。</p></div>
    ${r.ai ? `<div class="card"><h3>AI 看内容</h3>${r.ai.overall ? `<p class="rdsum">${esc(r.ai.overall)}</p>` : ""}${r.ai.items.map(itemHTML).join("")}<p class="muted">由 ${esc(r.ai.model)} 给出，仅供参考。</p></div>` : ""}
    ${r.ai_error ? `<div class="card"><div class="msg">AI 看内容没有完成：${esc(r.ai_error)}</div></div>` : ""}`;
  box.scrollIntoView({ behavior: "smooth", block: "start" });
}
