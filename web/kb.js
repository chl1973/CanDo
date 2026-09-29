// 资料库与有依据问答（项目资料库与个人资料库共用）
import { $, $$, esc, api, apiBg, toast, modal, closeModal, actions, fmtTime, extractPdf, takeOpenJob, popMenu } from "/core.js";
import { startOCR, needsOCR } from "/ocr.js";

export const CITES = {};
let CTX = { pid: "", el: null, opts: {} };

// ---------------- 资料库 ----------------

const ic = (id, cls = "sm") => `<svg class="ic ${cls}"><use href="#${id}"/></svg>`;

export async function renderMaterials(el, pid, opts = {}) {
  CTX = { pid, el, opts, q: "", f: "" };
  const group = !pid && opts.scope === "group";
  const upload = opts.readonly || group ? "" : `<div class="card">
      <form data-submit="upload" id="upForm">
        <label class="upzone" id="upZone">
          <span class="upic">${ic("i-upload", "")}</span>
          <div><b>点击选择文件，或把文件拖到这里</b><span class="muted">PDF、Markdown、TXT，可以一次选多个，单个不超过 50 MB。扫描件上传后可以 OCR 识别文字。${pid ? "项目论文对项目成员和指导老师可见。" : "默认只有你自己能看到，可以逐篇共享到全组。"}</span></div>
          <input type="file" name="file" accept=".pdf,.md,.markdown,.txt" multiple data-change="upPick">
        </label>
        <details class="upmore"><summary>上传一篇时，可以先填写标题和作者</summary>
          <div class="grid2" style="margin-top:8px"><input name="title" placeholder="标题（可不填，默认用文件名；同名再次上传记为新版本）"><input name="author" placeholder="作者（可不填；上传者不等于作者）"></div></details>
        <div class="row hide" id="upRow" style="margin-top:8px"><button class="pri" type="submit">上传</button></div>
        <div id="upState" class="muted" style="margin-top:6px"></div>
      </form></div>`;
  el.innerHTML = `${group ? `<div class="msg info" style="margin-top:0">${ic("i-users")} 全组同学共享出来的论文：大家都能看、能 AI 速读、能在问答和 AI 起草里引用；只有上传者可以修改和删除。想共享自己的论文，到“我的论文”里点“⋯ → 共享到全组”。</div>` : ""}
    ${upload}
    <div class="libbar" id="libBar"><div class="search">${ic("i-search")}<input id="libQ" placeholder="搜索标题、作者、文件名…" data-input="libFilter" autocomplete="off"></div>
      <select id="libF" data-change="libFilter"><option value="">全部状态</option><option value="ready">可用</option><option value="ocr">需要 OCR</option>${!pid && !group ? '<option value="shared">已共享到全组</option>' : ""}</select>
      <span class="sp"></span><span class="muted" id="libCount"></span></div>
    <div class="card" id="matList" style="padding-top:8px;padding-bottom:8px"><div class="muted">加载中…</div></div>`;
  const z = $("#upZone");
  if (z) {
    ["dragenter", "dragover"].forEach((t) => z.addEventListener(t, (e) => { e.preventDefault(); z.classList.add("over"); }));
    ["dragleave", "drop"].forEach((t) => z.addEventListener(t, (e) => { e.preventDefault(); z.classList.remove("over"); }));
    z.addEventListener("drop", (e) => { const fs = [...(e.dataTransfer?.files || [])]; if (fs.length) uploadFiles(fs).catch((er) => toast(er.message || String(er), true)); });
  }
  await loadMatList();
}

const needOCR = (m) => m.ftype === "pdf" && (m.status === "failed" || m.status === "partial");

async function loadMatList() {
  const { pid, opts } = CTX;
  const group = !pid && opts.scope === "group";
  const ms = await api("GET", `/api/materials${pid ? `?project_id=${pid}` : group ? "?scope=group" : ""}`);
  CTX.ms = ms;
  paintMatList();
}

