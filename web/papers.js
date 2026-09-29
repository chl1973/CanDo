// 文献检索：AI 拆关键词、OpenAlex 检索、题录导入、一键收入资料库、GB/T 7714 引用、检索留痕
import { $, $$, esc, api, toast, modal, actions, fmtTime, extractPdf } from "/core.js";
import { renderZotero, saveToZotero } from "/zotero.js";
import { startOCR, needsOCR } from "/ocr.js";

const CTX = { pid: "", el: null, results: [], logId: "", lastQuery: null, readonly: false, mode: "online" };

const EXT = [
  ["知网", "https://kns.cnki.net/kns8s/defaultresult/index?kw="],
  ["万方", "https://s.wanfangdata.com.cn/paper?q="],
  ["百度学术", "https://xueshu.baidu.com/s?wd="],
];

export function renderPapers(el, pid, opts = {}) {
  CTX.pid = pid || "";
  CTX.el = el;
  CTX.readonly = !!opts.readonly;
  CTX.results = [];
  CTX.logId = "";
  el.innerHTML = `
  <div class="tabs sub"><button class="${CTX.mode !== "zotero" ? "on" : ""}" data-act="ppMode" data-m="online">在线检索</button><button class="${CTX.mode === "zotero" ? "on" : ""}" data-act="ppMode" data-m="zotero">从 Zotero 导入</button></div>
  <div id="ppMain"></div>
  <div class="card"><div class="row"><h3 style="margin:0">检索记录</h3><span class="sp"></span><button class="sm" data-act="ppLogs">刷新</button></div>
    <p class="muted">每次检索、从 Zotero 导入的来源和条件、结果数和收入的文献都会自动记录${CTX.pid ? "，项目成员和老师都能看到，方便写文献综述的检索策略" : ""}。</p><div id="ppLogs"></div></div>`;
  renderMode();
  loadLogs();
}

function renderMode() {
  const box = $("#ppMain");
  if (CTX.mode === "zotero") {
    renderZotero(box, CTX.pid, { readonly: CTX.readonly, onChange: loadLogs }).catch((e) => { box.innerHTML = `<div class="card"><div class="msg">${esc(e.message || e)}</div></div>`; });
    return;
  }
  const y = new Date().getFullYear();
  box.innerHTML = `
  <div class="card"><h3>① 先把研究问题拆成检索词（可选）</h3>
    <form data-submit="ppKeywords"><textarea name="question" style="min-height:60px" placeholder="用一两句话写下你的研究问题，例如：短视频平台算法推荐对大学生注意力的影响"></textarea>
    <div class="row" style="margin-top:8px"><button type="submit">AI 帮我拆关键词</button><span class="muted" id="ppKwState"></span></div></form>
    <div id="ppKw"></div></div>
  <div class="card"><h3>② 检索文献</h3>
    <form data-submit="ppSearch">
      <div class="row"><input name="query" id="ppQuery" required placeholder="输入中文或英文检索词，例如：algorithmic recommendation attention" style="flex:1;min-width:220px"><button class="pri" type="submit">检索</button></div>
      <div class="row" style="margin-top:8px">
        <span class="muted">年份</span><input name="year_from" type="number" min="1900" max="${y}" placeholder="起" style="width:90px"><span class="muted">—</span><input name="year_to" type="number" min="1900" max="${y}" placeholder="止" style="width:90px">
        <select name="sort" style="width:auto"><option value="">按相关度</option><option value="cited">按被引次数</option><option value="new">按发表时间</option></select>
        <label class="muted"><input type="checkbox" name="oa_only">只看可免费下载全文</label></div>
    </form>
    <p class="muted" style="margin-top:8px" id="ppQuota"></p>
    <p class="muted" style="margin-top:8px">检索来源：公开学术数据库 OpenAlex（覆盖大部分英文期刊和部分中文期刊，只发送检索词；额度用完时自动改用 Crossref）。中文文献更全的数据库需要学校账号，可以点下面的按钮到对应网站检索，再把导出的题录导入进来：</p>
    <div class="row">${EXT.map(([n]) => `<button class="sm" data-act="ppExt" data-n="${n}">去${n}检索 ↗</button>`).join("")}
      <label class="muted" style="cursor:pointer">导入题录文件（RIS / EndNote / NoteExpress，UTF-8）：<input type="file" accept=".ris,.txt,.enw,.net,.bib" data-change="ppImport" style="width:auto"></label></div>
  </div>
  <div id="ppRes"></div>`;
  CTX.results = [];
  showQuota();
}

