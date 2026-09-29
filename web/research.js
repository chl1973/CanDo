// 深度调研：文献筛选（逐篇打分 + 抽取成表格）与调研报告（多轮“检索 → 筛选 → 综合 → 找缺口”）
import { $, $$, esc, api, toast, actions, fmtTime } from "/core.js";

const R = { pid: "", readonly: false, id: "", timer: 0, job: null, filter: 2 };
const DEF_COLS = "研究对象、方法、样本/数据、主要结论";

export async function renderResearch(el, pid, opts = {}) {
  R.pid = pid || ""; R.readonly = !!opts.readonly;
  clearTimeout(R.timer);
  el.innerHTML = `<div class="card"><h3>深度调研</h3>
    <p class="muted" style="margin-top:0">写下研究问题，AI 自动拆检索式、检索公开数据库（OpenAlex / Crossref），逐篇阅读题名和摘要判断相关度、抽取信息；“调研报告”模式还会根据发现的缺口继续检索几轮，最后写出每句都带出处的调研要点。<b>只依据题名和摘要</b>，重要结论请打开原文核对。</p>
    <form data-submit="rsStart">
      <textarea name="question" required maxlength="500" style="min-height:64px" placeholder="例如：短视频平台的算法推荐会不会影响大学生的注意力？有哪些测量方法？"></textarea>
      <div class="row" style="margin-top:8px">
        <label class="inline"><input type="radio" name="mode" value="screen" checked> 文献筛选（快，出表格）</label>
        <label class="inline"><input type="radio" name="mode" value="report"> 调研报告（多轮检索，写要点）</label>
      </div>
      <div class="row" style="margin-top:8px">
        <span class="muted">抽取的列</span><input name="columns" value="${DEF_COLS}" style="flex:1;min-width:220px" title="用顿号或逗号分隔，最多 8 列">
      </div>
      <details style="margin-top:6px"><summary class="muted">更多设置</summary>
        <div class="row" style="margin-top:6px">
          <span class="muted">最多</span><select name="max_papers" style="width:auto"><option>20</option><option selected>40</option><option>60</option><option>100</option></select><span class="muted">篇 / 轮</span>
          <span class="muted">轮数</span><select name="max_rounds" style="width:auto"><option>2</option><option selected>3</option><option>4</option></select>
          <span class="muted">起始年份</span><input name="year_from" type="number" min="1900" max="2100" style="width:90px" placeholder="不限">
        </div>
        <textarea name="queries" style="min-height:54px;margin-top:6px" placeholder="自己指定英文检索式（可选，每行一条；不填则由 AI 生成）"></textarea>
      </details>
      <div class="row" style="margin-top:8px"><button class="pri" type="submit">开始调研</button><span class="muted">筛选 40 篇大约需要 5–8 次模型调用；可以随时停止，已完成的部分会保留。</span></div>
    </form></div>
    <div id="rsJob"></div>
    <div class="card"><h3>调研记录</h3><div id="rsList" class="muted">加载中…</div></div>`;
  await loadList();
  if (R.id) openJob(R.id);
}

async function loadList() {
  const list = await api("GET", `/api/research${R.pid ? `?project_id=${encodeURIComponent(R.pid)}` : ""}`);
  const box = $("#rsList");
  if (!box) return;
  box.classList.remove("muted");
  box.innerHTML = list.length ? list.map((j) => `<div class="row rsrow"><a href="#" data-act="rsOpen" data-id="${esc(j.id)}">${esc(j.question)}</a>
      <span class="tag">${j.mode === "report" ? "调研报告" : "文献筛选"}</span><span class="tag ${{ done: "ok", running: "pri", error: "bad" }[j.status] || ""}">${{ done: "完成", running: "进行中", error: "出错", stopped: "已停止" }[j.status]}</span>
      <span class="muted">${j.papers} 篇 · 相关 ${j.relevant} 篇 · ${fmtTime(j.created_at)}</span><span class="sp"></span><button class="sm danger" data-act="rsDel" data-id="${esc(j.id)}">删除</button></div>`).join("")
    : '<p class="muted">还没有调研记录。</p>';
}