function paintMatList() {
  const { pid, opts, ms = [] } = CTX;
  const group = !pid && opts.scope === "group";
  const box = $("#matList");
  if (!box) return;
  $("#libBar")?.classList.toggle("hide", ms.length < 1);
  if (!ms.length) {
    box.innerHTML = `<div class="empty">${ic("i-library", "")}${group ? "还没有人共享论文。" : pid ? "项目论文库还是空的。上传论文后，项目成员都能基于原文提问。" : "还没有论文。把 PDF 拖到上面的框里，或者到“检索添加”在线检索、从 Zotero 导入。"}</div>`;
    return;
  }
  const q = (CTX.q || "").toLowerCase(), f = CTX.f || "";
  const list = ms.filter((m) => (!q || `${m.title} ${m.author || ""} ${m.filename} ${m.uploader || ""}`.toLowerCase().includes(q))
    && (!f || (f === "ready" && m.status === "ready") || (f === "ocr" && needOCR(m)) || (f === "shared" && m.shared)));
  const c = $("#libCount");
  if (c) c.textContent = list.length === ms.length ? `共 ${ms.length} 篇` : `${list.length} / ${ms.length} 篇`;
  if (!list.length) { box.innerHTML = `<div class="empty">${ic("i-search", "")}没有符合条件的论文</div>`; return; }
  const tag = (m) => `<span class="tag ${{ ready: "ok", partial: "warn", failed: "bad" }[m.status] || ""}">${esc(m.status_text)}</span>`;
  box.innerHTML = `<div class="tw"><table class="mtable"><thead><tr><th>论文</th><th>状态</th><th></th></tr></thead><tbody>
    ${list.map((m) => `<tr><td><div class="mtitle">${esc(m.title)}${m.version > 1 ? ` <span class="tag">v${m.version}</span>` : ""}${!pid && !group && m.shared ? ` <span class="tag pri">${ic("i-users")}已共享</span>` : ""}</div>
      <div class="mmeta">${[m.author ? esc(m.author) : "", esc(m.filename), pid || group ? `${esc(m.uploader)} 上传` : "", fmtTime(m.uploaded_at).slice(0, 10)].filter(Boolean).join(" · ")}</div>
      ${m.error || m.parse_note ? `<div class="mmeta" style="color:var(--warn)">${esc(m.error || m.parse_note)}</div>` : ""}</td>
      <td>${tag(m)}</td>
      <td class="mact-td"><div class="mact">${m.status !== "failed" ? `<button class="sm pri" data-act="readBrief" data-id="${esc(m.id)}" data-title="${esc(m.title)}" title="AI 快速读懂：概括、方法、发现、术语、需要先懂的知识">${ic("i-assistant")}AI 速读</button>` : ""}${needOCR(m) && !opts.readonly && !group ? `<button class="sm pri" data-act="ocrMat" data-id="${esc(m.id)}">OCR 识别</button>` : ""}<a class="btnlike sm" href="/api/materials/${esc(m.id)}/file" target="_blank">原文</a><button class="sm ghost" data-act="matMenu" data-id="${esc(m.id)}" title="更多操作" aria-label="更多操作">${ic("i-dots")}</button></div></td></tr>`).join("")}</tbody></table></div>`;
}
actions.libFilter = () => { CTX.q = $("#libQ")?.value.trim() || ""; CTX.f = $("#libF")?.value || ""; paintMatList(); };
actions.matMenu = (el) => {
  const { pid, opts } = CTX;
  const group = !pid && opts.scope === "group";
  const m = (CTX.ms || []).find((x) => x.id === el.dataset.id);
  if (!m) return;
  const mine = m.owner_id === window.KY_ME_ID;
  popMenu(el, [
    m.status !== "failed" && { label: "查看提取的文字", act: "viewChunks", icon: "i-file", data: { id: m.id, title: `${m.title} v${m.version}` } },
    !pid && !group && { label: m.shared ? "取消共享" : "共享到全组", act: "shareMat", icon: "i-share", data: { id: m.id, v: m.shared ? 0 : 1 } },
    group && mine && { label: "取消共享", act: "shareMat", icon: "i-share", data: { id: m.id, v: 0 } },
    !opts.readonly && !group && "-",
    !opts.readonly && !group && { label: "删除", act: "delMat", icon: "i-x", cls: "danger", data: { id: m.id, title: m.title } },
  ]);
};