async function showQuota() {
  const el = $("#ppQuota");
  if (!el) return;
  const st = await api("GET", "/api/scholar/status").catch(() => null);
  if (!st || !$("#ppQuota")) return;
  const who = { mine: "你的 OpenAlex 密钥", team: "团队的 OpenAlex 密钥", none: "没有填 OpenAlex 密钥（免费额度很少，同一校园网的人共用）" }[st.using];
  let t = `检索额度：使用${who}`;
  if (st.exhausted) t += `；今日额度已用完，${st.reset} 重置，期间自动改用 Crossref`;
  else if (st.searches_left !== undefined) t += `；今日约还能检索 ${st.searches_left} 次`;
  el.innerHTML = esc(t) + (st.using === "none" || st.exhausted ? ' · <a href="#/settings">免费申请密钥（30 秒）→</a>' : "");
}

// 设置页：文献数据库
export async function renderScholarSettings(el) {
  const st = await api("GET", "/api/scholar/status");
  const admin = st.contact_email !== undefined;
  el.innerHTML = `<h3>文献数据库（OpenAlex）</h3>
    <p class="muted">“文献检索”和引用核验的联网核对使用公开数据库 OpenAlex。它从 2026 年起按每日额度限制：<b>不填密钥时每天只能检索约 100 次，而且同一个校园网的人共用</b>，很快就会出现“检索太频繁”。
      填一个<b>免费密钥</b>后每天约 1000 次。额度用完时工作台会自动改用 Crossref（没有摘要的较多）。</p>
    <div class="msg ${st.exhausted ? "" : "info"}">现在使用：${{ mine: "我的密钥", team: "团队密钥（老师已填写）", none: "没有密钥" }[st.using]}${st.exhausted ? `；今日额度已用完，${esc(st.reset)} 重置` : st.searches_left !== undefined ? `；今日约还能检索 ${st.searches_left} 次` : ""}</div>
    <ol class="muted steps3"><li>打开 <a href="https://openalex.org/" target="_blank" rel="noopener">openalex.org</a>，右上角注册/登录（邮箱即可，约 30 秒）；</li>
      <li>打开 <a href="https://openalex.org/settings/api" target="_blank" rel="noopener">openalex.org/settings/api</a>，复制 API key；</li><li>粘贴到下面保存。</li></ol>
    <form data-submit="oaKeySave" data-scope="me" class="row"><input name="key" autocomplete="off" placeholder="${st.my_key ? "已填写（留空并保存 = 删除）" : "粘贴我的 OpenAlex 密钥"}" style="flex:1;min-width:200px"><button type="submit">${st.my_key ? "更新" : "保存"}我的密钥</button></form>
    ${admin ? `<h4 style="margin-top:14px">团队设置（管理员）</h4>
      <form data-submit="oaKeySave" data-scope="team" class="row"><input name="key" autocomplete="off" placeholder="${st.team_key ? "团队密钥已填写（留空并保存 = 删除）" : "团队 OpenAlex 密钥：全组共用，学生不用自己申请"}" style="flex:1;min-width:200px"><button type="submit">保存团队密钥</button></form>
      <form data-submit="oaMailSave" class="row" style="margin-top:8px"><input name="email" type="email" value="${esc(st.contact_email || "")}" placeholder="联系邮箱（可选，用于 Crossref 礼貌访问，速度更稳定）" style="flex:1;min-width:200px"><button type="submit">保存邮箱</button></form>` : ""}`;
  el.dataset.ready = "1";
}
actions.oaKeySave = async (f) => {
  await api("PUT", "/api/scholar/key", { scope: f.dataset.scope, key: f.key.value.trim() });
  toast(f.key.value.trim() ? "密钥有效，已保存" : "已删除密钥");
  renderScholarSettings(f.closest(".card"));
};
actions.oaMailSave = async (f) => {
  await api("PUT", "/api/scholar/key", { scope: "team", contact_email: f.email.value.trim() });
  toast("已保存");
};
actions.ppMode = (el) => {
  CTX.mode = el.dataset.m;
  $$(".tabs.sub button").forEach((b) => b.classList.toggle("on", b === el));
  renderMode();
};

