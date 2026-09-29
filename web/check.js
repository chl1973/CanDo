// 引用核验：列表、新建、报告（证据附录）
import { $, $$, esc, api, toast, actions, fmtTime, fileToText } from "/core.js";
import { CITES, citeChip } from "/kb.js";

let CK = { pid: "", id: null, data: null, timer: null, filter: "all" };

const LEVEL = { ok: ["有原文对应", "ok"], partial: ["部分对应", "warn"], review: ["待核查", "bad"], skipped: ["未核验", ""] };
const EXISTS = { found: ["公开库已找到", "ok"], not_found: ["公开库未找到", "warn"], error: ["无法联网核对", ""], skipped: ["未识别题名", ""], off: ["未联网核对", ""], pending: ["尚未核对", ""] };

export async function renderChecks(el, pid) {
  CK.pid = pid;
  const list = await api("GET", `/api/citechecks${pid ? `?project_id=${pid}` : ""}`);
  el.innerHTML = `<div class="card"><h3>新建引用核验</h3>
    <p class="muted">把论文或报告的全文（含“参考文献”列表）粘贴到下面，系统会：① 检查参考文献能否在公开数据库中找到；② 对照你上传的文献原文，逐句核对引用是否有原文对应。结果只分“有原文对应 / 部分对应 / 待核查”，最终由你和老师判断。</p>
    <form data-submit="newCheck">
      <input name="title" placeholder="名称，例如：结题报告初稿（可不填）">
      <div class="row" style="margin:8px 0"><span class="muted">也可以直接读取文件：</span><input type="file" accept=".pdf,.md,.txt" data-change="loadPaper" style="width:auto"><span id="paperState" class="muted"></span></div>
      <textarea name="text" style="min-height:160px" placeholder="粘贴全文。建议从 Word 中复制（从 PDF 复制时上标形式的 [1] 可能丢失）。引用标注需为 [1]、[2,3]、[4-6] 这类格式。"></textarea>
      <button class="pri" type="submit" style="margin-top:8px">识别引用</button>
    </form></div>
    <div class="card"><h4>核验记录</h4>${list.length ? list.map((c) => `<div class="row" style="padding:6px 0;border-bottom:1px solid var(--line)">
      <a href="#/check/${esc(c.id)}" style="flex:1">${esc(c.title)}</a><span class="muted">${esc(c.owner)} · ${fmtTime(c.created_at)}</span>
      ${c.summary ? `<span class="tag ok">${c.summary.ok || 0} 对应</span><span class="tag bad">${c.summary.review || 0} 待核查</span>` : ""}
      <span class="tag">${{ parsed: "待运行", running: "核验中", done: "已完成", failed: "失败" }[c.status] || c.status}</span></div>`).join("") : '<p class="muted">暂无记录</p>'}</div>`;
}

actions.loadPaper = async (el) => {
  const f = el.files[0];
  if (!f) return;
  $("#paperState").textContent = "读取中…";
  try {
    const t = await fileToText(f);
    el.form.text.value = t;
    if (!el.form.title.value) el.form.title.value = f.name.replace(/\.[^.]+$/, "");
    $("#paperState").textContent = `已读取 ${t.length} 字，请检查后点击“识别引用”`;
  } catch (e) { $("#paperState").textContent = ""; toast("读取失败：" + (e.message || e), true); }
};

actions.newCheck = async (f) => {
  const c = await api("POST", "/api/citechecks", { title: f.title.value, text: f.text.value, project_id: CK.pid });
  location.hash = `#/check/${c.id}`;
};

export async function renderCheckDetail(main, id) {
  clearTimeout(CK.timer);
  CK.id = id;
  const c = await api("GET", `/api/citechecks/${id}`);
  CK.data = c;
  CK.pid = c.project_id;
  Object.assign(CITES, c.citations || {});
  draw(main);
  if (c.status === "running") CK.timer = setTimeout(() => { if (location.hash === `#/check/${id}`) renderCheckDetail(main, id).catch(() => {}); }, 1500);
}

