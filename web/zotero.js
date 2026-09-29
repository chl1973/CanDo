// Zotero 直连：浏览文库/分类、导入到资料库（有 PDF 导入全文，否则导入题录）、把检索结果存回 Zotero、设置页连接卡片
import { $, $$, esc, api, toast, modal, closeModal, actions, fmtTime, extractPdf } from "/core.js";
import { needsOCR, rememberMat } from "/ocr.js";

const Z = { el: null, pid: "", readonly: false, onChange: null, src: "", lib: "user", libName: "我的文库", col: "", colName: "", q: "", start: 0, items: [], total: 0, status: null, busy: false };
const SRC_NAME = { local: "本机 Zotero", cloud: "Zotero 云端" };
const LOCAL_HELP = "需要 Zotero 7 或更新版本：打开 Zotero → 编辑 → 设置 → 高级，勾选“允许此计算机上的其他应用程序与 Zotero 通信”，并保持 Zotero 开着。";

async function status() {
  Z.status = await api("GET", "/api/zotero/status");
  return Z.status;
}

function sources(s) {
  const out = [];
  if (s.local_allowed) out.push(["local", "本机 Zotero"]);
  if (s.cloud.configured) out.push(["cloud", `Zotero 云端（${s.cloud.username || s.cloud.user_id}）`]);
  return out;
}

// ---------------- 文献检索页：从 Zotero 导入 ----------------

export async function renderZotero(el, pid, opts = {}) {
  Z.el = el;
  Z.pid = pid || "";
  Z.readonly = !!opts.readonly;
  Z.onChange = opts.onChange;
  el.innerHTML = `<div class="card muted">正在连接 Zotero…</div>`;
  const s = await status();
  const srcs = sources(s);
  if (!srcs.find((x) => x[0] === Z.src)) Z.src = s.local_allowed && s.local && s.local.ok ? "local" : s.cloud.configured ? "cloud" : srcs[0] ? srcs[0][0] : "";
  if (!srcs.length) {
    el.innerHTML = `<div class="card"><h3>从 Zotero 导入</h3>
      <p>你现在不是在运行工作台的电脑上，需要先连接 <b>Zotero 云端</b>（一次设置，约 2 分钟）：</p>
      <p class="muted">到“设置 → 我的 Zotero”，按提示在 zotero.org 新建一个私人密钥并粘贴进来。连接后手机上也能浏览自己的 Zotero 文库、导入文献。</p>
      <a class="btn pri" href="#/settings">去设置</a></div>`;
    return;
  }
  el.innerHTML = `<div class="card">
    <div class="row">
      <select data-change="zSrc" style="width:auto">${srcs.map(([k, n]) => `<option value="${k}" ${k === Z.src ? "selected" : ""}>${esc(n)}</option>`).join("")}</select>
      <select id="zLib" data-change="zLib" style="width:auto;min-width:140px"></select>
      <select id="zCol" data-change="zCol" style="width:auto;min-width:160px;max-width:100%"></select>
      <form data-submit="zSearch" class="row" style="flex:1;min-width:220px"><input name="q" value="${esc(Z.q)}" placeholder="按题名、作者、年份筛选" style="flex:1;min-width:120px"><button type="submit">筛选</button></form>
    </div>
    <div id="zMsg"></div><div id="zList"></div></div>`;
  await openSource();
}

async function openSource() {
  const msg = $("#zMsg");
  msg.innerHTML = "";
  $("#zList").innerHTML = "";
  if (Z.src === "local" && Z.status.local && !Z.status.local.ok) {
    msg.innerHTML = `<div class="msg">${esc(Z.status.local.message)}</div><p class="muted">${LOCAL_HELP}</p>
      <button data-act="zRetry">重新连接</button>${Z.status.cloud.configured ? "" : ' <span class="muted">也可以在“设置 → 我的 Zotero”连接 Zotero 云端。</span>'}`;
    $("#zLib").innerHTML = "";
    $("#zCol").innerHTML = "";
    return;
  }
  try {
    const libs = await api("GET", `/api/zotero/libraries?src=${Z.src}`);
    if (!libs.find((l) => l.id === Z.lib)) Z.lib = "user";
    Z.libName = (libs.find((l) => l.id === Z.lib) || libs[0]).name;
    $("#zLib").innerHTML = libs.map((l) => `<option value="${esc(l.id)}" ${l.id === Z.lib ? "selected" : ""}>${esc(l.name)}</option>`).join("");
    await loadCols();
    await loadItems();
  } catch (e) {
    msg.innerHTML = `<div class="msg">${esc(e.message || e)}</div><button data-act="zRetry">重新连接</button>`;
  }
}