// ---------- 关键词 ----------
actions.ppKeywords = async (f) => {
  const q = f.question.value.trim();
  if (!q) return toast("请先写下研究问题", true);
  const st = $("#ppKwState");
  st.textContent = "AI 思考中…";
  try {
    const d = await api("POST", "/api/papers/keywords", { question: q, project_id: CTX.pid });
    $("#ppKw").innerHTML = `<div style="margin-top:10px">
      ${d.keywords_zh.length ? `<div><b>中文关键词：</b>${d.keywords_zh.map((k) => `<span class="tag">${esc(k)}</span>`).join(" ")}</div>` : ""}
      ${d.keywords_en.length ? `<div style="margin-top:4px"><b>英文关键词：</b>${d.keywords_en.map((k) => `<span class="tag">${esc(k)}</span>`).join(" ")}</div>` : ""}
      ${d.queries.length ? `<div style="margin-top:8px"><b>建议检索式（点一下填入检索框）：</b>${d.queries.map((x) => `<div class="row" style="margin-top:4px"><button class="sm" data-act="ppUseQ" data-q="${esc(x.query)}">${x.lang === "zh" ? "中" : "英"}</button><code>${esc(x.query)}</code><span class="muted">${esc(x.note)}</span></div>`).join("")}</div>` : ""}
      ${d.tips ? `<p class="muted">${esc(d.tips)}</p>` : ""}
      <div class="muted">${esc(d.note)} · 由 ${esc(d.model)} 生成</div></div>`;
  } finally { st.textContent = ""; }
};
actions.ppUseQ = (el) => { $("#ppQuery").value = el.dataset.q; $("#ppQuery").focus(); };
actions.ppExt = (el) => {
  const q = $("#ppQuery").value.trim();
  if (!q) return toast("请先在检索框输入检索词", true);
  const u = EXT.find((x) => x[0] === el.dataset.n)[1] + encodeURIComponent(q);
  if (window.KyApp && window.KyApp.openExternal) window.KyApp.openExternal(u);
  else window.open(u, "_blank", "noopener");
};

// ---------- 检索 ----------
actions.ppSearch = async (f) => {
  const page = 1;
  const d = Object.fromEntries(new FormData(f).entries());
  const q = { query: d.query.trim(), year_from: Number(d.year_from) || 0, year_to: Number(d.year_to) || 0, sort: d.sort, oa_only: d.oa_only === "on", page, project_id: CTX.pid };
  CTX.lastQuery = q;
  await doSearch(q);
};
async function doSearch(q) {
  const box = $("#ppRes");
  box.innerHTML = `<div class="card muted">检索中…</div>`;
  try {
    const r = await api("POST", "/api/papers/search", q);
    showResults(r, q);
    showQuota();
  } catch (e) {
    box.innerHTML = `<div class="card"><div class="msg">检索失败：${esc(e.message || e)}</div><p class="muted">如果运行工作台的电脑不能上网，或学校网络屏蔽了 OpenAlex，可以改用知网/万方检索后导入题录文件。</p>
      <p><a href="#/settings">设置 → 文献数据库：填一个免费的 OpenAlex 密钥（每天约 1000 次检索）</a></p></div>`;
    throw e;
  }
  loadLogs();
}
actions.ppPage = (el) => { if (CTX.lastQuery) doSearch({ ...CTX.lastQuery, page: Number(el.dataset.p) }); };

actions.ppImport = async (el) => {
  const file = el.files[0];
  if (!file) return;
  const fd = new FormData();
  fd.append("file", file);
  if (CTX.pid) fd.append("project_id", CTX.pid);
  try {
    const r = await api("POST", "/api/papers/import", fd);
    CTX.lastQuery = null;
    showResults(r, null);
    toast(`识别出 ${r.total} 条题录`);
    loadLogs();
  } finally { el.value = ""; }
};