function draw(main) {
  const c = CK.data;
  const s = c.summary || {};
  const back = c.project_id ? `#/p/${c.project_id}/checks` : "#/mine";
  const editable = c.is_owner && c.status !== "running";
  const refTitle = {};
  c.refs.forEach((r) => (refTitle[r.no] = r.title || r.raw));
  const pct = c.total ? Math.round((c.progress / c.total) * 100) : 0;
  const results = c.sentences.flatMap((x) => x.results);
  const ran = results.length > 0;
  main.innerHTML = `<div class="row noprint"><a href="${back}" class="muted">← 返回</a><span class="sp"></span>${ran ? '<button data-act="printCheck">打印 / 导出证据附录</button>' : ""}</div>
    <h2 style="margin:8px 0 2px">引用核验：${esc(c.title)}</h2>
    <p class="muted">发起人：${esc(c.owner)} · ${fmtTime(c.created_at)}${c.run_at ? ` · 最近核验 ${fmtTime(c.run_at)}` : ""}</p>
    <div class="msg info">本报告由系统辅助生成：“有原文对应”表示在所关联的原文中找到了相应内容，不等于论证一定正确；“待核查”表示需要人工查看，不代表作者有意出错。</div>
    ${c.warnings.map((w) => `<div class="msg">${esc(w)}</div>`).join("")}
    ${c.status === "running" ? `<div class="card"><b>核验中…</b> ${c.progress}/${c.total}<div class="progress" style="margin-top:8px"><div style="width:${pct}%"></div></div><p class="muted">可以离开此页，完成后回来查看。</p></div>` : ""}
    ${c.status === "failed" ? `<div class="msg err">${esc(c.error)}</div>` : ""}
    <div class="card"><div class="row" style="gap:8px">
      <div class="stat"><span class="muted">参考文献</span><b>${s.refs || 0}</b></div>
      <div class="stat"><span class="muted">引用句</span><b>${s.sentences || 0}</b></div>
      ${ran ? `<div class="stat ok"><span class="muted">有原文对应</span><b>${s.ok || 0}</b></div>
      <div class="stat warn"><span class="muted">部分对应</span><b>${s.partial || 0}</b></div>
      <div class="stat bad"><span class="muted">待核查</span><b>${s.review || 0}</b></div>
      <div class="stat"><span class="muted">未核验</span><b>${s.skipped || 0}</b></div>` : ""}
      ${s.refs_found || s.refs_not_found ? `<div class="stat"><span class="muted">公开库找到/未找到</span><b>${s.refs_found || 0}/${s.refs_not_found || 0}</b></div>` : ""}
    </div></div>

    <div class="card"><h3>参考文献与原文关联</h3>
      <p class="muted noprint">${editable ? "系统已按标题尝试自动关联你上传的资料。请检查：没关联上的，先在资料库上传该文献原文，再在这里选择。未关联原文的文献只做“是否存在”检查。" : ""}</p>
      <div class="tw"><table><tr><th style="width:40px">编号</th><th>文献</th><th style="width:150px">公开数据库</th><th style="width:230px">关联原文</th></tr>
      ${c.refs.map((r) => {
        const [et, ec] = EXISTS[r.exists] || ["", ""];
        const sel = editable ? `<select data-ref="${r.no}" class="mapSel"><option value="">不关联</option>${(c.candidates || []).map((m) => `<option value="${esc(m.id)}" ${m.id === r.material_id ? "selected" : ""}>${esc(m.title)}</option>`).join("")}</select>`
          : esc(c.material_titles[r.material_id] || (r.material_id ? "（已删除或无权访问）" : "未关联"));
        return `<tr><td>[${r.no}]</td><td><b>${esc(r.title || "（未识别题名）")}</b><div class="muted">${esc(r.raw)}</div></td>
          <td><span class="tag ${ec}">${et}</span>${r.exists_note && r.exists !== "pending" ? `<div class="muted">${esc(r.exists_note)}</div>` : ""}${r.match_title ? `<div class="muted">匹配：${esc(r.match_title)} ${r.match_year || ""}</div>` : ""}</td>
          <td>${sel}${r.auto_mapped ? '<div class="muted">自动关联</div>' : ""}</td></tr>`;
      }).join("")}</table></div>
      ${c.refs.length === 0 ? '<p class="muted">没有识别出参考文献</p>' : ""}
      ${editable ? `<div class="row noprint" style="margin-top:10px"><button data-act="saveMap">保存关联</button><button class="pri" data-act="runCheck">${ran ? "重新核验" : "开始核验"}</button><span class="muted">核验单次最多处理 80 处引用。</span></div>` : ""}
    </div>

    ${ran ? `<div class="card"><div class="row noprint" style="margin-bottom:6px"><h3 style="margin:0">逐句核验结果</h3><span class="sp"></span>
      ${["all", "review", "partial", "ok", "skipped"].map((k) => `<button class="sm ${CK.filter === k ? "pri" : ""}" data-act="ckFilter" data-k="${k}">${k === "all" ? "全部" : LEVEL[k][0]}</button>`).join("")}</div>
      ${c.sentences.map((st) => {
        const rs = st.results.filter((r) => CK.filter === "all" || r.level === CK.filter);
        if (!rs.length) return "";
        return `<div class="sent"><div><span class="muted">第 ${st.idx} 处：</span>${esc(st.text)}</div>
        ${rs.map((r) => { const [lt, lc] = LEVEL[r.level] || ["", ""];
          return `<div class="res"><div class="row"><b>[${r.ref_no}]</b><span>${esc(refTitle[r.ref_no] || "")}</span><span class="tag ${lc}">${lt}</span>${r.verdict && r.verdict !== "未核验" ? `<span class="muted">模型判断：${esc(r.verdict)}</span>` : ""}</div>
          ${r.note ? `<div class="muted">${esc(r.note)}</div>` : ""}
          ${r.chunk_ids.length ? `<div>依据：${r.chunk_ids.map(citeChip).join("")}</div>${r.chunk_ids.map((id) => CITES[id] && !CITES[id].deleted ? `<div class="frag">${esc(CITES[id].text)}</div>` : "").join("")}`
            : r.candidates.length ? `<div class="muted">可能对应的原文：</div><div>${r.candidates.map(citeChip).join("")}</div>` : ""}</div>`; }).join("")}</div>`;
      }).join("")}</div>` : c.sentences.length ? `<div class="card"><h3>识别到的引用句（${c.sentences.length}）</h3>${c.sentences.slice(0, 50).map((st) => `<div class="sent"><span class="muted">第 ${st.idx} 处 · 引用 [${st.refs.join(", ")}]</span><div>${esc(st.text)}</div></div>`).join("")}${c.sentences.length > 50 ? '<p class="muted">……</p>' : ""}</div>` : ""}
    ${c.is_owner && c.status !== "running" ? `<div class="noprint" style="margin-top:10px"><button class="sm danger" data-act="delCheck">删除这条核验记录</button></div>` : ""}`;
}

function mapping() {
  const m = {};
  $$(".mapSel").forEach((s) => (m[s.dataset.ref] = s.value));
  return m;
}
actions.saveMap = async () => { CK.data = await api("PUT", `/api/citechecks/${CK.id}/mapping`, mapping()); Object.assign(CITES, CK.data.citations || {}); toast("已保存关联"); draw($("#main")); };
actions.runCheck = async () => {
  await api("PUT", `/api/citechecks/${CK.id}/mapping`, mapping());
  await api("POST", `/api/citechecks/${CK.id}/run`, {});
  CK.filter = "all";
  renderCheckDetail($("#main"), CK.id);
};
actions.ckFilter = (el) => { CK.filter = el.dataset.k; draw($("#main")); };
actions.printCheck = () => { const f = CK.filter; CK.filter = "all"; draw($("#main")); window.print(); CK.filter = f; draw($("#main")); };
actions.delCheck = async () => {
  if (!confirm("删除这条核验记录？")) return;
  await api("DELETE", `/api/citechecks/${CK.id}`);
  location.hash = CK.pid ? `#/p/${CK.pid}/checks` : "#/mine";
};