actions.rsStart = async (f) => {
  const cols = f.columns.value.split(/[、,，;；]/).map((x) => x.trim()).filter(Boolean);
  const body = { question: f.question.value, mode: f.mode.value, columns: cols, project_id: R.pid, max_papers: Number(f.max_papers.value), max_rounds: Number(f.max_rounds.value),
    year_from: Number(f.year_from.value) || 0, queries: f.queries.value.split("\n").map((x) => x.trim()).filter(Boolean) };
  const j = await api("POST", "/api/research", body);
  toast("已开始，可以离开这个页面，稍后回来查看");
  R.id = j.id;
  loadList();
  openJob(j.id);
};
actions.rsOpen = (el) => openJob(el.dataset.id);
actions.rsDel = async (el) => {
  if (!confirm("删除这条调研记录？")) return;
  await api("DELETE", `/api/research/${el.dataset.id}`);
  if (R.id === el.dataset.id) { R.id = ""; $("#rsJob").innerHTML = ""; }
  loadList();
};
actions.rsStop = async () => { await api("POST", `/api/research/${R.id}/stop`, {}); toast("会在当前这一步完成后停止"); };
actions.rsFilter = (el) => { R.filter = Number(el.value); renderJob(); };

async function openJob(id) {
  clearTimeout(R.timer);
  R.id = id;
  let j;
  try { j = await api("GET", `/api/research/${id}`); } catch (e) { toast(e.message, true); return; }
  if (R.id !== id || !$("#rsJob")) return;
  const wasRunning = R.job && R.job.id === id && R.job.status === "running";
  R.job = j;
  renderJob();
  if (j.status === "running") R.timer = setTimeout(() => openJob(id), 1500);
  else if (wasRunning) loadList();
}

const SCORE = { 3: ["直接相关", "ok"], 2: ["高度相关", "pri"], 1: ["有点关系", ""], 0: ["无关", "muted"], [-1]: ["未筛选", ""] };

