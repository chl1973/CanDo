// 能力中心：智能体能做什么，一页看清。内置能力（按用途分组，标出现在能不能用）+ 合作者接入的外部工具（每个工具一张卡片）。
// 这里只是展示和入口：点“用它”把示例填进智能体的输入框；真正的调用、确认、写入仍然在智能体里按原来的规则进行。
import { $, $$, esc, api, toast, modal, closeModal, actions } from "/core.js";

const C = { data: null, hooks: null, q: "" };
const ic = (id, cls = "") => `<svg class="ic ${cls}"><use href="#${id}"/></svg>`;

export async function renderCapCenter(box, hooks) {
  C.hooks = hooks; // { toAgent(text), toFolders() }
  C.box = box;
  C.data = await api("GET", "/api/agent/capabilities");
  draw();
}

async function reload(fresh) {
  C.data = await api("GET", "/api/agent/capabilities" + (fresh ? "?refresh=1" : ""));
  if (C.box && C.box.isConnected) draw();
}

function draw() {
  const d = C.data, n = d.counts;
  C.box.innerHTML = `<div class="card cchero">
      <div class="cchead"><span class="ccmark">${ic("i-grid")}</span>
        <div><h3>能力中心</h3><p>智能体能做的事都在这里。自带的能力开箱即用；合作的同学用 Python 等写的工具，接进来就多一项能力，不用改工作台。</p></div></div>
      <div class="ccstats">
        <div class="ccstat"><span class="v">${n.builtin}</span><span class="l">项内置能力${n.ready < n.builtin ? ` · ${n.ready} 项现在可用` : " · 全部可用"}</span></div>
        <div class="ccstat ext"><span class="v">${n.external}</span><span class="l">个接入的工具${n.services ? ` · 来自 ${n.services} 个服务` : ""}</span></div>
        <input id="ccFind" data-input="ccFind" placeholder="搜索能力，例如：Word、画图、编译" value="${esc(C.q)}">
      </div>
      ${d.local ? "" : `<div class="msg info">智能体只能在运行工作台的电脑上使用。这里可以先看看它能做什么。</div>`}
    </div>
    ${d.local ? extSection(d) : ""}
    ${d.groups.map(groupCard).join("")}
    <p class="muted ccfoot">安全规则不因工具多少而变：智能体只能碰你授权的文件夹；改文件、保存外部工具生成的文件都要你确认，写入前备份、可以撤销；运行命令和第一次调用外部工具会先问你。</p>`;
  filter();
}