async function loadCols() {
  const cols = await api("GET", `/api/zotero/collections?src=${Z.src}&lib=${encodeURIComponent(Z.lib)}`);
  if (Z.col && !cols.find((c) => c.key === Z.col)) { Z.col = ""; Z.colName = ""; }
  $("#zCol").innerHTML = `<option value="">全部文献</option>` + cols.map((c) => `<option value="${esc(c.key)}" ${c.key === Z.col ? "selected" : ""}>${"　".repeat(c.depth)}${esc(c.name)}</option>`).join("");
}

async function loadItems() {
  const box = $("#zList");
  box.innerHTML = `<p class="muted">读取中…</p>`;
  const q = new URLSearchParams({ src: Z.src, lib: Z.lib, start: String(Z.start), project_id: Z.pid });
  if (Z.col) q.set("collection", Z.col);
  if (Z.q) q.set("q", Z.q);
  const r = await api("GET", `/api/zotero/items?${q}`);
  Z.items = r.items;
  Z.total = r.total;
  const fresh = r.items.filter((x) => !x.imported).length;
  const where = Z.pid ? "项目论文库" : "我的论文库";
  box.innerHTML = `
    <div class="row" style="margin-top:10px"><span class="muted">${esc(Z.libName)}${Z.colName ? " / " + esc(Z.colName) : ""} · 共 ${r.total} 条${r.total > 50 ? `，当前第 ${Z.start + 1}–${Z.start + r.items.length} 条` : ""}</span><span class="sp"></span>
      ${Z.readonly ? "" : `<button class="sm" data-act="zSelAll">全选未导入</button>
      <button class="sm pri" data-act="zImportSel">导入选中的到${where}</button>
      <button class="sm" data-act="zImportAll" ${fresh ? "" : "disabled"}>导入本页全部未导入的（${fresh}）</button>`}</div>
    <div id="zProg" class="muted"></div>
    ${r.items.length ? r.items.map((it, i) => row(it, i)).join("") : `<p class="muted">这里没有文献${Z.q ? "（可以换个筛选词）" : ""}。</p>`}
    ${r.total > 50 ? `<div class="row" style="margin-top:10px">${Z.start > 0 ? `<button class="sm" data-act="zPage" data-s="${Math.max(0, Z.start - 50)}">上一页</button>` : ""}${Z.start + 50 < r.total ? `<button class="sm" data-act="zPage" data-s="${Z.start + 50}">下一页</button>` : ""}</div>` : ""}
    <p class="muted" style="margin-top:8px">有 PDF 附件的导入全文（可以逐句核对引用）；没有 PDF 的导入题录和摘要。已经在${where}里的会标出，不会重复导入。${Z.src === "cloud" ? "云端只能下载已同步到 Zotero 云端的附件（Zotero 设置 → 同步 → 同步附件文件）。" : ""}</p>`;
}

function row(it, i) {
  const p = it.paper;
  const au = p.authors.length > 6 ? p.authors.slice(0, 6).join("; ") + " 等" : p.authors.join("; ");
  const att = it.is_pdf ? '<span class="tag">PDF 文件</span>' : it.num_children > 0 ? '<span class="tag">有附件</span>' : "";
  return `<div style="border-top:1px solid var(--line);padding:8px 0" class="row">
    <input type="checkbox" class="zSel" data-i="${i}" style="width:auto;align-self:flex-start;margin-top:4px" ${it.imported || Z.readonly ? "disabled" : ""}>
    <div style="flex:1;min-width:0">
      <div><b>${esc(p.title || "（无题名）")}</b> ${it.imported ? '<span class="tag ok">已在论文库</span>' : ""} ${att}</div>
      <div class="muted">${esc(au || "作者不详")}${p.venue ? ` · <i>${esc(p.venue)}</i>` : ""}${p.year ? ` · ${p.year}` : ""}${p.doi ? ` · DOI ${esc(p.doi)}` : ""}
        ${(it.tags || []).map((t) => `<span class="tag">${esc(t)}</span>`).join(" ")}</div>
    </div></div>`;
}