function showResults(r, q) {
  CTX.results = r.results;
  CTX.logId = r.log_id || (q && q.page > 1 ? CTX.logId : "");
  const page = r.page || 1;
  const pages = q ? Math.min(Math.ceil(r.total / 20), 10) : 1;
  $("#ppRes").innerHTML = `<div class="card">
    <div class="row"><h3 style="margin:0">结果</h3><span class="muted">来源：${esc(r.source)} · 共 ${r.total} 条${q ? `，第 ${page} 页` : ""}</span><span class="sp"></span>
      <button class="sm" data-act="ppSelAll">全选</button><button class="sm" data-act="ppCopySel">复制选中的参考文献</button><button class="sm" data-act="ppZotSel">选中的存到 Zotero</button></div>
    ${r.notice ? `<div class="msg ${r.cached ? "info" : ""}" style="margin-top:8px">${esc(r.notice)}${/密钥/.test(r.notice) ? ' <a href="#/settings" data-act="closeModal">去填密钥 →</a>' : ""}</div>` : ""}
    ${r.results.length ? r.results.map((p, i) => card(p, i)).join("") : '<p class="muted">没有找到结果。可以换个说法、用英文检索，或放宽年份条件。</p>'}
    ${pages > 1 ? `<div class="row" style="margin-top:10px">${page > 1 ? `<button class="sm" data-act="ppPage" data-p="${page - 1}">上一页</button>` : ""}<span class="muted">第 ${page} / ${pages} 页</span>${page < pages ? `<button class="sm" data-act="ppPage" data-p="${page + 1}">下一页</button>` : ""}</div>` : ""}
    <p class="muted" style="margin-top:8px">参考文献格式按 GB/T 7714—2015 自动生成，数据库中的信息可能不全（如卷期页码），投稿前请核对。</p></div>`;
}

function card(p, i) {
  const link = p.doi ? `https://doi.org/${p.doi}` : p.landing_url;
  const au = p.authors.length > 6 ? p.authors.slice(0, 6).join(", ") + " 等" : p.authors.join(", ");
  return `<div class="paper" style="border-top:1px solid var(--line);padding:10px 0">
    <div class="row" style="align-items:flex-start"><input type="checkbox" class="ppSel" data-i="${i}" style="width:auto;margin-top:4px">
      <div style="flex:1;min-width:0">
        <div><b>${link ? `<a href="${esc(link)}" target="_blank" rel="noopener">${esc(p.title)}</a>` : esc(p.title)}</b></div>
        <div class="muted">${esc(au || "作者不详")}${p.venue ? ` · <i>${esc(p.venue)}</i>` : ""}${p.year ? ` · ${p.year}` : ""}${p.cited ? ` · 被引 ${p.cited}` : ""}
          ${p.is_oa && p.pdf_url ? ' <span class="tag ok">可下载全文</span>' : ""}${p.doi ? ` · DOI ${esc(p.doi)}` : ""}</div>
        ${p.abstract ? `<details><summary class="muted" style="cursor:pointer">摘要</summary><p class="pre" style="margin:6px 0">${esc(p.abstract)}</p></details>` : ""}
        <div class="muted" style="font-size:13px;margin-top:4px">${esc(p.gbt)}</div>
        <div class="row" style="margin-top:6px">
          ${CTX.readonly ? "" : `<button class="sm pri" data-act="ppAdd" data-i="${i}">${p.is_oa && p.pdf_url ? "收入论文库（下载全文）" : "收入论文库（题录+摘要）"}</button>`}
          <button class="sm" data-act="ppCopy" data-i="${i}">复制引用</button><button class="sm" data-act="ppZot" data-i="${i}">存到 Zotero</button></div>
      </div></div></div>`;
}