actions.shareMat = async (el) => {
  const on = el.dataset.v === "1";
  if (on && !confirm("共享后，全组同学都能看到这篇论文的原文和提取的文字（只读），可以速读、提问和在写作中引用。\n请确认没有版权或保密方面的限制。继续？")) return;
  await api("PATCH", `/api/materials/${el.dataset.id}`, { shared: on });
  toast(on ? "已共享到全组" : "已取消共享");
  loadMatList();
};

actions.upPick = (el) => { const fs = [...el.files]; if (fs.length) uploadFiles(fs).catch((e) => toast(e.message || String(e), true)); };
actions.upload = async (f) => {
  const fs = [...(f.file.files || [])];
  if (!fs.length) return toast("请选择文件", true);
  await uploadFiles(fs);
};

async function uploadFiles(files) {
  const form = $("#upForm"), st = $("#upState");
  const say = (t) => { if (st) st.textContent = t; };
  const one = files.length === 1;
  let ok = 0, lastOCR = null;
  const fails = [];
  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    const pre = one ? "" : `（${i + 1}/${files.length}）${file.name}：`;
    if (file.size > 50 * 1024 * 1024) { fails.push(`${file.name}：文件超过 50 MB`); continue; }
    if (!/\.(pdf|md|markdown|txt)$/i.test(file.name)) { fails.push(`${file.name}：只支持 PDF、Markdown、TXT`); continue; }
    const fd = new FormData();
    fd.append("file", file);
    if (one && form) { fd.append("title", form.title.value); fd.append("author", form.author.value); }
    if (CTX.pid) fd.append("project_id", CTX.pid);
    if (file.name.toLowerCase().endsWith(".pdf")) {
      say(pre + "正在读取 PDF 文字…");
      try {
        const pages = await extractPdf(file, (a, n) => say(`${pre}正在读取 PDF 文字 ${a}/${n} 页…`));
        fd.append("pages", JSON.stringify(pages));
      } catch (e) {
        fails.push(`${file.name}：无法读取（可能已加密或损坏）`);
        continue;
      }
    }
    say(pre + "上传中…");
    try {
      const m = await api("POST", "/api/materials", fd);
      ok++;
      if (m.status === "failed" && !needsOCR(m)) fails.push(`${m.title}：解析失败（${m.error}）`);
      if (needsOCR(m)) lastOCR = m;
    } catch (e) { fails.push(`${file.name}：${e.message || e}`); }
  }
  say("");
  if (form) form.reset();
  await loadMatList();
  if (fails.length) toast(`${ok ? `已上传 ${ok} 篇；` : ""}${fails.length} 个没有成功：${fails.join("；")}`, true);
  else if (ok) toast(one ? "上传成功" : `已上传 ${ok} 篇`);
  if (lastOCR && confirm(`${lastOCR.title}：${lastOCR.status === "failed" ? "这个 PDF 没有文字层（可能是扫描件）。" : "这个 PDF 有的页没有文字（可能是扫描页）。"}\n要现在用 AI 识别文字（OCR）吗？${one ? "" : "\n（其他扫描件可以之后在列表里点“OCR 识别”。）"}`)) {
    startOCR(lastOCR, () => loadMatList());
  }
}

actions.viewChunks = async (el) => {
  const cs = await api("GET", `/api/materials/${el.dataset.id}/chunks`);
  modal(`<h3>${esc(el.dataset.title)}</h3><p class="muted">以下是系统从文件中提取出的文字（共 ${cs.length} 段），检索和问答都基于这些文字。请以原文件为准。</p>
    <a href="/api/materials/${esc(el.dataset.id)}/file" target="_blank">打开原文件</a>
    ${cs.map((c) => `<div class="frag"><span class="muted">${esc(c.location)}</span>\n${esc(c.text)}</div>`).join("")}`);
};