actions.zSrc = (el) => { Z.src = el.value; Z.lib = "user"; Z.col = ""; Z.colName = ""; Z.start = 0; openSource(); };
actions.zLib = async (el) => { Z.lib = el.value; Z.libName = el.options[el.selectedIndex].text; Z.col = ""; Z.colName = ""; Z.start = 0; await loadCols(); await loadItems(); };
actions.zCol = (el) => { Z.col = el.value; Z.colName = el.value ? el.options[el.selectedIndex].text.trim() : ""; Z.start = 0; loadItems(); };
actions.zSearch = (f) => { Z.q = f.q.value.trim(); Z.start = 0; return loadItems(); };
actions.zPage = (el) => { Z.start = Number(el.dataset.s); loadItems(); };
actions.zRetry = async () => { await status(); renderZotero(Z.el, Z.pid, { readonly: Z.readonly, onChange: Z.onChange }); };
actions.zSelAll = () => { const bs = $$(".zSel:not(:disabled)"); const all = bs.every((b) => b.checked); bs.forEach((b) => (b.checked = !all)); };
actions.zImportSel = () => {
  const sel = $$(".zSel").filter((b) => b.checked).map((b) => Z.items[Number(b.dataset.i)]);
  if (!sel.length) return toast("请先勾选要导入的文献", true);
  return importItems(sel);
};
actions.zImportAll = () => importItems(Z.items.filter((x) => !x.imported));

async function importItems(list) {
  if (Z.busy) return;
  Z.busy = true;
  $$("#zList button").forEach((b) => (b.disabled = true));
  const prog = $("#zProg");
  const done = { full: 0, record: 0 }, notes = [], failed = [], scans = [];
  try {
    const { log_id } = await api("POST", "/api/zotero/log", { project_id: Z.pid, src: Z.src, library: Z.libName, collection: Z.colName, query: Z.q, count: list.length });
    for (let i = 0; i < list.length; i++) {
      const it = list[i];
      const t = it.paper.title || "（无题名）";
      prog.textContent = `正在导入 ${i + 1}/${list.length}：${t}`;
      try {
        const r = await importOne(it, log_id, (s) => { prog.textContent = `正在导入 ${i + 1}/${list.length}：${t}（${s}）`; });
        done[r.mode]++;
        if (r.note) notes.push(`${t}：${r.note}`);
        if (needsOCR(r.mat)) { rememberMat(r.mat); scans.push(r.mat); }
      } catch (e) {
        failed.push(`${t}：${e.message || e}`);
      }
    }
  } finally {
    Z.busy = false;
    prog.textContent = "";
  }
  modal(`<h3>导入完成</h3><p>全文 <b>${done.full}</b> 篇，题录和摘要 <b>${done.record}</b> 篇${failed.length ? `，失败 <b>${failed.length}</b> 篇` : ""}。</p>
    ${notes.length ? `<div class="muted"><b>说明：</b>${notes.map((n) => `<div>${esc(n)}</div>`).join("")}</div>` : ""}
    ${failed.length ? `<div class="msg"><b>失败：</b>${failed.map((n) => `<div>${esc(n)}</div>`).join("")}</div>` : ""}
    ${scans.length ? `<div class="msg warnbox"><b>${scans.length} 篇是扫描版 PDF，没有文字层</b>，需要 OCR 识别后才能检索和问答：${scans.map((m) => `<div class="row" style="margin-top:4px"><span style="flex:1">${esc(m.title)}</span><button class="sm pri" data-act="ocrStartId" data-id="${esc(m.id)}">OCR 识别</button></div>`).join("")}</div>` : ""}
    <p class="muted">可以到“库中论文”查看。只有题录的文献，问答只能依据摘要。</p>`);
  loadItems().catch(() => {});
  if (Z.onChange) Z.onChange();
}