function renderJob() {
  const j = R.job, box = $("#rsJob");
  if (!j || !box) return;
  const ps = j.papers.filter((p) => p.score >= R.filter || (R.filter < 0)).sort((a, b) => b.score - a.score || a.n - b.n);
  const rel = j.papers.filter((p) => p.score >= 2).length;
  const running = j.status === "running";
  box.innerHTML = `<div class="card">
    <div class="row"><h3 style="margin:0">${esc(j.question)}</h3><span class="tag">${j.mode === "report" ? "调研报告" : "文献筛选"}</span>
      ${running ? '<span class="tag pri">进行中…</span><span class="sp"></span><button class="sm" data-act="rsStop">停止</button>' : `<span class="tag ${j.status === "done" ? "ok" : "bad"}">${{ done: "完成", error: "出错", stopped: "已停止" }[j.status]}</span>`}</div>
    ${j.error ? `<div class="msg">${esc(j.error)}</div>` : ""}
    <details ${running ? "open" : ""}><summary class="muted">过程记录（${j.log.length}）</summary><pre class="code rslog">${esc(j.log.slice(-40).join("\n"))}</pre></details>
    ${j.rounds.length > 1 || j.mode === "report" ? `<h4>检索轮次</h4><div class="tw"><table><tr><th>轮</th><th>检索式</th><th>新文献</th><th>新增相关</th><th>决定</th></tr>
      ${j.rounds.map((r) => `<tr><td>${r.n}</td><td>${esc(r.queries.join("；"))}</td><td>${r.found}</td><td>${r.new_relevant}</td><td class="muted">${esc(r.decision)}</td></tr>`).join("")}</table></div>` : ""}
    ${j.report ? reportHTML(j) : ""}
    <h4>文献（共 ${j.papers.length} 篇，相关度 ≥ 2 的 ${rel} 篇）</h4>
    <div class="row"><select data-change="rsFilter" style="width:auto">
        <option value="2" ${R.filter === 2 ? "selected" : ""}>只看高度相关（≥2）</option><option value="1" ${R.filter === 1 ? "selected" : ""}>≥1</option><option value="-1" ${R.filter < 0 ? "selected" : ""}>全部</option></select>
      <span class="sp"></span>
      ${R.readonly ? "" : '<button class="sm pri" data-act="rsSave">把勾选的收入论文库</button>'}
      <button class="sm" data-act="rsCSV">导出表格（CSV）</button><button class="sm" data-act="rsCopyRefs">复制勾选的引用</button>
      ${j.report ? '<button class="sm" data-act="rsMD">下载报告（Markdown）</button>' : ""}</div>
    <div class="tw"><table class="rstable"><tr><th><input type="checkbox" data-change="rsAll" style="width:auto"></th><th>#</th><th>相关度</th><th>文献</th>${j.columns.map((c) => `<th>${esc(c)}</th>`).join("")}<th>理由</th></tr>
    ${ps.map((p) => `<tr id="rp-${p.n}"><td><input type="checkbox" class="rsSel" data-n="${p.n}" style="width:auto" ${p.score >= 2 ? "checked" : ""}></td><td>P${p.n}</td>
      <td><span class="tag ${SCORE[p.score][1]}">${p.score >= 0 ? p.score + " " : ""}${SCORE[p.score][0]}</span></td>
      <td class="rsp">${paperLink(p.paper)}<div class="muted">${esc((p.paper.authors || []).slice(0, 3).join(", "))}${p.paper.venue ? " · " + esc(p.paper.venue) : ""}${p.paper.year ? " · " + p.paper.year : ""}${p.paper.abstract ? "" : ' · <span class="tag">无摘要</span>'}</div></td>
      ${j.columns.map((c) => `<td>${esc((p.fields || {})[c] || "")}</td>`).join("")}<td class="muted">${esc(p.reason || "")}</td></tr>`).join("") || `<tr><td colspan="${5 + j.columns.length}" class="muted">${running ? "正在检索和筛选…" : "没有符合条件的文献"}</td></tr>`}
    </table></div>
    <p class="muted">相关度和抽取的信息由 AI 依据题名和摘要给出，可能有误；“摘要未提及”表示需要看全文。</p></div>`;
}

function paperLink(p) {
  const u = p.doi ? `https://doi.org/${p.doi}` : p.landing_url;
  return u ? `<a href="${esc(u)}" target="_blank" rel="noopener"><b>${esc(p.title)}</b></a>` : `<b>${esc(p.title)}</b>`;
}

function citeChips(cs) { return cs.map((n) => `<a href="#" class="cite" data-act="rsGo" data-n="${n}">P${n}</a>`).join(" "); }

function reportHTML(j) {
  const r = j.report;
  return `<div class="rsreport"><h4>调研要点</h4>
    <p class="rdsum">${esc(r.summary)}</p>
    ${r.sections.map((s) => `<h5>${esc(s.heading)}</h5>${s.points.map((p) => `<div class="rditem">· ${esc(p.text)} ${citeChips(p.cites)}</div>`).join("")}`).join("")}
    ${r.disagreements.length ? `<h5>不一致的结论</h5>${r.disagreements.map((p) => `<div class="rditem">· ${esc(p.text)} ${citeChips(p.cites)}</div>`).join("")}` : ""}
    ${r.gaps.length ? `<h5>现有文献还没回答的问题</h5>${r.gaps.map((g) => `<div class="rditem">· ${esc(g)}</div>`).join("")}` : ""}
    ${r.next_steps.length ? `<h5>下一步建议</h5>${r.next_steps.map((g) => `<div class="rditem">· ${esc(g)}</div>`).join("")}` : ""}
    <p class="muted">由 ${esc(r.model || "")} 综合，只依据下表中相关文献的摘要；没有有效出处的结论已自动去掉。点 P 编号可跳到对应文献。</p></div>`;
}