actions.delMat = async (el) => {
  if (!confirm(`确定删除“${el.dataset.title}”？\n删除后不可恢复，之前问答中对它的引用会显示“材料已删除”。`)) return;
  const also = confirm("是否同时删除你引用过这份资料的问答记录？\n确定 = 一并删除；取消 = 保留问答（引用显示为已删除）");
  const r = await api("DELETE", `/api/materials/${el.dataset.id}?delete_answers=${also}`);
  toast(r.warning || "已删除");
  loadMatList();
};

// ---------------- 引用展示 ----------------

export function citeChip(cid) {
  const c = CITES[cid];
  if (!c) return "";
  if (c.deleted) return `<span class="cite del" title="${esc(c.message)}">${esc(c.title)} · 已删除</span>`;
  return `<span class="cite" data-act="cite" data-cid="${esc(cid)}">${esc(c.title)} v${c.version} · ${esc(c.location)}</span>`;
}

export function showCite(cid) {
  const c = CITES[cid];
  if (!c) return;
  if (c.deleted) return modal(`<h3>${esc(c.title)}</h3><div class="msg err">${esc(c.message)}</div>`);
  const page = c.ftype === "pdf" && c.page_index ? `#page=${c.page_index}` : "";
  modal(`<h3>${esc(c.title)} v${c.version}</h3><p class="muted">${esc(c.filename)} · ${esc(c.location)}</p>
    <div class="frag">${esc(c.text)}</div>
    <a href="/api/materials/${esc(c.material_id)}/file${page}" target="_blank">打开原文件${c.page_index ? `（第 ${c.page_index} 页）` : ""}</a>
    <p class="muted">请核对原文是否真的支持对应的说法。引用存在，不等于结论一定正确。</p>`);
}

const evidenceList = (ev) => ev.map((h) => `<div class="frag"><span class="muted">${esc(h.title)} v${h.version} · ${esc(h.location)}</span>\n${esc(h.text)}</div>`).join("");

// ---------------- 问答与对比 ----------------

let ASK = { pid: "", mats: [] };

export async function renderAsk(el, pid) {
  ASK.pid = pid;
  const ms = (await api("GET", `/api/materials${pid ? `?project_id=${pid}` : "?scope=all"}`)).filter((m) => m.status === "ready" || m.status === "partial");
  ASK.mats = ms;
  const opts = ms.map((m) => `<option value="${esc(m.id)}">${esc(m.title)} v${m.version}</option>`).join("");
  el.innerHTML = `<div class="card">
    ${ms.length ? `<form data-submit="ask">
      <div class="row"><label><input type="radio" name="mode" value="ask" checked data-change="askMode">提问（只依据勾选的资料回答）</label><label><input type="radio" name="mode" value="cmp" data-change="askMode">对比两份资料</label></div>
      <div id="askPick" style="margin:8px 0">${ms.map((m) => `<label style="margin-right:14px;display:inline-block"><input type="checkbox" name="mids" value="${esc(m.id)}">${esc(m.title)} v${m.version}${!pid && m.owner_id !== window.KY_ME_ID ? `<span class="muted">（${esc(m.uploader)} 共享）</span>` : ""}</label>`).join("")}</div>
      <div id="cmpPick" class="grid2 hide" style="margin:8px 0"><select name="a">${opts}</select><select name="b">${opts}</select></div>
      <textarea name="q" placeholder="例如：5 秒内的路程是多少？在什么条件下成立？" style="min-height:70px"></textarea>
      <div class="row" style="margin-top:8px"><button class="pri" type="submit">提交</button>
        <select name="effort" style="width:auto" title="自动：简单问题用日常模型、难题用难题模型"><option value="">自动选择模型</option><option value="fast">快速（省钱）</option><option value="deep">深度（用难题模型）</option></select>
        <span class="muted">回答中每条结论都会标明依据；资料里没有的，系统会直接说明。</span></div>
    </form>` : `<div class="empty">论文库里还没有可用的论文，请先在“论文库”中上传或检索添加。</div>`}
  </div><div id="askOut"></div><div class="card"><h4>我的问答记录</h4><div id="askHis" class="muted">加载中…</div></div>`;
  if (ms.length > 1) { const b = el.querySelector("select[name=b]"); if (b) b.selectedIndex = 1; }
  loadHis();
  const oj = takeOpenJob(["资料问答", "资料对比"]);
  if (oj) api("GET", `/api/jobs/${oj.id}`).then((r) => { const o = $("#askOut"); if (o) { renderAnswer(r.result, o); o.scrollIntoView({ behavior: "smooth" }); } }).catch((e) => toast(e.message, true));
}