// ---------- 接入的工具 ----------
function extSection(d) {
  const svcs = d.services;
  return `<div class="card ccext" id="ccExt">
    <div class="sechead"><span class="ccgi pri">${ic("i-plug", "sm")}</span><h3>接入的工具</h3><span class="tag pri">可扩展</span><span class="sp"></span>
      <a class="btnlike sm" href="/api/agent/extools/kit" title="接口约定和 Python 示例，发给合作的同学">下载开发包</a>
      <button class="sm pri" data-act="ccAdd">＋ 接入工具服务</button></div>
    ${svcs.length ? svcs.map(svcBlock).join("") : `<div class="ccempty">
        <p><b>还没有接入外部工具。</b>合作的同学可以把画图、往 Word 或 LaTeX 里插图、解析模板这些能力写成一个在这台电脑上运行的小程序，接进来交给智能体使用。</p>
        <ol><li>点“下载开发包”，把它发给写工具的同学（里面有接口说明和 Python 示例）。</li>
          <li>同学在这台电脑上把工具服务运行起来，会得到一个地址，例如 <code>http://127.0.0.1:8765</code>。</li>
          <li>点“接入工具服务”，填上地址。接好后，每个工具在这里显示成一张卡片。</li></ol></div>`}
    <p class="muted" style="margin-bottom:0">外部工具看不到对话和你的资料，也碰不到你的文件夹：文件由工作台读出后交给它，它生成的文件交回工作台，你确认后才保存。只能接这台电脑上的服务。</p></div>`;
}
function svcBlock(s, i) {
  return `<div class="ccsvc" data-i="${i}">
    <div class="row ccsvchead"><b>${esc(s.name)}</b>
      ${s.ok ? `<span class="tag ok">已连接 · ${s.tools.length} 个工具</span>` : `<span class="tag bad" title="${esc(s.error)}">${esc(s.error || "连不上")}</span>`}
      ${s.enabled ? "" : '<span class="tag warn">已停用</span>'}
      <span class="muted">${s.ok && s.service ? esc(s.service) + (s.version ? " " + esc(s.version) : "") + " · " : ""}<code>${esc(s.url)}</code></span><span class="sp"></span>
      <select data-change="ccToggle" data-i="${i}" style="width:auto"><option value="1" ${s.enabled ? "selected" : ""}>启用</option><option value="0" ${s.enabled ? "" : "selected"}>停用</option></select>
      <button class="sm" data-act="ccRefresh">重新检查</button>
      <button class="sm danger" data-act="ccDel" data-i="${i}">移除</button></div>
    ${s.tools.length ? `<div class="ccgrid">${s.tools.map((t) => toolCard(s, i, t)).join("")}</div>`
      : `<p class="muted">${s.ok ? "这个服务还没有提供工具。" : "现在连不上这个服务。请确认它已经在这台电脑上运行，然后点“重新检查”。"}</p>`}</div>`;
}
function toolCard(s, i, t) {
  const ps = (t.params || []).filter((p) => p.desc || p.required).slice(0, 4);
  const usable = s.enabled && s.ok;
  const ex = t.example || `用“${t.title}”工具：`;
  return `<div class="cccard ext ${usable ? "" : "off"}" data-name="${esc((t.title + " " + t.description + " " + t.full + " " + s.name + " 外部").toLowerCase())}">
    <div class="cct"><b>${esc(t.title)}</b><span class="tag pri">外部 · ${esc(s.name)}</span></div>
    <div class="ccd">${esc(t.description)}</div>
    ${ps.length ? `<div class="ccp">需要：${ps.map((p) => esc(p.desc || p.name) + (p.file ? "（文件）" : "") + (p.required ? "" : "（可选）")).join("；")}</div>` : ""}
    ${t.example ? `<div class="ccex"><span>试试说</span>${esc(t.example)}</div>` : ""}
    <div class="ccsafe">${ic("i-shield", "sm")}${t.always ? "已设为始终允许，调用时不再询问" : "第一次调用会先问你"}${t.makes_files ? "；生成的文件要你确认后才保存" : ""}</div>
    <div class="row ccact"><code class="muted">${esc(t.full)}</code><span class="sp"></span>
      ${t.always ? `<button class="sm" data-act="ccRevoke" data-i="${i}" data-t="${esc(t.name)}">改回每次询问</button>` : ""}
      <button class="sm pri" data-act="ccUse" data-ex="${esc(ex)}" ${usable ? "" : "disabled"}>用它</button></div></div>`;
}

// ---------- 内置能力 ----------
function groupCard(g) {
  return `<div class="card ccgroup"><div class="sechead"><span class="ccgi">${ic(g.icon, "sm")}</span><h3>${esc(g.name)}</h3><span class="muted">${g.items.length} 项</span></div>
    <div class="ccgrid">${g.items.map(itemCard).join("")}</div></div>`;
}
function itemCard(it) {
  const local = C.data.local;
  return `<div class="cccard ${it.ready ? "" : "off"}" data-name="${esc((it.title + " " + it.desc + " " + it.example).toLowerCase())}">
    <div class="cct"><b>${esc(it.title)}</b>${it.ready ? '<span class="tag ok">可用</span>' : '<span class="tag warn">需要设置</span>'}</div>
    <div class="ccd">${esc(it.desc)}</div>
    <div class="ccex"><span>试试说</span>${esc(it.example)}</div>
    ${it.safe ? `<div class="ccsafe">${ic("i-shield", "sm")}${esc(it.safe)}</div>` : ""}
    ${it.ready ? "" : `<div class="ccneed">${esc(it.need)}${local && it.link ? ` <a href="#" data-act="ccGo" data-link="${esc(it.link)}">去设置</a>` : ""}</div>`}
    ${local ? `<div class="row ccact"><span class="sp"></span><button class="sm ${it.ready ? "pri" : ""}" data-act="ccUse" data-ex="${esc(it.example)}" ${it.ready ? "" : "disabled"}>用它</button></div>` : ""}</div>`;
}