async function importOne(it, logId, onStep) {
  const p = it.paper;
  let note = "";
  if (it.is_pdf || it.num_children > 0) {
    onStep("读取 PDF 附件");
    const res = await fetch(`/api/zotero/pdf?src=${Z.src}&lib=${encodeURIComponent(Z.lib)}&key=${it.zkey}`, { headers: { "X-KY": "1" }, credentials: "same-origin" });
    if (res.ok) {
      const blob = await res.blob();
      const file = new File([blob], ((p.title || "zotero").slice(0, 60).replace(/[\\/:*?"<>|]/g, "_")) + ".pdf", { type: "application/pdf" });
      let pages = null;
      try {
        pages = await extractPdf(file, (i, n) => onStep(`读取 PDF ${i}/${n} 页`));
      } catch (e) {
        note = "PDF 无法读取文字（可能已加密），已改为导入题录";
      }
      if (pages) {
        onStep("保存");
        const fd = new FormData();
        fd.append("file", file);
        fd.append("title", p.title || file.name);
        fd.append("author", p.authors.join("；"));
        fd.append("pages", JSON.stringify(pages));
        if (p.doi) fd.append("doi", p.doi);
        if (p.landing_url) fd.append("source_url", p.landing_url);
        if (p.year) fd.append("source_date", String(p.year));
        fd.append("zotero_key", it.zid);
        fd.append("search_log_id", logId);
        if (Z.pid) fd.append("project_id", Z.pid);
        const m = await api("POST", "/api/materials", fd);
        return { mode: "full", mat: m };
      }
    } else {
      const err = await res.json().catch(() => ({}));
      if (res.status !== 404) throw new Error(err.detail || `读取附件失败（${res.status}）`);
      if (it.num_children > 0 && err.detail && err.detail.includes("链接到文件")) note = err.detail;
    }
    if (it.is_pdf && !note) throw new Error("读取 PDF 附件失败");
  }
  onStep("保存题录");
  await api("POST", "/api/papers/save-record", { paper: p, project_id: Z.pid, search_log_id: logId, zotero_key: it.zid });
  return { mode: "record", note };
}

// ---------------- 检索结果存到 Zotero ----------------

export async function saveToZotero(papers, btn) {
  const s = await status();
  const localOk = s.local_allowed && s.local && s.local.ok;
  let src = "";
  if (localOk) src = "local";
  else if (s.cloud.configured) src = "cloud";
  if (!src) {
    if (s.local_allowed) return modal(`<h3>存到 Zotero</h3><div class="msg">${esc(s.local.message)}</div><p class="muted">${LOCAL_HELP}</p><p class="muted">也可以在“设置 → 我的 Zotero”连接 Zotero 云端。</p>`);
    return modal(`<h3>存到 Zotero</h3><p>在这台设备上需要先连接 Zotero 云端（并给密钥写入权限）。</p><a class="btn pri" href="#/settings" data-act="closeModal">去设置</a>`);
  }
  if (src === "cloud" && !s.cloud.write) {
    return modal(`<h3>存到 Zotero</h3><p>你的 Zotero 密钥只有读取权限。请到 <b>zotero.org/settings/keys</b> 编辑这个密钥，勾选“Allow write access”，再到“设置 → 我的 Zotero”重新保存一次密钥。</p>`);
  }
  const label = btn ? btn.textContent : "";
  if (btn) { btn.disabled = true; btn.textContent = "保存中…"; }
  try {
    const r = await api("POST", "/api/zotero/save", { src, lib: "user", papers });
    if (r.failed.length) modal(`<h3>已存入 ${r.saved} 条</h3><div class="msg">${r.failed.map((x) => `<div>${esc(x)}</div>`).join("")}</div>`);
    else toast(`已存入 ${SRC_NAME[src]}：${r.saved} 条（${r.where}，带“CanDo 可为”标签）`);
    if (btn) btn.textContent = "✓ 已存入 Zotero";
  } catch (e) {
    if (btn) { btn.disabled = false; btn.textContent = label; }
    throw e;
  }
}

// ---------------- 设置页：我的 Zotero ----------------

let SEL = null;
export async function renderZoteroSettings(el) {
  SEL = el;
  const s = await status();
  const c = s.cloud;
  const perm = (ok, t) => `<span class="tag ${ok ? "ok" : ""}">${ok ? "✓" : "✗"} ${t}</span>`;
  el.innerHTML = `<h3>我的 Zotero</h3>
    <p class="muted">连接后，可以在“论文库 → 检索添加 → 从 Zotero 导入”里浏览自己的 Zotero 文库（含分类和群组文库），把文献连同 PDF 一键导入论文库；检索到的新文献也可以一键存回 Zotero。</p>
    ${s.local_allowed ? `<h4>本机 Zotero（在这台电脑上使用，不用填任何东西）</h4>
      <div class="row">${s.local.ok ? '<span class="tag ok">已连接</span>' : '<span class="tag warn">未连接</span>'}<span class="${s.local.ok ? "muted" : ""}">${esc(s.local.message)}</span><button class="sm" data-act="zSetRetry">重新检测</button></div>
      ${s.local.ok ? "" : `<p class="muted">${LOCAL_HELP}</p>`}` : ""}
    <h4 style="margin-top:14px">Zotero 云端（手机和其他电脑也能用）</h4>
    ${c.configured ? `<div class="row"><span class="tag ok">已连接</span><b>${esc(c.username || "")}</b><span class="muted">用户 ID ${c.user_id} · 密钥 ${esc(c.key_masked)} · ${fmtTime(c.saved_at)}</span></div>
      <div class="row" style="margin-top:6px">${perm(c.library, "读取文库")}${perm(c.files, "下载附件")}${perm(c.write, "写入（存回 Zotero）")}${perm(c.groups !== "none", c.groups === "some" ? "部分群组" : "群组文库")}</div>
      <div class="row" style="margin-top:8px"><button class="sm" data-act="zSetChange">更换密钥</button><button class="sm danger" data-act="zSetRemove">断开</button></div>` : keyForm()}`;
}

function keyForm() {
  return `<ol class="muted" style="margin:6px 0 8px 18px;padding:0">
      <li>用电脑浏览器打开 <b>zotero.org/settings/keys/new</b> 并登录（和 Zotero 软件里同步用的是同一个账号）。</li>
      <li>勾选 “Allow library access”；想把检索结果存回 Zotero，再勾选 “Allow write access”；课题组有群组文库的，在 Groups 里选 “Read Only”（或 Read/Write）。</li>
      <li>点 “Save Key”，把显示的一串字母数字复制，粘贴到下面。</li></ol>
    <form data-submit="zSetSave" class="row"><input name="key" required autocomplete="off" placeholder="粘贴 Zotero 私人密钥" style="flex:1;min-width:200px"><button class="pri" type="submit">连接</button></form>
    <p class="muted">密钥加密保存在运行工作台的电脑上，只用于你自己的请求，管理员也看不到。要下载 PDF，需要在 Zotero 软件里开启“同步附件文件”。</p>`;
}

actions.zSetRetry = () => renderZoteroSettings(SEL);
actions.zSetChange = (el) => { el.closest(".row").insertAdjacentHTML("afterend", keyForm()); el.disabled = true; };
actions.zSetSave = async (f) => {
  const c = await api("PUT", "/api/zotero/cloud", { key: f.key.value.trim() });
  toast(`已连接 Zotero 云端：${c.username || c.user_id}`);
  renderZoteroSettings(SEL);
};
actions.zSetRemove = async () => {
  if (!confirm("断开 Zotero 云端？已导入的文献不受影响。")) return;
  await api("DELETE", "/api/zotero/cloud");
  renderZoteroSettings(SEL);
};