actions.askMode = (el) => {
  const cmp = el.value === "cmp";
  $("#askPick").classList.toggle("hide", cmp);
  $("#cmpPick").classList.toggle("hide", !cmp);
};

async function loadHis() {
  const list = await api("GET", `/api/answers${ASK.pid ? `?project_id=${ASK.pid}` : ""}`);
  const box = $("#askHis");
  if (!box) return;
  box.innerHTML = list.length ? list.map((a) => `<div class="row" style="padding:4px 0;border-bottom:1px solid var(--line)"><span class="tag">${a.kind === "ask" ? "问答" : "对比"}</span>
    <a href="#" style="flex:1" data-act="openAnswer" data-id="${esc(a.id)}">${esc(a.question)}</a><span class="muted">${fmtTime(a.created_at)}</span>
    <button class="sm" data-act="delAnswer" data-id="${esc(a.id)}">删除</button></div>`).join("") : "暂无记录";
}

actions.ask = async (f) => {
  const fd = new FormData(f);
  const q = fd.get("q").trim();
  if (!q) return toast("请输入问题", true);
  const out = $("#askOut");
  out.innerHTML = `<div class="card muted">正在检索资料并生成回答（在工作台后台运行，可以离开本页，完成后在右上角“后台任务”或下方历史记录里查看）…</div>`;
  let d;
  const meta = { title: q, link: ASK.pid ? `#/p/${ASK.pid}/ask` : "#/mine/ask" };
  try {
    if (fd.get("mode") === "cmp") {
      d = await apiBg("POST", "/api/compare", { material_a: fd.get("a"), material_b: fd.get("b"), question: q, project_id: ASK.pid, effort: fd.get("effort") }, meta);
    } else {
      const mids = fd.getAll("mids");
      if (!mids.length) { out.innerHTML = ""; return toast("请至少勾选一份资料", true); }
      d = await apiBg("POST", "/api/ask", { material_ids: mids, question: q, project_id: ASK.pid, effort: fd.get("effort") }, meta);
    }
  } catch (e) { if (out.isConnected) out.innerHTML = `<div class="msg err">${esc(e.message)}（问题已保留，可以重试）</div>`; return; }
  if (!out.isConnected) return;
  renderAnswer(d, out);
  loadHis();
};

actions.openAnswer = async (el) => { const d = await api("GET", `/api/answers/${el.dataset.id}`); renderAnswer(d, $("#askOut")); $("#askOut").scrollIntoView({ behavior: "smooth" }); };
actions.delAnswer = async (el) => { if (!confirm("删除这条记录？")) return; await api("DELETE", `/api/answers/${el.dataset.id}`); loadHis(); };

const TYPE_NOTE = { 原文支持: "资料原文直接写明", 解释: "对原文的通俗解释", 推断: "由资料推出，资料未直接确认", 验证计算: "为核对所做的计算，不属于原文" };