// ---------- 搜索 ----------
function filter() {
  const q = C.q.trim().toLowerCase();
  $$(".cccard", C.box).forEach((c) => c.classList.toggle("hide", !!q && !c.dataset.name.includes(q)));
  $$(".ccgroup, .ccsvc", C.box).forEach((g) => g.classList.toggle("hide", !!q && !g.querySelector(".cccard:not(.hide)")));
}
actions.ccFind = (el) => { C.q = el.value; filter(); };

// ---------- 操作 ----------
actions.ccUse = (el) => C.hooks.toAgent(el.dataset.ex);
actions.ccGo = (el) => {
  if (el.dataset.link === "agent:folders") return C.hooks.toFolders();
  location.hash = el.dataset.link;
};
actions.ccAdd = () => {
  modal(`<h3>接入工具服务</h3>
    <p class="muted">工具服务是合作的同学写的、在<b>这台电脑上</b>运行的小程序。运行后它会显示一个地址，填到下面。</p>
    <form data-submit="ccAddGo">
      <label class="f">短名（给这个服务起个简短的英文名，例如 fig）</label>
      <input name="name" required placeholder="fig" pattern="[a-z][a-z0-9_]{0,15}" title="小写英文字母开头，只含小写字母、数字、下划线">
      <label class="f">地址</label>
      <input name="url" required placeholder="http://127.0.0.1:8765">
      <p class="muted">只能填这台电脑上的地址（127.0.0.1 或 localhost）。还没有工具服务？先 <a href="/api/agent/extools/kit">下载开发包</a>，里面有可以直接运行的 Python 示例。</p>
      <div class="row" style="margin-top:12px"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button class="pri" type="submit">接入</button></div>
    </form>`);
};
async function save(list) {
  await api("PUT", "/api/agent/extools", { services: list.map((s) => ({ name: s.name, url: s.url, enabled: !!s.enabled, allow: s.allow || [] })) });
  await reload(false);
}
actions.ccAddGo = async (f) => {
  const name = f.name.value.trim().toLowerCase();
  await save([...C.data.services, { name, url: f.url.value.trim(), enabled: true, allow: [] }]);
  closeModal();
  const s = C.data.services.find((x) => x.name === name);
  toast(s && s.ok ? `已接入，发现 ${s.tools.length} 个工具` : "已保存，但现在连不上：" + (s ? s.error : ""), !(s && s.ok));
};
actions.ccDel = (el) => {
  const s = C.data.services[Number(el.dataset.i)];
  if (!confirm(`移除工具服务“${s.name}”？它的工具会从智能体里消失；以后可以再接入。`)) return;
  return save(C.data.services.filter((_, i) => i !== Number(el.dataset.i)));
};
actions.ccToggle = (el) => { const l = C.data.services.map((s) => ({ ...s })); l[Number(el.dataset.i)].enabled = el.value === "1"; return save(l); };
actions.ccRevoke = (el) => { const l = C.data.services.map((s) => ({ ...s })); const s = l[Number(el.dataset.i)]; s.allow = (s.allow || []).filter((t) => t !== el.dataset.t); return save(l).then(() => toast("已改回每次询问")); };
actions.ccRefresh = async () => { await reload(true); toast("已重新检查"); };