// ---------- 收入资料库 ----------
actions.ppAdd = async (el) => {
  const p = CTX.results[Number(el.dataset.i)];
  const label = el.textContent;
  el.disabled = true;
  try {
    if (p.is_oa && p.pdf_url) {
      try {
        el.textContent = "正在下载全文…";
        const res = await fetch(`/api/papers/fetch-pdf?url=${encodeURIComponent(p.pdf_url)}&project_id=${encodeURIComponent(CTX.pid)}`, { headers: { "X-KY": "1" }, credentials: "same-origin" });
        if (!res.ok) throw new Error((await res.json().catch(() => ({}))).detail || `下载失败（${res.status}）`);
        const blob = await res.blob();
        const file = new File([blob], (p.title.slice(0, 60).replace(/[\\/:*?"<>|]/g, "_") || "paper") + ".pdf", { type: "application/pdf" });
        const pages = await extractPdf(file, (i, n) => { el.textContent = `读取 PDF ${i}/${n} 页…`; });
        el.textContent = "保存中…";
        const fd = new FormData();
        fd.append("file", file);
        fd.append("title", p.title);
        fd.append("author", p.authors.join("；"));
        fd.append("pages", JSON.stringify(pages));
        if (p.doi) fd.append("doi", p.doi);
        fd.append("source_url", p.landing_url || p.pdf_url);
        if (p.year) fd.append("source_date", String(p.year));
        if (CTX.logId) fd.append("search_log_id", CTX.logId);
        if (CTX.pid) fd.append("project_id", CTX.pid);
        const m = await api("POST", "/api/materials", fd);
        el.textContent = "✓ 已收入全文";
        loadLogs();
        if (needsOCR(m)) {
          if (confirm(`已收入“${m.title}”，但这是扫描版 PDF，${m.status === "failed" ? "没有" : "有的页没有"}文字层。\n要现在用 AI 识别文字（OCR）吗？`)) startOCR(m, null);
          else toast("可以稍后在“库中论文”里点“OCR 识别”", true);
          return;
        }
        toast(`已收入：${m.title}（全文 PDF）`);
        return;
      } catch (e) {
        if (!confirm(`全文下载或读取失败：${e.message || e}\n\n改为只收入题录和摘要？`)) { el.textContent = label; el.disabled = false; return; }
      }
    }
    el.textContent = "保存中…";
    await api("POST", "/api/papers/save-record", { paper: p, project_id: CTX.pid, search_log_id: CTX.logId });
    el.textContent = "✓ 已收入题录";
    toast("已收入题录和摘要。问答只能依据摘要；需要逐句核对时请上传全文 PDF");
    loadLogs();
  } catch (e) {
    el.textContent = label;
    el.disabled = false;
    throw e;
  }
};

// ---------- 引用 ----------
async function copyText(t) {
  try { await navigator.clipboard.writeText(t); return true; } catch {}
  const ta = document.createElement("textarea");
  ta.value = t; ta.style.position = "fixed"; ta.style.opacity = "0";
  document.body.appendChild(ta); ta.select();
  let ok = false;
  try { ok = document.execCommand("copy"); } catch {}
  ta.remove();
  if (!ok) modal(`<h3>请手动复制</h3><textarea style="min-height:200px">${esc(t)}</textarea>`);
  return ok;
}
actions.ppCopy = async (el) => { if (await copyText(CTX.results[Number(el.dataset.i)].gbt)) toast("已复制（GB/T 7714）"); };
actions.ppSelAll = () => { const bs = $$(".ppSel"); const all = bs.every((b) => b.checked); bs.forEach((b) => (b.checked = !all)); };
actions.ppCopySel = async () => {
  const sel = $$(".ppSel").filter((b) => b.checked).map((b) => CTX.results[Number(b.dataset.i)]);
  if (!sel.length) return toast("请先勾选文献", true);
  const t = sel.map((p, i) => `[${i + 1}] ${p.gbt}`).join("\n");
  if (await copyText(t)) toast(`已复制 ${sel.length} 条参考文献`);
};

// ---------- 检索记录 ----------
async function loadLogs() {
  const box = $("#ppLogs");
  if (!box) return;
  const logs = await api("GET", `/api/papers/logs?project_id=${encodeURIComponent(CTX.pid)}`);
  if (!logs.length) { box.innerHTML = '<p class="muted">还没有检索记录。</p>'; return; }
  box.innerHTML = `<div class="tw"><table><tr><th>时间</th><th>检索人</th><th>方式</th><th>检索词 / 条件</th><th>结果</th><th>收入</th></tr>
    ${logs.map((l) => `<tr><td style="white-space:nowrap">${fmtTime(l.at)}</td><td style="white-space:nowrap">${esc(l.user)}</td><td style="white-space:nowrap">${esc({ import: "导入题录", zotero: "Zotero" }[l.kind] || "OpenAlex")}</td>
      <td style="word-break:break-all;min-width:140px"><code>${esc(l.query)}</code>${l.filters ? `<div class="muted">${esc(l.filters)}</div>` : ""}</td><td>${l.count}</td>
      <td>${(l.added || []).length ? (l.added || []).map((a) => `<div>${esc(a.title)} <span class="tag">${esc(a.mode)}</span></div>`).join("") : '<span class="muted">—</span>'}</td></tr>`).join("")}</table></div>`;
}
actions.ppLogs = () => loadLogs();

actions.ppZot = (el) => saveToZotero([CTX.results[Number(el.dataset.i)]], el);
actions.ppZotSel = (el) => {
  const sel = $$(".ppSel").filter((b) => b.checked).map((b) => CTX.results[Number(b.dataset.i)]);
  if (!sel.length) return toast("请先勾选文献", true);
  return saveToZotero(sel, el);
};