actions.rsGo = (el) => {
  const row = document.getElementById("rp-" + el.dataset.n);
  if (!row) { R.filter = -1; renderJob(); }
  const r = document.getElementById("rp-" + el.dataset.n);
  if (r) { r.scrollIntoView({ behavior: "smooth", block: "center" }); r.classList.add("flash"); setTimeout(() => r.classList.remove("flash"), 1500); }
};
actions.rsAll = (el) => $$(".rsSel").forEach((c) => (c.checked = el.checked));

function selected() {
  const ns = new Set($$(".rsSel").filter((c) => c.checked).map((c) => Number(c.dataset.n)));
  return R.job.papers.filter((p) => ns.has(p.n));
}

actions.rsSave = async (el) => {
  const ps = selected();
  if (!ps.length) return toast("先勾选要收入的文献", true);
  if (!confirm(`把 ${ps.length} 篇文献的题录和摘要收入论文库？\n（需要全文时，可到“检索添加”里下载可免费获取的全文，或自己上传 PDF）`)) return;
  el.disabled = true;
  let ok = 0;
  for (const p of ps) {
    try { await api("POST", "/api/papers/save-record", { paper: p.paper, project_id: R.pid }); ok++; } catch (e) { toast(e.message, true); }
  }
  el.disabled = false;
  toast(`已收入 ${ok} 篇`);
};

function csvCell(s) { s = String(s ?? ""); return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s; }
function download(name, text, type) {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = document.createElement("a");
  a.href = url; a.download = name;
  document.body.appendChild(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}
actions.rsCSV = () => {
  const j = R.job;
  const head = ["编号", "相关度", "题名", "作者", "年份", "期刊", "DOI", ...j.columns, "理由", "GB/T 7714"];
  const rows = j.papers.slice().sort((a, b) => b.score - a.score).map((p) => ["P" + p.n, p.score, p.paper.title, (p.paper.authors || []).join("; "), p.paper.year || "", p.paper.venue || "", p.paper.doi || "",
    ...j.columns.map((c) => (p.fields || {})[c] || ""), p.reason || "", p.paper.gbt || ""]);
  download("文献筛选.csv", "﻿" + [head, ...rows].map((r) => r.map(csvCell).join(",")).join("\r\n"), "text/csv;charset=utf-8");
};
actions.rsCopyRefs = async () => {
  const t = selected().map((p, i) => `[${i + 1}] ${p.paper.gbt}`).join("\n");
  if (!t) return toast("先勾选文献", true);
  try { await navigator.clipboard.writeText(t); toast("已复制 GB/T 7714 引用"); } catch { toast("复制失败，请手动选择", true); }
};
actions.rsMD = () => {
  const j = R.job, r = j.report;
  const cited = new Set();
  const pts = (arr) => arr.map((p) => { p.cites.forEach((n) => cited.add(n)); return `- ${p.text} ${p.cites.map((n) => `[P${n}]`).join("")}`; }).join("\n");
  let md = `# 调研：${j.question}\n\n> 由 CanDo 可为生成，只依据文献题名和摘要，重要结论请核对原文。\n\n**总体结论：** ${r.summary}\n\n`;
  for (const s of r.sections) md += `## ${s.heading}\n\n${pts(s.points)}\n\n`;
  if (r.disagreements.length) md += `## 不一致的结论\n\n${pts(r.disagreements)}\n\n`;
  if (r.gaps.length) md += `## 还没回答的问题\n\n${r.gaps.map((g) => "- " + g).join("\n")}\n\n`;
  if (r.next_steps.length) md += `## 下一步建议\n\n${r.next_steps.map((g) => "- " + g).join("\n")}\n\n`;
  md += `## 参考文献\n\n` + j.papers.filter((p) => cited.has(p.n)).sort((a, b) => a.n - b.n).map((p) => `[P${p.n}] ${p.paper.gbt}`).join("\n") + "\n";
  download("调研报告.md", md, "text/markdown;charset=utf-8");
};