export function renderAnswer(d, out) {
  Object.assign(CITES, d.citations || {});
  let h = `<div class="card"><div class="muted">问题：${esc(d.question)}</div>`;
  if (["llm_unavailable", "llm_error", "no_evidence"].includes(d.status)) {
    h += `<div class="msg ${d.status === "llm_error" ? "err" : ""}">${esc(d.message)}</div>`;
    if (d.kind === "compare" || d.evidence?.a) {
      const ev = d.evidence || { a: [], b: [] };
      h += `<div class="grid2"><div><b>资料 A 的相关原文</b>${evidenceList(ev.a || []) || '<p class="muted">无</p>'}</div><div><b>资料 B 的相关原文</b>${evidenceList(ev.b || []) || '<p class="muted">无</p>'}</div></div>`;
    } else if (d.evidence?.length) h += `<b>检索到的原文片段</b>${evidenceList(d.evidence)}`;
    out.innerHTML = h + "</div>";
    return;
  }
  if (d.kind === "compare") {
    if (d.summary) h += `<p>${esc(d.summary)}</p>`;
    for (const it of d.items || []) {
      h += `<div class="claim"><div class="row"><b>${esc(it.aspect)}</b><span class="tag ${it.relation === "可能冲突" ? "bad" : it.relation === "一致" ? "ok" : "warn"}">${esc(it.relation)}</span></div>
        <div class="grid2"><div><div class="muted">A 的说法</div>${esc(it.a.says)}<div>${it.a.chunk_ids.map(citeChip).join("")}</div></div>
        <div><div class="muted">B 的说法</div>${esc(it.b.says)}<div>${it.b.chunk_ids.map(citeChip).join("")}</div></div></div>
        ${it.note ? `<p class="muted">${esc(it.note)}</p>` : ""}${it.calc ? `<div class="frag"><span class="tag">验证计算</span> ${esc(it.calc.text)}\n<span class="muted">${esc(it.calc.label)}</span></div>` : ""}</div>`;
    }
    h += `<p class="muted">系统不裁定哪份资料正确，请结合原文判断。</p>`;
  } else {
    if (d.status === "insufficient") h += `<div class="msg">资料中没有找到能直接支持回答的原文，以下内容仅供参考。</div>`;
    for (const c of d.claims || []) {
      h += `<div class="claim t-${esc(c.type)} ${c.check === "no_valid_citation" ? "bad" : ""}"><span class="tag">${esc(c.type)}</span> <span class="muted">${TYPE_NOTE[c.type] || ""}</span>
        <div>${esc(c.text)}</div>${c.conditions ? `<div class="muted">适用条件：${esc(c.conditions)}</div>` : ""}${c.premises ? `<div class="muted">推断前提：${esc(c.premises)}</div>` : ""}
        ${c.check_note ? `<div style="color:var(--bad);font-size:13px">⚠ ${esc(c.check_note)}</div>` : ""}<div>${c.chunk_ids.map(citeChip).join("")}</div></div>`;
    }
    if (d.example) h += `<div class="claim"><span class="tag">例子</span> <span class="muted">${esc(d.example.note)}</span><div>${esc(d.example.text)}</div></div>`;
  }
  if (d.missing?.length) h += `<div class="msg"><b>资料不足：</b><ul style="margin:4px 0">${d.missing.map((m) => `<li>${esc(m)}</li>`).join("")}</ul></div>`;
  if (d.model) h += `<div class="muted" style="font-size:12px;margin-top:6px">由 ${esc(d.model)} 生成${d.route ? `（${esc(d.route)}）` : ""}</div>`;
  const mid = (d.material_ids || [])[0];
  if (mid && (d.material_ids || []).length) h += `<form data-submit="askConcept" data-mid="${esc(mid)}" class="row noprint" style="margin-top:8px"><input name="term" maxlength="60" required placeholder="回答里有看不懂的词？输入后点“讲解”" style="flex:1;min-width:160px"><button class="sm" type="submit">讲解</button></form>`;
  out.innerHTML = h + "</div>";
}

actions.askConcept = async (f) => {
  const { openConcept } = await import("/read.js");
  openConcept(f.term.value.trim(), "", f.dataset.mid);
};

actions.ocrMat = (el) => {
  const m = (CTX.ms || []).find((x) => x.id === el.dataset.id);
  if (m) startOCR(m, () => loadMatList());
};
