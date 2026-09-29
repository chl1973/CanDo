import { $, $$, esc, api, toast, modal, closeModal, actions, showErr, fmtTime, formData, ROLE, STAGE_ST, setUnauthorizedHandler } from "/core.js";
import { renderMaterials, renderAsk, showCite, CITES } from "/kb.js";
import { renderChecks, renderCheckDetail } from "/check.js";
import { renderPapers, renderScholarSettings } from "/papers.js";
import { renderMyModels, PRESETS, checkTag, checkReport } from "/models.js";
import { renderZoteroSettings } from "/zotero.js";
import { pageAssistant } from "/assistant.js";
import { pageUsage } from "/usage.js";
import { renderPaperLib } from "/paperlib.js";
import { pageWriting } from "/writing.js";
import { startJobCenter } from "/jobs.js";
import "/read.js";
import "/wizard.js";

export const S = { me: null, users: null, templates: null, proj: null, stageSel: null, tab: "flow" };

setUnauthorizedHandler(() => { if (S.me) { S.me = null; location.hash = "#/login"; boot(); } });

// ---------------- 启动与路由 ----------------

async function boot() {
  let h;
  try { h = await api("GET", "/api/health"); } catch (e) { $("#main").innerHTML = `<div class="card center-card"><div class="msg err">${esc(e.message)}</div></div>`; return; }
  if (!h.setup_done) return renderSetup(h.local);
  try { S.me = await api("GET", "/api/me"); } catch { S.me = null; }
  if (!S.me) return renderLogin(h);
  document.body.classList.add("authed");
  document.body.classList.remove("guest");
  window.KY_ME_ID = S.me.id;
  $("#nav").classList.remove("hide");
  setOrg(S.me.org_name);
  $("#who").innerHTML = `<span class="wn">${esc(S.me.name)} · ${esc(ROLE[S.me.role])}</span><a class="avatar" href="#/settings" title="${esc(S.me.name)}">${esc([...(S.me.name || "?")].slice(-1)[0] || "?")}</a>`;
  $("#navFoot").innerHTML = `<b>CanDo</b> 可为 · v${esc(h.version || "")}`;
  $("#navAdmin").classList.toggle("hide", S.me.role !== "admin");
  if (!location.hash || location.hash === "#/login" || location.hash === "#/setup") location.hash = "#/";
  route();
  offerHomeScreen();
  startJobCenter();
}

window.addEventListener("hashchange", () => { if (S.me) route(); });

function route() {
  const parts = location.hash.replace(/^#\/?/, "").split("/").map(decodeURIComponent);
  const [page, a, b, c] = parts;
  const cur = { "": "home", p: "projects", check: "projects", usage: "settings" }[page] ?? page;
  $$("#nav a").forEach((x) => x.classList.toggle("on", x.dataset.nav === cur || (x.dataset.nav === "more" && MORE_PAGES.includes(cur))));
  closeModal();
  window.scrollTo(0, 0);
  const r = {
    "": pageHome, projects: pageProjects, p: () => pageProject(a, b, c), mine: () => pageMine(a), library: () => (a ? pageLibraryItem(a) : pageLibrary()),
    settings: pageSettings, assistant: () => pageAssistant($("#main"), a), writing: () => pageWriting($("#main"), a), usage: () => pageUsage($("#main"), a), check: () => renderCheckDetail($("#main"), a), admin: pageAdmin,
  }[page];
  (r || pageHome)().catch(showErr);
}
// 苹果手机用 Safari 打开时，提示“添加到主屏幕”（像 App 一样全屏使用）；关掉后 30 天内不再提示
function offerHomeScreen() {
  const ua = navigator.userAgent || "";
  const ios = /iPhone|iPad|iPod/.test(ua) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  const standalone = navigator.standalone === true || (window.matchMedia && matchMedia("(display-mode: standalone)").matches);
  if (!ios || standalone || window.KyApp) return;
  let until = 0;
  try { until = Number(localStorage.getItem("a2hsHideUntil") || 0); } catch { until = 0; }
  if (Date.now() < until || document.querySelector(".a2hs")) return;
  const safari = /Safari\//.test(ua) && !/CriOS|FxiOS|EdgiOS|MicroMessenger|QQ\//.test(ua);
  const d = document.createElement("div");
  d.className = "a2hs";
  d.innerHTML = safari
    ? `<button class="x" data-act="a2hsClose" aria-label="关闭">×</button><b>把工作台装到桌面</b><div class="muted" style="margin-top:4px">点屏幕底部的分享按钮 <span class="shareico"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"/><path d="M8 7l4-4 4 4"/><path d="M6 11H5a1 1 0 0 0-1 1v8a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-8a1 1 0 0 0-1-1h-1"/></svg></span>，选“添加到主屏幕”。以后从桌面图标打开，全屏使用，和 App 一样。</div>`
    : `<button class="x" data-act="a2hsClose" aria-label="关闭">×</button><b>想装到桌面？</b><div class="muted" style="margin-top:4px">请用 Safari 打开这个地址（微信里点右上角“…” → “在默认浏览器中打开”），再点分享按钮 → “添加到主屏幕”。</div>`;
  document.body.appendChild(d);
}
actions.a2hsClose = (el) => {
  try { localStorage.setItem("a2hsHideUntil", String(Date.now() + 30 * 864e5)); } catch { /* 无痕模式等 */ }
  el.closest(".a2hs").remove();
};

// ---------------- 外观（浅色 / 深色 / 跟随系统）与手机端“更多” ----------------
const MORE_PAGES = ["assistant", "library", "admin", "settings"];
const THEME_NAME = { auto: "跟随系统", light: "浅色", dark: "深色" };
const THEME_ICON = { auto: "i-auto", light: "i-sun", dark: "i-moon" };
function getTheme() { try { return localStorage.getItem("kyTheme") || "auto"; } catch { return "auto"; } }
export function setTheme(t) {
  try { localStorage.setItem("kyTheme", t); } catch { /* 无痕模式等 */ }
  if (t === "auto") document.documentElement.removeAttribute("data-theme");
  else document.documentElement.setAttribute("data-theme", t);
  paintThemeBtn();
  $$(".themeseg button").forEach((b) => b.classList.toggle("on", b.dataset.t === t));
}
function paintThemeBtn() {
  const t = getTheme(), b = $("#themeBtn");
  if (!b) return;
  b.title = `外观：${THEME_NAME[t]}（点击切换）`;
  b.innerHTML = `<svg class="ic"><use href="#${THEME_ICON[t]}"/></svg>`;
}
export const themeSeg = () => `<div class="themeseg">${["auto", "light", "dark"].map((t) => `<button type="button" class="${getTheme() === t ? "on" : ""}" data-act="themeSet" data-t="${t}"><svg class="ic sm"><use href="#${THEME_ICON[t]}"/></svg>${THEME_NAME[t]}</button>`).join("")}</div>`;
actions.themeCycle = () => { const order = ["auto", "light", "dark"]; const t = order[(order.indexOf(getTheme()) + 1) % 3]; setTheme(t); toast(`外观：${THEME_NAME[t]}`); };
actions.themeSet = (el) => setTheme(el.dataset.t);
paintThemeBtn();

const ni = (id) => `<svg class="ni"><use href="#${id}"/></svg>`;
actions.navMore = () => {
  const items = [["#/assistant", "i-assistant", "AI 助手"], ["#/library", "i-award", "经验库"], ["#/writing/check", "i-check", "格式检查"], ["#/mine/research", "i-radar", "深度调研"], ["#/settings", "i-settings", "设置"]];
  if (S.me && S.me.role === "admin") items.splice(4, 0, ["#/admin", "i-shield", "后台"]);
  modal(`<h3>更多</h3><div class="sheetgrid">${items.map(([h, i, n]) => `<a href="${h}">${ni(i)}${n}</a>`).join("")}</div>
    <p class="muted" style="margin:18px 0 0">外观</p>${themeSeg()}`);
};

// 顶栏：CanDo 标志 + 课题组名称（没有设置名称时只显示标志）
function setOrg(name) {
  const n = (name || "").trim();
  $("#orgName").textContent = n;
  $("#orgName").classList.toggle("hide", !n);
  $("#orgSep").classList.toggle("hide", !n);
}

export const go = (h) => { if (location.hash === h) route(); else location.hash = h; };

// ---------------- 首次设置 / 登录 ----------------

const ic = (id, cls = "") => `<svg class="ic ${cls}"><use href="#${id}"/></svg>`;

// 登录和首次设置页：左侧品牌介绍，右侧表单（手机上只显示表单）
function authShell(inner) {
  document.body.classList.remove("authed");
  document.body.classList.add("guest");
  $("#nav").classList.add("hide");
  $("#who").textContent = "";
  const feats = [["i-projects", "项目按阶段推进", "科研、大创 / 挑战杯、数学建模模板，老师逐阶段审核"],
    ["i-library", "论文库 + 原文问答", "上传、检索、OCR、深度调研，每个回答都能追到原文"],
    ["i-writing", "带新同学写论文", "论文向导、AI 起草（逐句标依据）、格式与投稿规范检查"],
    ["i-shield", "数据留在本机", "账号、论文、草稿都保存在这台电脑上"]];
  $("#main").innerHTML = `<div class="auth">
    <aside class="authside"><div class="authlogo"><span class="authmk"><svg><use href="#i-cando"/></svg></span><span class="wm">CanDo</span><span class="zh">可为</span></div>
      <h1>Everyone can do research.<span>人人都能做科研。</span></h1>
      <p class="lead">从选题、读文献、做实验，到写论文、查格式、交成果，每一步都有记录、有依据。</p>
      <ul class="authfeat">${feats.map(([i, t, d]) => `<li>${ic(i)}<div><b>${t}</b><span>${d}</span></div></li>`).join("")}</ul>
      <div class="authnote">支持 Windows 电脑、安卓 App、iPhone；同一 Wi-Fi 下手机扫码即可使用。</div></aside>
    <section class="authmain"><div class="authcard"><div class="authmlogo"><img src="/icon-192.png" alt=""><span class="wm">CanDo</span><span class="muted" style="font-size:15px">可为</span></div>${inner}</div></section></div>`;
}

function renderSetup(local) {
  if (!local) {
    authShell(`<h2>还没有完成初始设置</h2><p class="sub">请在安装工作台的电脑上打开程序，完成首次设置后再用手机访问。</p>`);
    return;
  }
  authShell(`<h2>欢迎使用</h2>
    <p class="sub">第一次使用，请创建管理员账号（通常是指导老师）。之后可在“设置 → 成员”中为学生和其他老师添加账号。</p>
    <form data-submit="doSetup">
      <label class="f">课题组 / 团队名称（显示在左上角，可不填）</label><input name="org_name" placeholder="例如：王老师课题组">
      <label class="f">你的姓名</label><input name="name" required placeholder="例如：王老师">
      <label class="f">登录用户名</label><input name="username" required autocomplete="username" placeholder="例如：wang">
      <div class="grid2"><div><label class="f">密码（至少 6 位）</label><input name="password" type="password" required minlength="6" autocomplete="new-password"></div>
      <div><label class="f">再次输入密码</label><input name="password2" type="password" required minlength="6" autocomplete="new-password"></div></div>
      <p class="muted" style="margin-top:10px">所有数据只保存在这台电脑上。请记住密码；忘记了可以用开始菜单里的“忘记管理员密码”重置。</p>
      <button class="pri" type="submit">完成设置</button>
    </form>`);
}
actions.doSetup = async (f) => {
  const d = formData(f);
  if (d.password !== d.password2) return toast("两次输入的密码不一致", true);
  await api("POST", "/api/setup", d);
  toast("设置完成");
  location.hash = "#/";
  boot();
};

function renderLogin(h = {}) {
  const hint = h.local && h.admin_usernames && h.admin_usernames.length
    ? `<div class="msg info">本机管理员用户名：<b>${h.admin_usernames.map(esc).join("、")}</b></div>` : "";
  authShell(`<h2>登录</h2><p class="sub">欢迎回来，登录后继续你的项目和论文。</p>${hint}
    <form data-submit="doLogin">
      <label class="f">用户名</label><input name="username" required autocomplete="username">
      <label class="f">密码</label><input name="password" type="password" required autocomplete="current-password">
      <button class="pri" type="submit">登录</button>
    </form>
    <p class="muted" style="margin-top:16px">没有账号？请联系指导老师在“设置 → 成员”中为你创建。
    <button class="linkbtn" type="button" data-act="forgotPw">忘记密码？</button></p>`);
}
actions.forgotPw = () => modal(`<h3>忘记密码怎么办</h3>
  <p><b>学生或其他老师：</b>请管理员在“设置 → 成员”中点“重置密码”。</p>
  <p><b>管理员：</b>在运行工作台的电脑上，点开始菜单 → <b>CanDo 可为</b> → <b>忘记管理员密码</b>。系统会弹窗显示你的用户名和一个临时密码，用它登录后再改成自己的密码。所有数据都不受影响。</p>
  <p class="muted">免安装版：在程序所在文件夹按住 Shift 右键 → “在终端中打开”，输入 <code>.\\CanDo.exe -reset-admin</code>。</p>`);
actions.doLogin = async (f) => {
  await api("POST", "/api/login", formData(f));
  location.hash = "#/";
  boot();
};

// ---------------- 首页 ----------------

const isTeacher = () => S.me && (S.me.role === "admin" || S.me.role === "teacher");

function progressBar(done, total) {
  const pct = total ? Math.round((done / total) * 100) : 0;
  return `<div class="progress" title="${done}/${total}"><div style="width:${pct}%"></div></div>`;
}

const KIND = { research: "课题组科研", dachuang: "大创 / 挑战杯", modeling: "数学建模", general: "通用" };
function projectCard(p) {
  const pct = p.stage_total ? Math.round((p.stage_done / p.stage_total) * 100) : 0;
  return `<div class="card pcard" data-act="openProject" data-id="${esc(p.id)}" tabindex="0">
    <div class="row" style="gap:8px"><span class="tag pri kind">${esc(KIND[p.kind] || "项目")}</span><span class="sp"></span>${p.status === "archived" ? '<span class="tag">已归档</span>' : ""}
      ${p.pending_reviews && p.manage ? `<span class="tag warn">${p.pending_reviews} 个待审核</span>` : ""}</div>
    <h3 style="margin:10px 0 0">${esc(p.name)}</h3>
    <div class="pmeta"><span>${ic("i-users", "sm")} ${esc(p.advisors.map((a) => a.name).join("、") || "—")} 指导 · ${p.member_count} 名成员</span></div>
    ${progressBar(p.stage_done, p.stage_total)}
    <div class="pfoot"><span>当前：${esc(p.current_stage)}</span><span class="num">${p.stage_done}/${p.stage_total} · ${pct}%</span></div></div>`;
}
actions.openProject = (el) => go(`#/p/${el.dataset.id}`);

const WEEK = "日一二三四五六";
function greet() {
  const h = new Date().getHours();
  return h < 6 ? "夜深了" : h < 11 ? "早上好" : h < 14 ? "中午好" : h < 18 ? "下午好" : "晚上好";
}
const TODO = {
  review: ["i-inbox", "warn", "待审核"], returned: ["i-alert", "bad", "被退回，需要修改"],
  overdue: ["i-clock", "bad", "已逾期"], due: ["i-clock", "pri", "快到期"],
  contract_review: ["i-writing", "warn", "请你审阅论文契约"], contract_returned: ["i-alert", "bad", "论文契约被老师退回"],
};
function ago(t) {
  const d = (Date.now() - new Date(t).getTime()) / 1000;
  if (!(d >= 0)) return "";
  if (d < 60) return "刚刚";
  if (d < 3600) return `${Math.floor(d / 60)} 分钟前`;
  if (d < 86400) return `${Math.floor(d / 3600)} 小时前`;
  if (d < 86400 * 30) return `${Math.floor(d / 86400)} 天前`;
  return fmtTime(t).slice(0, 10);
}

async function pageHome() {
  const [ps, hm] = await Promise.all([api("GET", "/api/projects"), api("GET", "/api/home")]);
  const active = ps.filter((p) => p.status === "active");
  const st = hm.stats || {};
  const now = new Date();
  let banners = "";
  if (S.me.must_change_pw) banners += `<div class="msg">你正在使用老师设置的初始密码，请先 <a href="#/settings">修改密码</a>。</div>`;
  if (!S.me.llm_configured) banners += `<div class="msg info">${ic("i-assistant", "sm")} 还没有可用的 AI 模型：论文库、项目流程都能正常用；AI 速读、问答、起草需要先接入模型。<a href="#/settings">去接入（约 3 分钟）</a></div>`;
  let guide = "";
  if (!ps.length) {
    guide = isTeacher()
      ? `<div class="card"><h3>三步开始</h3><ol style="margin:0;padding-left:20px;line-height:2">
          <li>在 <a href="#/settings">设置 → 成员</a> 中为学生创建账号</li>
          <li>点击右上角“新建项目”，选择流程模板（科研 / 大创 / 数学建模 / 通用）</li>
          <li>在 <a href="#/settings">设置 → 手机与同学访问</a> 中开启访问，让学生用手机扫码加入</li></ol></div>`
      : `<div class="card empty">${ic("i-projects")}你还没有加入任何项目。请联系指导老师把你加入项目；论文库和写论文现在就可以用。</div>`;
  }
  const kpi = (href, cls, icon, v, l) => `<a class="kpi ${cls}" href="${href}"><span class="kic">${ic(icon)}</span><div><div class="v">${v}</div><div class="l">${l}</div></div></a>`;
  const four = isTeacher()
    ? kpi("#/projects", "k4", "i-inbox", st.pending_reviews || 0, "项待我审核")
    : kpi("#/projects", "k4", "i-inbox", (hm.todos || []).length, "件待办");
  const quick = [["#/mine", "i-upload", "上传论文"], ["#/writing/draft", "i-writing", "AI 起草"], ["#/writing/check", "i-check", "格式检查"], ["#/assistant", "i-formula", "图片转 LaTeX"], ["#/mine/research", "i-radar", "深度调研"]];
  const todos = (hm.todos || []).map((t) => {
    const [i, c, n] = TODO[t.kind] || TODO.due;
    if (t.kind.startsWith("contract_")) return `<a class="lrow" href="#/writing/contract" data-act="homeContract" data-id="${esc(t.id)}"><span class="lic ${c}">${ic(i, "sm")}</span><div><div class="lt">${esc(t.title)}</div><div class="ls">${n} · ${esc(t.who || "")}</div></div></a>`;
    return `<a class="lrow" href="#/p/${esc(t.project_id)}/flow/${esc(t.stage_id)}"><span class="lic ${c}">${ic(i, "sm")}</span><div><div class="lt">${esc(t.stage)}</div><div class="ls">${esc(t.project)} · ${n}${t.due ? ` · 截止 ${esc(t.due)}` : ""}</div></div></a>`;
  }).join("");
  const papers = (hm.recent_papers || []).map((m) => `<a class="lrow" href="#/mine"><span class="lic pri">${ic("i-file", "sm")}</span><div><div class="lt">${esc(m.title)}</div><div class="ls">${esc(m.author || m.uploader || "")}${m.owner_id !== S.me.id ? " · 全组共享" : ""} · ${ago(m.uploaded_at)}</div></div></a>`).join("");
  const drafts = (hm.recent_drafts || []).map((d) => `<a class="lrow" href="#/writing/draft" data-act="homeDraft" data-id="${esc(d.id)}"><span class="lic">${ic("i-writing", "sm")}</span><div><div class="lt">${esc(d.section)} · ${esc(d.idea || "（无标题）")}</div><div class="ls">${esc(d.profile)} · ${d.stage === "draft" ? "已起草" : "论证骨架"} · ${ago(d.updated_at)}</div></div></a>`).join("");
  $("#main").innerHTML = `<div class="hello"><div><h2>${greet()}，${esc(S.me.name)}</h2><div class="date">${now.getMonth() + 1} 月 ${now.getDate()} 日 · 星期${WEEK[now.getDay()]}${S.me.org_name ? " · " + esc(S.me.org_name) : ""}</div></div><span class="sp"></span>
      ${isTeacher() ? `<button class="pri" data-act="newProject">＋ 新建项目</button>` : ""}</div>
    ${banners}
    <div class="kpis">${kpi("#/projects", "k1", "i-projects", st.projects || 0, "个进行中的项目")}${kpi("#/mine", "k2", "i-library", (st.papers_mine || 0), `篇我的论文${st.papers_shared ? ` · 全组共享 ${st.papers_shared}` : ""}`)}${kpi("#/writing/draft", "k3", "i-writing", st.drafts || 0, "份 AI 草稿")}${four}</div>
    <div class="quick">${quick.map(([h, i, n]) => `<a class="qa" href="${h}">${ic(i)}${n}</a>`).join("")}</div>
    ${guide}
    <div class="homegrid"><div>
      ${active.length ? `<div class="sechead"><h3>进行中的项目</h3><span class="sp"></span><a href="#/projects">全部项目 ${ic("i-arrow", "sm")}</a></div><div class="cards">${active.slice(0, 6).map(projectCard).join("")}</div>` : ""}
    </div><div>
      <div class="card"><div class="sechead"><h3>待我处理</h3></div>${todos || `<div class="nothing">没有需要处理的事项。</div>`}</div>
      <div class="card"><div class="sechead"><h3>最近的论文</h3><span class="sp"></span><a href="#/mine">论文库 ${ic("i-arrow", "sm")}</a></div>${papers || `<div class="nothing">还没有论文。<a href="#/mine">上传第一篇</a></div>`}</div>
      <div class="card"><div class="sechead"><h3>最近的草稿</h3><span class="sp"></span><a href="#/writing/draft">AI 起草 ${ic("i-arrow", "sm")}</a></div>${drafts || `<div class="nothing">还没有草稿。写下想法，AI 帮你搭论证骨架。</div>`}</div>
    </div></div>`;
}
actions.homeContract = (el) => { window.KY_OPEN_CONTRACT = el.dataset.id; go("#/writing/contract"); };
actions.homeDraft = (el) => { window.KY_OPEN_DRAFT = el.dataset.id; go("#/writing/draft"); };

// ---------------- 项目列表 / 新建 ----------------

async function pageProjects() {
  const ps = await api("GET", "/api/projects");
  const act = ps.filter((p) => p.status === "active"), arc = ps.filter((p) => p.status !== "active");
  $("#main").innerHTML = `<div class="row" style="margin-bottom:12px"><h2 style="margin:0">项目</h2><span class="sp"></span>
      ${isTeacher() ? '<button class="pri" data-act="newProject">＋ 新建项目</button>' : ""}</div>
    ${act.length ? `<div class="cards">${act.map(projectCard).join("")}</div>` : `<div class="card empty">暂无进行中的项目</div>`}
    ${arc.length ? `<h3 style="margin:18px 0 10px">已归档</h3><div class="cards">${arc.map(projectCard).join("")}</div>` : ""}`;
}

async function loadUsers() { S.users = await api("GET", "/api/users"); return S.users; }

function peoplePicker(name, users, selected, filter) {
  const list = users.filter(filter).filter((u) => !u.disabled);
  if (!list.length) return `<p class="muted">暂无可选的人员</p>`;
  return `<div style="max-height:180px;overflow:auto;border:1px solid var(--line);border-radius:8px;padding:6px 10px">${list
    .map((u) => `<label style="display:block;padding:3px 0"><input type="checkbox" name="${name}" value="${u.id}" ${selected.includes(u.id) ? "checked" : ""}>${esc(u.name)} <span class="muted">${esc(u.username)} · ${ROLE[u.role]}</span></label>`)
    .join("")}</div>`;
}

actions.newProject = async (el) => {
  const [tpls, users] = await Promise.all([api("GET", "/api/templates"), loadUsers()]);
  const from = el.dataset.from || "";
  modal(`<h3>新建项目</h3>
    <form data-submit="createProject">
      ${from ? `<input type="hidden" name="from_project_id" value="${esc(from)}"><div class="msg info">将复制经验库项目“${esc(el.dataset.fromName || "")}”的流程（不含提交内容）。</div>` : `
      <label class="f">选择流程模板</label>
      <div class="col">${tpls.map((t, i) => `<label class="card" style="margin:0;padding:10px 12px;cursor:pointer"><input type="radio" name="template_key" value="${esc(t.key)}" ${i === 0 ? "checked" : ""}>
        <b>${esc(t.name)}</b> ${t.built_in ? "" : '<span class="tag pri">自定义</span>'}<div class="muted">${esc(t.desc || "")}</div>
        <div class="muted">${t.stages.map(esc).join(" → ")}</div></label>`).join("")}</div>`}
      <label class="f">项目名称</label><input name="name" required placeholder="例如：2026 大创：校园智能灌溉系统">
      <label class="f">简介（可不填）</label><textarea name="desc" style="min-height:60px"></textarea>
      <label class="f">项目成员（学生）</label>${peoplePicker("members", users, [], (u) => u.role === "student")}
      <label class="f">其他指导老师（你自动成为指导老师）</label>${peoplePicker("advisors", users, [], (u) => u.role !== "student" && u.id !== S.me.id)}
      <div class="row" style="margin-top:14px"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button class="pri" type="submit">创建</button></div>
    </form>`);
};
actions.createProject = async (f) => {
  const fd = new FormData(f);
  const body = { name: fd.get("name"), desc: fd.get("desc"), template_key: fd.get("template_key") || "", from_project_id: fd.get("from_project_id") || "",
    members: fd.getAll("members").map(Number), advisors: fd.getAll("advisors").map(Number) };
  const p = await api("POST", "/api/projects", body);
  closeModal();
  toast("项目已创建");
  go(`#/p/${p.id}`);
};

// ---------------- 项目详情 ----------------

const TABS = [["flow", "流程"], ["materials", "论文库"], ["ask", "问答"], ["checks", "引用核验"], ["activity", "动态"], ["manage", "管理"]];

async function pageProject(id, tab, stageId) {
  S.proj = await api("GET", `/api/projects/${id}`);
  S.tab = tab || "flow";
  if (stageId) S.stageSel = stageId;
  else if (!S.stageSel || !S.proj.stages.find((s) => s.id === S.stageSel)) {
    const cur = S.proj.stages.find((s) => s.status !== "done") || S.proj.stages[0];
    S.stageSel = cur ? cur.id : null;
  }
  renderProject();
}

function renderProject() {
  const p = S.proj;
  const tabs = TABS.filter(([k]) => k !== "manage" || p.manage);
  $("#main").innerHTML = `
    <div class="row"><a href="#/projects" class="muted">← 项目</a></div>
    <div class="row" style="margin:6px 0 2px"><h2 style="margin:0">${esc(p.name)}</h2>${p.status === "archived" ? '<span class="tag">已归档（只读）</span>' : ""}</div>
    <p class="muted">${esc(p.advisors.map((a) => a.name).join("、"))} 指导 · 成员：${esc(p.members.map((m) => m.name).join("、") || "暂无")}</p>
    ${p.desc ? `<p class="pre">${esc(p.desc)}</p>` : ""}
    <div class="tabs">${tabs.map(([k, n]) => `<button class="${S.tab === k || (k === "materials" && S.tab === "papers") ? "on" : ""}" data-act="tab" data-tab="${k}">${n}</button>`).join("")}</div>
    <div id="tabBody"></div>`;
  const body = $("#tabBody");
  ({
    flow: () => renderFlow(body),
    materials: () => renderPaperLib(body, p.id, { readonly: p.status === "archived", manage: p.manage }),
    papers: () => renderPaperLib(body, p.id, { readonly: p.status === "archived", manage: p.manage }, "add"),
    ask: () => renderAsk(body, p.id),
    checks: () => renderChecks(body, p.id),
    activity: () => renderActivity(body),
    manage: () => renderManage(body),
  }[S.tab] || (() => renderFlow(body)))();
}
actions.tab = (el) => { S.tab = el.dataset.tab; history.replaceState(null, "", `#/p/${S.proj.id}/${S.tab}`); renderProject(); };

function pipeline(p) {
  return `<div class="pipe">${p.stages.map((s, i) => `${i ? '<div class="step"><div class="bar"></div></div>' : ""}
    <div class="step st-${s.status}"><div class="dot ${s.id === S.stageSel ? "sel" : ""}" data-act="selStage" data-id="${esc(s.id)}">
      <div class="c">${s.status === "done" ? "✓" : i + 1}</div><div class="n">${esc(s.name)}</div></div></div>`).join("")}</div>`;
}
actions.selStage = (el) => { S.stageSel = el.dataset.id; renderFlow($("#tabBody")); };

function renderFlow(body) {
  const p = S.proj;
  const s = p.stages.find((x) => x.id === S.stageSel);
  const ro = p.status === "archived";
  const canWork = (p.is_member || p.manage) && !ro;
  if (!s) { body.innerHTML = `${pipeline(p)}<div class="card empty">还没有阶段${p.manage && !ro ? `，<button class="linkbtn" data-act="addStage" data-after="-1">添加第一个阶段</button>` : ""}</div>`; return; }
  const [stText, stCls] = STAGE_ST[s.status] || ["", ""];
  const idx = p.stages.indexOf(s);
  const timeline = [...s.submissions.map((x) => ({ ...x, kind: "sub" })), ...s.reviews.map((x) => ({ ...x, kind: x.decision }))].sort((a, b) => new Date(a.created_at) - new Date(b.created_at));
  body.innerHTML = `${pipeline(p)}
  <div class="card">
    <div class="row"><h3 style="margin:0">第 ${idx + 1} 阶段：${esc(s.name)}</h3><span class="tag ${stCls}">${stText}</span>${s.due ? `<span class="muted">截止 ${esc(s.due)}</span>` : ""}<span class="sp"></span>
      ${p.manage && !ro ? `<button class="sm" data-act="editStage">编辑</button><button class="sm" data-act="moveStage" data-dir="-1" ${idx === 0 ? "disabled" : ""}>↑</button><button class="sm" data-act="moveStage" data-dir="1" ${idx === p.stages.length - 1 ? "disabled" : ""}>↓</button><button class="sm" data-act="addStage" data-after="${idx}">＋ 在后面加阶段</button>` : ""}</div>
    ${s.goal ? `<p><b>目标：</b>${esc(s.goal)}</p>` : ""}
    ${s.guide ? `<details ${s.status === "todo" ? "open" : ""}><summary class="muted" style="cursor:pointer">怎么做 · 常见坑</summary><p class="pre">${esc(s.guide)}</p></details>` : ""}
    ${s.checklist.length ? `<h4 style="margin-top:12px">交付清单</h4><div class="checklist">${s.checklist.map((c, i) => `<label><input type="checkbox" data-change="toggleCheck" data-i="${i}" ${c.done ? "checked" : ""} ${canWork ? "" : "disabled"}>
      <span>${esc(c.text)}${c.done && c.done_by ? `<span class="muted by">（${esc(c.done_by)}）</span>` : ""}</span></label>`).join("")}</div>` : ""}
    ${canWork && (s.status === "todo" || s.status === "returned") ? `<button class="sm" data-act="startStage" style="margin-top:8px">开始这个阶段</button>` : ""}
  </div>
  <div class="card"><h4>过程记录</h4>
    ${timeline.length ? `<div class="timeline">${timeline.map((t) => `<div class="it ${t.kind}">
      <div class="row"><b>${esc(t.user)}</b><span class="tag ${t.kind === "approve" ? "ok" : t.kind === "return" ? "bad" : t.kind === "sub" ? "pri" : ""}">${{ sub: "提交", approve: "审核通过", return: "退回修改", comment: "评论" }[t.kind]}</span><span class="muted">${fmtTime(t.created_at)}</span></div>
      ${t.content || t.comment ? `<div class="pre">${esc(t.content || t.comment)}</div>` : ""}
      ${t.materials && t.materials.length ? `<div>${t.materials.map((m) => m.deleted ? `<span class="cite del">${esc(m.title)}</span>` : `<a class="cite" href="/api/materials/${esc(m.id)}/file" target="_blank">📎 ${esc(m.title)} v${m.version}</a>`).join("")}</div>` : ""}
    </div>`).join("")}</div>` : `<p class="muted">还没有记录</p>`}
  </div>
  ${canWork && s.status !== "done" && p.is_member ? `<div class="card"><h4>提交进展或成果</h4>
    <form data-submit="submitStage">
      <textarea name="content" placeholder="写下本阶段完成了什么、遇到的问题。成果文件请先上传到“资料”，再在下方勾选关联。"></textarea>
      <div id="subMats" class="muted" style="margin:6px 0">正在加载项目资料…</div>
      <label style="display:block;margin:8px 0"><input type="checkbox" name="request_review" checked>提交给指导老师审核</label>
      <button class="pri" type="submit">提交</button></form></div>` : ""}
  ${p.manage && !ro ? `<div class="card"><h4>指导老师审核</h4>
    <form data-submit="reviewStage"><textarea name="comment" placeholder="审核意见（退回或评论时必填）"></textarea>
    <div class="row" style="margin-top:8px"><button type="submit" class="pri" data-decision="approve">通过本阶段</button><button type="submit" class="danger" data-decision="return">退回修改</button><button type="submit" data-decision="comment">仅评论</button>
    ${p.manage && s.submissions.length === 0 ? '<span class="sp"></span><button type="button" class="sm danger" data-act="delStage">删除本阶段</button>' : ""}</div></form></div>` : ""}`;
  if ($("#subMats")) {
    api("GET", `/api/materials?project_id=${p.id}`).then((ms) => {
      const ok = ms.filter((m) => m.status !== "failed");
      $("#subMats").innerHTML = ok.length ? `关联资料：${ok.map((m) => `<label style="margin-right:12px;color:var(--ink)"><input type="checkbox" name="material_ids" value="${esc(m.id)}">${esc(m.title)} v${m.version}</label>`).join("")}` : "（项目资料库中还没有资料，可在“论文库”上传）";
    }).catch(() => {});
  }
  $$("form[data-submit=reviewStage] button[type=submit]").forEach((b) => b.addEventListener("click", () => { b.form.dataset.decision = b.dataset.decision; }));
}

async function stageCall(path, body) {
  S.proj = await api("POST", `/api/projects/${S.proj.id}/stages/${S.stageSel}${path}`, body);
  renderFlow($("#tabBody"));
}
actions.toggleCheck = (el) => stageCall("/check", { index: Number(el.dataset.i), done: el.checked });
actions.startStage = () => stageCall("/start", {});
actions.submitStage = async (f) => {
  const fd = new FormData(f);
  await stageCall("/submit", { content: fd.get("content"), material_ids: fd.getAll("material_ids"), request_review: fd.get("request_review") === "on" });
  toast(fd.get("request_review") === "on" ? "已提交，等待老师审核" : "已记录");
};
actions.reviewStage = async (f) => {
  const d = f.dataset.decision || "comment";
  await stageCall("/review", { decision: d, comment: new FormData(f).get("comment") });
  toast({ approve: "已通过", return: "已退回", comment: "已评论" }[d]);
};
actions.moveStage = async (el) => {
  S.proj = await api("POST", `/api/projects/${S.proj.id}/stages/${S.stageSel}/move`, { dir: Number(el.dataset.dir) });
  renderFlow($("#tabBody"));
};
actions.delStage = async () => {
  if (!confirm("确定删除这个阶段？")) return;
  S.proj = await api("DELETE", `/api/projects/${S.proj.id}/stages/${S.stageSel}`);
  S.stageSel = null;
  pageProject(S.proj.id, "flow");
};
function stageForm(s, submitAct, extra = "") {
  return `<form data-submit="${submitAct}">${extra}
    <label class="f">阶段名称</label><input name="name" required value="${esc(s.name || "")}">
    <label class="f">目标</label><input name="goal" value="${esc(s.goal || "")}">
    <label class="f">怎么做 · 常见坑</label><textarea name="guide">${esc(s.guide || "")}</textarea>
    <label class="f">交付清单（每行一项）</label><textarea name="checklist">${esc((s.checklist || []).map((c) => c.text || c).join("\n"))}</textarea>
    ${s.id ? `<label class="f">截止日期（可不填）</label><input name="due" type="date" value="${esc(s.due || "")}">` : ""}
    <div class="row" style="margin-top:14px"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button class="pri" type="submit">保存</button></div></form>`;
}
actions.editStage = () => {
  const s = S.proj.stages.find((x) => x.id === S.stageSel);
  modal(`<h3>编辑阶段</h3>${stageForm(s, "saveStage")}`);
};
actions.saveStage = async (f) => {
  const d = formData(f);
  S.proj = await api("PATCH", `/api/projects/${S.proj.id}/stages/${S.stageSel}`, { name: d.name, goal: d.goal, guide: d.guide, due: d.due, checklist: d.checklist.split("\n") });
  closeModal();
  renderFlow($("#tabBody"));
};
actions.addStage = (el) => modal(`<h3>新增阶段</h3>${stageForm({}, "createStage", `<input type="hidden" name="after" value="${esc(el.dataset.after)}">`)}`);
actions.createStage = async (f) => {
  const d = formData(f);
  const before = new Set(S.proj.stages.map((s) => s.id));
  S.proj = await api("POST", `/api/projects/${S.proj.id}/stages`, { name: d.name, goal: d.goal, guide: d.guide, checklist: d.checklist.split("\n"), after: Number(d.after) });
  const added = S.proj.stages.find((s) => !before.has(s.id));
  if (added) S.stageSel = added.id;
  closeModal();
  renderFlow($("#tabBody"));
};

async function renderActivity(body) {
  const acts = await api("GET", `/api/projects/${S.proj.id}/activity`);
  body.innerHTML = `<div class="card"><p class="muted">项目中的每一步操作都会自动留痕，便于追溯。</p>${acts.length ? `<div class="tw"><table><tr><th style="width:150px">时间</th><th style="width:90px">成员</th><th style="width:100px">操作</th><th>内容</th></tr>
    ${acts.map((a) => `<tr><td class="muted">${fmtTime(a.at)}</td><td>${esc(a.user)}</td><td>${esc(a.action)}</td><td>${esc(a.detail)}</td></tr>`).join("")}</table></div>` : '<p class="muted">暂无动态</p>'}</div>`;
}

async function renderManage(body) {
  const p = S.proj;
  const users = await loadUsers();
  const ro = p.status === "archived";
  body.innerHTML = `
  ${ro ? `<div class="card"><h3>项目已归档</h3>${p.retro ? `<p><b>做得好的：</b>${esc(p.retro.good)}</p><p><b>踩过的坑：</b>${esc(p.retro.pitfalls)}</p><p><b>给下一届的建议：</b>${esc(p.retro.advice)}</p>` : ""}
    <p class="muted">${p.share_to_library ? "已共享到经验库，全组成员可以查看各阶段提交内容与复盘（不含资料原文）。" : "未共享到经验库。"}</p>
    <button data-act="unarchive">取消归档</button></div>` : `
  <div class="card"><h3>基本信息与成员</h3>
    <form data-submit="saveProject">
      <label class="f">项目名称</label><input name="name" required value="${esc(p.name)}">
      <label class="f">简介</label><textarea name="desc" style="min-height:60px">${esc(p.desc)}</textarea>
      <label class="f">项目成员</label>${peoplePicker("members", users, p.member_ids, (u) => u.role === "student")}
      <label class="f">指导老师</label>${peoplePicker("advisors", users, p.advisor_ids, (u) => u.role !== "student")}
      <button class="pri" type="submit" style="margin-top:12px">保存</button></form></div>
  <div class="card"><h3>归档与复盘</h3><p class="muted">项目结束后归档。归档后项目变为只读；如共享到经验库，下一届同学可以参考流程与经验，并一键复制流程新建项目。</p>
    <form data-submit="archiveProject">
      <label class="f">做得好的</label><textarea name="good" style="min-height:60px"></textarea>
      <label class="f">踩过的坑</label><textarea name="pitfalls" style="min-height:60px"></textarea>
      <label class="f">给下一届的建议</label><textarea name="advice" style="min-height:60px"></textarea>
      <label style="display:block;margin:10px 0"><input type="checkbox" name="share" checked>共享到经验库（全组可见各阶段提交内容与复盘，不含资料原文）</label>
      <button class="pri" type="submit">归档项目</button></form></div>`}
  ${ro ? "" : `<div class="card" id="projModel"><h3>项目使用的 AI 模型</h3><div class="muted">加载中…</div></div>`}
  <div class="card"><h3>把这个项目的流程另存为模板</h3>
    <form data-submit="saveTemplate" class="row"><input name="name" required placeholder="模板名称，例如：我们组的大创流程" style="flex:1;min-width:200px"><button type="submit">另存为模板</button></form></div>
  ${p.can_delete ? `<div class="card"><h3 style="color:var(--bad)">删除项目</h3><p class="muted">将永久删除项目、阶段记录、项目资料（含原文件）、问答与核验记录，不可恢复。</p><button class="danger" data-act="deleteProject">删除项目</button></div>` : ""}`;
  renderProjModel().catch(showErr);
}
async function renderProjModel() {
  const box = $("#projModel");
  if (!box) return;
  const d = await api("GET", `/api/models?project_id=${encodeURIComponent(S.proj.id)}`);
  const cur = S.proj.model_profile_id || "";
  const mine = d.mine.filter((m) => m.check && m.check.passed);
  const curIsOther = cur && cur !== "team" && !mine.find((m) => m.id === cur);
  box.innerHTML = `<h3>项目使用的 AI 模型</h3>
    <p class="muted">默认“不指定”：每个成员用自己启用的模型，没有就用团队默认模型。指定后，本项目里所有人的问答、核验、拆关键词都使用该模型——指定你自己的模型意味着大家的调用费用记在你的 Key 上。</p>
    <div class="row"><select data-change="setProjModel" style="width:auto;min-width:260px">
      <option value="" ${cur === "" ? "selected" : ""}>不指定（成员各自的模型 / 团队默认）</option>
      <option value="team" ${cur === "team" ? "selected" : ""}>团队默认模型${d.team.configured ? `（${esc(d.team.model)}）` : "（未配置）"}</option>
      ${mine.map((m) => `<option value="${esc(m.id)}" ${cur === m.id ? "selected" : ""}>我的：${esc(m.name)}（${esc(m.model)}）</option>`).join("")}
      ${curIsOther ? `<option value="${esc(cur)}" selected>其他成员提供的模型</option>` : ""}
    </select></div>
    <p class="muted" style="margin-top:6px">当前实际使用：<b>${esc(d.effective.label)}</b></p>
    ${d.mine.length && !mine.length ? '<p class="muted">你的模型还没有通过兼容性自检，自检通过后才能指定给项目。</p>' : ""}`;
}
actions.setProjModel = async (el) => {
  S.proj = await api("PATCH", `/api/projects/${S.proj.id}`, { model_profile_id: el.value });
  toast("已设置项目模型");
  renderProjModel();
};
actions.saveProject = async (f) => {
  const fd = new FormData(f);
  S.proj = await api("PATCH", `/api/projects/${S.proj.id}`, { name: fd.get("name"), desc: fd.get("desc"), members: fd.getAll("members").map(Number), advisors: fd.getAll("advisors").map(Number) });
  toast("已保存");
  renderProject();
};
actions.archiveProject = async (f) => {
  if (!confirm("归档后项目变为只读，确定归档？")) return;
  const fd = new FormData(f);
  S.proj = await api("POST", `/api/projects/${S.proj.id}/archive`, { good: fd.get("good"), pitfalls: fd.get("pitfalls"), advice: fd.get("advice"), share: fd.get("share") === "on" });
  toast("已归档");
  renderProject();
};
actions.unarchive = async () => { S.proj = await api("POST", `/api/projects/${S.proj.id}/unarchive`, {}); toast("已取消归档"); renderProject(); };
actions.saveTemplate = async (f) => { await api("POST", "/api/templates", { project_id: S.proj.id, name: new FormData(f).get("name") }); f.reset(); toast("已保存为模板，新建项目时可选择"); };
actions.deleteProject = async () => {
  const name = prompt(`此操作不可恢复。请输入项目名称“${S.proj.name}”确认删除：`);
  if (name !== S.proj.name) return name === null ? null : toast("名称不一致，未删除", true);
  await api("DELETE", `/api/projects/${S.proj.id}`);
  toast("项目已删除");
  go("#/projects");
};

// ---------------- 我的资料 ----------------

async function pageMine(tab) {
  $("#main").innerHTML = `<h2>论文库</h2><p class="muted">上传 PDF（扫描件可以 OCR）、在线检索或从 Zotero 导入、深度调研，然后基于原文提问和核验引用。论文默认只有自己能看到，可以逐篇共享到全组；项目共用的论文放到对应项目的“论文库”。</p>
    <div class="tabs" id="mineTabs"><button class="on" data-act="mineTab" data-t="m">论文</button><button data-act="mineTab" data-t="a">问答</button><button data-act="mineTab" data-t="c">引用核验</button></div><div id="mineBody"></div>`;
  renderPaperLib($("#mineBody"), "", {}, ["group", "add", "research"].includes(tab) ? tab : undefined);
  if (tab === "ask" || tab === "check") { const b = $(`#mineTabs button[data-t=${tab === "ask" ? "a" : "c"}]`); if (b) actions.mineTab(b); }
}
actions.mineTab = (el) => {
  $$("#mineTabs button").forEach((b) => b.classList.toggle("on", b === el));
  const b = $("#mineBody");
  ({ m: () => renderPaperLib(b, "", {}), a: () => renderAsk(b, ""), c: () => renderChecks(b, "") }[el.dataset.t])();
};

// ---------------- 经验库 ----------------

async function pageLibrary() {
  const items = await api("GET", "/api/library");
  $("#main").innerHTML = `<h2>经验库</h2><p class="muted">往届已归档并共享的项目：可以查看每个阶段的做法、老师意见和复盘，老师可以一键复制流程新建项目。</p>
    ${items.length ? `<div class="cards">${items.map((p) => `<div class="card pcard" data-act="openLib" data-id="${esc(p.id)}"><h3>${esc(p.name)}</h3>
      <p class="muted">${esc(p.advisors.map((a) => a.name).join("、"))} 指导 · 归档于 ${fmtTime(p.archived_at).slice(0, 10)}</p>
      ${p.retro && p.retro.advice ? `<p>💡 ${esc(p.retro.advice.slice(0, 80))}${p.retro.advice.length > 80 ? "…" : ""}</p>` : ""}</div>`).join("")}</div>` : `<div class="card empty">暂时没有共享的项目。项目归档时勾选“共享到经验库”即可出现在这里。</div>`}`;
}
actions.openLib = (el) => go(`#/library/${el.dataset.id}`);

async function pageLibraryItem(id) {
  const p = await api("GET", `/api/library/${id}`);
  $("#main").innerHTML = `<div class="row"><a href="#/library" class="muted">← 经验库</a></div>
    <div class="row" style="margin:6px 0"><h2 style="margin:0">${esc(p.name)}</h2><span class="sp"></span>
      ${isTeacher() ? `<button class="pri" data-act="newProject" data-from="${esc(p.id)}" data-from-name="${esc(p.name)}">用这个流程新建项目</button>` : ""}</div>
    <p class="muted">${esc(p.advisors.map((a) => a.name).join("、"))} 指导 · 成员：${esc(p.members.map((m) => m.name).join("、"))}</p>
    ${p.retro ? `<div class="card"><h3>复盘</h3><p><b>做得好的：</b>${esc(p.retro.good || "—")}</p><p><b>踩过的坑：</b>${esc(p.retro.pitfalls || "—")}</p><p><b>给下一届的建议：</b>${esc(p.retro.advice || "—")}</p></div>` : ""}
    ${p.stages.map((s, i) => `<div class="card"><div class="row"><h3 style="margin:0">${i + 1}. ${esc(s.name)}</h3><span class="tag ${(STAGE_ST[s.status] || [])[1] || ""}">${(STAGE_ST[s.status] || [""])[0]}</span></div>
      ${s.goal ? `<p class="muted">${esc(s.goal)}</p>` : ""}
      ${s.submissions.map((x) => `<div class="frag"><b>${esc(x.user)}</b> <span class="muted">${fmtTime(x.created_at)}${x.material_count ? ` · 关联 ${x.material_count} 份资料` : ""}</span>\n${esc(x.content)}</div>`).join("")}
      ${s.reviews.filter((r) => r.comment).map((r) => `<p><span class="tag ${r.decision === "approve" ? "ok" : r.decision === "return" ? "bad" : ""}">老师意见</span> ${esc(r.comment)}</p>`).join("")}
    </div>`).join("")}`;
}

// ---------------- 管理员后台 ----------------

const fmtSize = (n) => (n > 1 << 20 ? (n / (1 << 20)).toFixed(1) + " MB" : n > 1024 ? (n / 1024).toFixed(0) + " KB" : n + " B");

async function pageAdmin() {
  if (S.me.role !== "admin") { $("#main").innerHTML = `<div class="card empty">只有管理员可以查看后台</div>`; return; }
  const o = await api("GET", "/api/admin/overview");
  const st = o.stats;
  const stat = (label, val, cls = "") => `<div class="stat ${cls}"><span class="muted">${label}</span><b>${val}</b></div>`;
  $("#main").innerHTML = `<h2>后台</h2>
  <div class="card"><h3>总览</h3><div class="row" style="gap:8px">
    ${stat("成员", st.users)}${stat("老师 / 学生", `${st.admins + st.teachers} / ${st.students}`)}
    ${stat("进行中项目", st.projects_active, "ok")}${stat("已归档项目", st.projects_archived)}${stat("待审核阶段", st.pending_reviews, st.pending_reviews ? "warn" : "")}
    ${stat("资料", `${st.materials} 份`)}${stat("解析失败", st.materials_failed, st.materials_failed ? "bad" : "")}${stat("问答记录", st.answers)}${stat("引用核验", st.cite_checks)}
  </div>
  <p class="muted" style="margin-top:10px">AI 模型：${st.llm_configured ? "已配置" : "未配置"} · 手机访问：${st.lan_enabled ? "已开启" : "未开启"} · 自定义模板：${st.templates_custom} 个 · 版本 ${esc(o.version)}</p></div>

  <div class="card"><h3>数据</h3>
    <p>所有数据保存在：<code>${esc(o.data_dir)}</code></p>
    <p class="muted">共 ${o.file_count} 个文件，占用 ${fmtSize(o.data_size)}。其中 data.json 是账号、项目和记录；files 文件夹是上传的原文件；chunks 是提取出的文字片段。</p>
    <div class="row">${o.local ? '<button data-act="openDataDir">打开数据文件夹</button>' : ""}<a href="/api/admin/backup" download><button type="button" class="pri">下载完整备份（.zip）</button></a></div>
    <p class="muted">建议每学期下载一次备份，保存到 U 盘或网盘。备份包含所有账号和资料，请妥善保管。恢复方法见压缩包内的“恢复说明.txt”。</p></div>

  <div class="card"><h3>所有项目</h3>${o.projects.length ? `<div class="tw"><table><tr><th>项目</th><th>指导老师</th><th>成员</th><th>进度</th><th>资料</th><th>最近动态</th></tr>
    ${o.projects.map((p) => `<tr><td><a href="#/p/${esc(p.id)}">${esc(p.name)}</a> ${p.status === "archived" ? '<span class="tag">已归档</span>' : ""}${p.pending_reviews ? ` <span class="tag warn">${p.pending_reviews} 待审核</span>` : ""}</td>
      <td>${esc(p.advisors.map((a) => a.name).join("、"))}</td><td>${esc((p.members || []).join("、") || "—")}</td>
      <td style="min-width:120px">${progressBar(p.stage_done, p.stage_total)}<span class="muted">${p.stage_done}/${p.stage_total} · ${esc(p.current_stage)}</span></td>
      <td>${p.material_count}</td><td class="muted">${p.last_activity ? fmtTime(p.last_activity) : "—"}</td></tr>`).join("")}</table></div>` : '<p class="muted">还没有项目</p>'}</div>

  <div class="card"><h3>最近动态（全部项目）</h3>${o.recent.length ? `<div class="tw"><table><tr><th style="width:140px">时间</th><th>项目</th><th>成员</th><th>操作</th><th>内容</th></tr>
    ${o.recent.map((a) => `<tr><td class="muted">${fmtTime(a.at)}</td><td>${a.project ? `<a href="#/p/${esc(a.project_id)}/activity">${esc(a.project)}</a>` : "—"}</td><td>${esc(a.user)}</td><td>${esc(a.action)}</td><td>${esc(a.detail)}</td></tr>`).join("")}</table></div>` : '<p class="muted">暂无动态</p>'}</div>
  <p class="muted">成员账号管理在 <a href="#/settings">设置 → 成员</a>。</p>`;
}
actions.openDataDir = async () => { await api("POST", "/api/admin/open-data-dir", {}); toast("已在电脑上打开数据文件夹"); };

// ---------------- 设置 ----------------


// 联网搜索（本机智能体用）：博查（国内）或 Tavily
async function renderWebSearch() {
  const el = $("#webCard");
  if (!el) return;
  const st = await api("GET", "/api/websearch/status");
  const provOpts = (sel) => Object.entries(st.providers).map(([k, v]) => `<option value="${k}" ${k === sel ? "selected" : ""}>${esc(v)}</option>`).join("");
  el.innerHTML = `<h3>联网搜索</h3>
    <p class="muted">让“本机智能体”能上网搜资料（比如查竞赛通知、LaTeX 模板写法、软件报错）。学术文献不需要这个，用“检索添加”就行。需要一个搜索服务的密钥：国内推荐<a href="https://open.bochaai.com/" target="_blank" rel="noopener">博查</a>，国外可用 <a href="https://tavily.com/" target="_blank" rel="noopener">Tavily</a>（都有免费额度）。密钥加密保存在这台电脑上。</p>
    <div class="msg ${st.using ? "ok" : "info"}">现在：${{ mine: "使用我的密钥（" + esc(st.providers[st.mine_provider] || "") + "）", team: "使用团队密钥（老师已填写）", "": "还没有设置，智能体不能联网搜索" }[st.using || ""]}</div>
    <form data-submit="webKeySave" data-scope="me" class="row"><select name="provider" style="width:auto">${provOpts(st.mine_provider || "bocha")}</select>
      <input name="key" autocomplete="off" placeholder="${st.mine ? "已填写 " + esc(st.mine) + "（留空并保存 = 删除）" : "粘贴我的搜索密钥"}" style="flex:1;min-width:200px"><button type="submit">${st.mine ? "更新" : "保存"}</button></form>
    ${st.admin ? `<form data-submit="webKeySave" data-scope="team" class="row" style="margin-top:8px"><select name="provider" style="width:auto">${provOpts(st.team_provider || "bocha")}</select>
      <input name="key" autocomplete="off" placeholder="${st.team ? "团队密钥已填写（留空并保存 = 删除）" : "团队搜索密钥：全组共用（管理员）"}" style="flex:1;min-width:200px"><button type="submit">保存团队密钥</button></form>` : ""}`;
}
actions.webKeySave = async (f) => {
  await api("PUT", "/api/websearch/key", { scope: f.dataset.scope, provider: f.provider.value, key: f.key.value.trim() });
  toast(f.key.value.trim() ? "密钥可用，已保存" : "已删除密钥");
  renderWebSearch();
};

async function pageSettings() {
  const me = S.me;
  let html = `<h2>设置</h2>
  ${window.KyApp ? `<div class="card"><h3>手机 App</h3><p>当前连接的电脑：<code>${esc(window.KyApp.server())}</code> · App 版本 ${esc(window.KyApp.version())}</p>
    <button data-act="appChangeServer">更换电脑地址</button></div>` : ""}
  <div class="card"><h3>我的账号</h3><p class="muted">角色：${ROLE[me.role]}</p>
    <form data-submit="saveMe" class="grid2">
      <div><label class="f">显示姓名（别人看到的名字）</label><input name="name" required value="${esc(me.name)}"></div>
      <div><label class="f">登录用户名</label><input name="username" required value="${esc(me.username)}" autocomplete="username"></div>
      <div><button type="submit">保存姓名和用户名</button></div></form>
    <h4 style="margin-top:14px">修改密码</h4>
    <form data-submit="changePw" class="grid2">
      <div><label class="f">原密码</label><input name="old" type="password" required autocomplete="current-password"></div>
      <div><label class="f">新密码（至少 6 位）</label><input name="new" type="password" required minlength="6" autocomplete="new-password"></div>
      <div><button type="submit" class="pri">修改密码</button> <button type="button" data-act="logout">退出登录</button></div></form></div>`;
  html += `<div class="card"><h3>外观</h3><p class="muted" style="margin:0">深色模式适合晚上用；“跟随系统”会随电脑或手机的深色设置自动切换。只对这台设备生效。</p>${themeSeg()}</div>`;
  html += `<div class="card" id="myModels"><h3>我的 AI 模型</h3><div class="muted">加载中…</div></div>
  <div class="card" id="myZotero"><h3>我的 Zotero</h3><div class="muted">加载中…</div></div>
  <div class="card" id="scholarCard"><h3>文献数据库（OpenAlex）</h3><div class="muted">加载中…</div></div>
  <div class="card" id="webCard"><h3>联网搜索</h3><div class="muted">加载中…</div></div>`;
  if (isTeacher()) html += `<div class="card" id="usersCard"><h3>成员</h3><div class="muted">加载中…</div></div>`;
  if (me.role === "admin") html += `<div class="card"><h3>团队名称</h3><form data-submit="saveOrg" class="row"><input name="org_name" value="${esc(me.org_name || "")}" placeholder="显示在左上角，例如：王老师课题组" style="flex:1;min-width:200px"><button type="submit">保存</button></form></div>
    <div class="card" id="llmCard"><h3>团队默认 AI 模型</h3><div class="muted">加载中…</div></div>
    <div class="card" id="lanCard"><h3>手机与同学访问</h3><div class="muted">加载中…</div></div>`;
  html += `<div class="card"><h3>关于</h3><p>CanDo 可为 ${esc(me.version)}</p>
    <p class="muted">所有数据（账号、项目、资料原文件）只保存在运行工作台的电脑上。使用 AI 模型时，只有检索到的少量相关片段会发送给你设置的模型服务商；开启“联网核对”后，参考文献的题名会发送给公开文献数据库 OpenAlex 查询。</p>
    ${me.role === "admin" && me.local ? `<p class="muted">关闭窗口后，工作台会继续在后台运行，方便手机访问。如需完全退出：</p><button class="danger" data-act="quitApp">退出工作台程序</button>` : ""}</div>`;
  $("#main").innerHTML = html;
  renderMyModels($("#myModels")).catch(showErr);
  renderZoteroSettings($("#myZotero")).catch(showErr);
  renderScholarSettings($("#scholarCard")).catch(showErr);
  renderWebSearch().catch(showErr);
  if (isTeacher()) renderUsers();
  if (me.role === "admin") { renderLLM(); renderLAN(); }
}
actions.saveMe = async (f) => {
  const d = formData(f);
  const changedUser = d.username.trim() !== S.me.username;
  S.me = await api("PATCH", "/api/me", d);
  $("#who").textContent = `${S.me.name} · ${ROLE[S.me.role]}`;
  toast(changedUser ? `已保存。以后请用新用户名“${S.me.username}”登录` : "已保存");
  if (isTeacher()) renderUsers();
};
actions.saveOrg = async (f) => {
  await api("PUT", "/api/settings", { org_name: new FormData(f).get("org_name") });
  S.me = await api("GET", "/api/me");
  setOrg(S.me.org_name);
  toast("已保存");
};
actions.appChangeServer = () => window.KyApp && window.KyApp.changeServer();
actions.renameUser = async (el) => {
  const n = prompt(`修改“${el.dataset.name}”的显示姓名：`, el.dataset.name);
  if (!n || !n.trim()) return;
  await api("PATCH", `/api/users/${el.dataset.id}`, { name: n.trim() });
  toast("已修改");
  renderUsers();
};
actions.changePw = async (f) => { await api("POST", "/api/me/password", formData(f)); f.reset(); S.me.must_change_pw = false; toast("密码已修改，其他设备需重新登录"); };
actions.logout = async () => { await api("POST", "/api/logout", {}); S.me = null; location.hash = "#/login"; boot(); };
actions.quitApp = async () => {
  if (!confirm("退出后手机和其他同学将无法访问，直到你再次打开工作台。确定退出？")) return;
  await api("POST", "/api/quit", {});
  document.body.innerHTML = `<div class="card center-card" style="margin-top:20vh"><h3>工作台已退出</h3><p>可以关闭这个窗口了。再次使用时，双击桌面上的“CanDo 可为”图标。</p></div>`;
};

async function renderUsers() {
  const users = await loadUsers();
  const admin = S.me.role === "admin";
  $("#usersCard").innerHTML = `<h3>成员</h3>
    <div class="tw"><table><tr><th>姓名</th><th>用户名</th><th>角色</th><th>状态</th><th></th></tr>
    ${users.map((u) => `<tr><td>${esc(u.name)}</td><td>${esc(u.username)}</td><td>${admin && u.id !== S.me.id ? `<select data-change="setRole" data-id="${u.id}">${Object.entries(ROLE).map(([k, v]) => `<option value="${k}" ${u.role === k ? "selected" : ""}>${v}</option>`).join("")}</select>` : ROLE[u.role]}</td>
      <td>${u.disabled ? '<span class="tag bad">已停用</span>' : '<span class="tag ok">正常</span>'}</td>
      <td style="white-space:nowrap">${u.id !== S.me.id && (admin || u.role === "student") ? `<button class="sm" data-act="renameUser" data-id="${u.id}" data-name="${esc(u.name)}">改名</button> <button class="sm" data-act="resetPw" data-id="${u.id}" data-name="${esc(u.name)}">重置密码</button> <button class="sm ${u.disabled ? "" : "danger"}" data-act="toggleUser" data-id="${u.id}" data-dis="${u.disabled ? 0 : 1}">${u.disabled ? "启用" : "停用"}</button>` : ""}</td></tr>`).join("")}</table></div>
    <h4 style="margin-top:14px">添加成员</h4>
    <form data-submit="addUser" class="grid2">
      <div><label class="f">姓名</label><input name="name" required placeholder="例如：张同学"></div>
      <div><label class="f">登录用户名</label><input name="username" required placeholder="例如：学号"></div>
      <div><label class="f">初始密码（至少 6 位，首次登录后会提示修改）</label><input name="password" required minlength="6"></div>
      <div><label class="f">角色</label><select name="role"><option value="student">学生</option>${admin ? '<option value="teacher">老师</option><option value="admin">管理员</option>' : ""}</select></div>
      <div><button class="pri" type="submit">添加</button></div></form>`;
}
actions.addUser = async (f) => { const d = formData(f); await api("POST", "/api/users", d); toast(`已添加 ${d.name}，请把用户名和初始密码告诉对方`); renderUsers(); };
actions.resetPw = async (el) => {
  const pw = prompt(`为“${el.dataset.name}”设置新的初始密码（至少 6 位）：`);
  if (!pw) return;
  await api("PATCH", `/api/users/${el.dataset.id}`, { password: pw });
  toast("已重置，对方下次登录后需修改密码");
};
actions.toggleUser = async (el) => { await api("PATCH", `/api/users/${el.dataset.id}`, { disabled: el.dataset.dis === "1" }); renderUsers(); };
actions.setRole = async (el) => { await api("PATCH", `/api/users/${el.dataset.id}`, { role: el.value }); toast("角色已更新"); };

async function renderLLM() {
  const s = await api("GET", "/api/settings");
  $("#llmCard").innerHTML = `<h3>团队默认 AI 模型 ${s.llm_configured ? '<span class="tag ok">已配置</span>' : '<span class="tag warn">未配置</span>'} ${s.llm_configured ? checkTag(s.team_check) : ""}</h3>
    <div class="row" style="margin-bottom:6px"><button class="pri" type="button" data-act="teamWizard">一步步接入（推荐）</button><a class="sm" href="#/usage/team">全组用量与花费 →</a></div>
    <p class="muted">没有添加自己模型的成员，会使用这里的团队默认模型（费用记在这个 Key 上）。成员也可以在上面“我的 AI 模型”里添加自己的模型。支持 DeepSeek、通义千问、智谱、Kimi、豆包等“OpenAI 兼容接口”，以及 Claude 原生接口。示例模型名可能随服务商调整，请以其文档为准。</p>
    <form data-submit="saveLLM">
      <label class="f">服务商</label><select data-change="preset">${PRESETS.map(([k, n]) => `<option value="${k}">${esc(n)}</option>`).join("")}</select>
      <div class="grid2">
        <div><label class="f">显示名称（可选）</label><input name="llm_name" value="${esc(s.llm_name || "")}" placeholder="例如：课题组 DeepSeek"></div>
        <div><label class="f">协议类型</label><select name="llm_protocol"><option value="openai" ${s.llm_protocol !== "anthropic" ? "selected" : ""}>OpenAI 兼容（大多数服务商）</option><option value="anthropic" ${s.llm_protocol === "anthropic" ? "selected" : ""}>Anthropic 原生</option></select></div>
      </div>
      <label class="f">接口地址（Base URL）</label><input name="llm_base_url" value="${esc(s.llm_base_url)}" placeholder="https://…/v1">
      <label class="f">日常模型名称</label><input name="llm_model" value="${esc(s.llm_model)}">
      <label class="f">难题模型名称（可选，同一个接口和密钥下更强的模型，难的任务自动交给它）</label><input name="llm_strong_model" value="${esc(s.llm_strong_model || "")}" placeholder="例如 deepseek-v4-pro、qwen-max">
      <details><summary class="muted">价格（元 / 百万 tokens，用于估算花费和额度）</summary><div class="grid2">
        <div><label class="f">日常 输入价</label><input name="llm_price_in" type="number" step="any" min="0" value="${s.llm_price_in || ""}"></div>
        <div><label class="f">日常 输出价</label><input name="llm_price_out" type="number" step="any" min="0" value="${s.llm_price_out || ""}"></div>
        <div><label class="f">难题 输入价</label><input name="llm_strong_price_in" type="number" step="any" min="0" value="${s.llm_strong_price_in || ""}"></div>
        <div><label class="f">难题 输出价</label><input name="llm_strong_price_out" type="number" step="any" min="0" value="${s.llm_strong_price_out || ""}"></div></div></details>
      <label class="f">每人每月额度（元，0 = 不限；只限制使用团队模型，自己接入的模型不受影响）</label><input name="team_budget" type="number" step="any" min="0" value="${s.team_budget || 0}" style="max-width:200px">
      <label class="f">API Key ${s.llm_key_set ? `<span class="muted">（已保存：${esc(s.llm_key_masked)}，不修改请留空）</span>` : ""}</label><input name="llm_key" type="password" autocomplete="off" placeholder="${s.llm_key_set ? "留空表示不修改" : "sk-…"}">
      <label style="display:block;margin:10px 0"><input type="checkbox" name="llm_vision" ${s.llm_vision ? "checked" : ""}> 这个模型能看图片（成员没有自己的识图模型时，“图片转 LaTeX”用它）</label>
      <label style="display:block;margin:10px 0"><input type="checkbox" name="online_check" ${s.online_check ? "checked" : ""}>引用核验时联网查询公开文献数据库（OpenAlex），核对参考文献是否存在（只发送题名）</label>
      <div class="row"><button class="pri" type="submit">保存</button><button type="button" data-act="testLLM">测试连接</button>${s.llm_configured ? '<button type="button" data-act="teamCheck">兼容性自检</button>' : ""}${s.llm_key_set ? '<button type="button" class="danger" data-act="clearKey">清除密钥</button>' : ""}<span id="llmTest" class="muted"></span></div>
    </form>
    ${s.team_check && !s.team_check.passed ? checkReport(s.team_check) : ""}`;
}
actions.preset = (el) => {
  const p = PRESETS.find((x) => x[0] === el.value);
  if (!p || !p[0]) return;
  const f = el.form;
  if (p[2]) f.llm_base_url.value = p[2];
  f.llm_model.value = p[3] || f.llm_model.value;
  f.llm_protocol.value = p[4];
  f.llm_vision.checked = !!p[5];
  if (el.value === "ollama" && !f.llm_key.value) f.llm_key.value = "ollama";
};
actions.saveLLM = async (f) => {
  const d = formData(f);
  await api("PUT", "/api/settings", { llm_base_url: d.llm_base_url, llm_model: d.llm_model, llm_key: d.llm_key, llm_protocol: d.llm_protocol, llm_name: d.llm_name, llm_vision: d.llm_vision === "on", online_check: d.online_check === "on",
    llm_strong_model: d.llm_strong_model || "", llm_price_in: Number(d.llm_price_in || 0), llm_price_out: Number(d.llm_price_out || 0),
    llm_strong_price_in: Number(d.llm_strong_price_in || 0), llm_strong_price_out: Number(d.llm_strong_price_out || 0), team_budget: Number(d.team_budget || 0) });
  S.me = await api("GET", "/api/me");
  toast("已保存");
  renderLLM();
};
actions.teamCheck = async (el) => {
  el.disabled = true;
  el.textContent = "自检中（约 30–60 秒）…";
  try {
    const r = await api("POST", "/api/models/team/check", {});
    modal(`<h3>团队模型兼容性自检 ${checkTag(r)}</h3>${checkReport(r)}`);
  } finally { renderLLM(); }
};
actions.clearKey = async () => { await api("PUT", "/api/settings", { clear_key: true }); S.me = await api("GET", "/api/me"); renderLLM(); };
actions.testLLM = async (el) => {
  el.disabled = true;
  $("#llmTest").textContent = "测试中…（请先保存）";
  try { const r = await api("POST", "/api/settings/test-llm", {}); $("#llmTest").textContent = r.message; $("#llmTest").style.color = r.ok ? "var(--ok)" : "var(--bad)"; }
  finally { el.disabled = false; }
};

async function renderLAN() {
  const info = await api("GET", "/api/lan-info");
  const local = S.me.local;
  const url = info.urls[0];
  const qrOf = (text, label) => {
    if (!window.qrcode) return "";
    const q = window.qrcode(0, "M"); q.addData(text); q.make();
    return `<div style="text-align:center"><div class="qr">${q.createSvgTag({ cellSize: 4, margin: 2, scalable: true })}</div><div class="muted" style="margin-top:4px">${label}</div></div>`;
  };
  let qr = "";
  if (info.enabled && url) qr = qrOf(url + "install.html", "① 扫码安装（安卓 App / 苹果主屏幕）") + qrOf(url, "② 扫码直接打开 / App 连接电脑");
  $("#lanCard").innerHTML = `<h3>手机与同学访问 ${info.enabled ? '<span class="tag ok">已开启</span>' : '<span class="tag">未开启</span>'}</h3>
    <p class="muted">开启后，和这台电脑连在同一个 Wi-Fi / 局域网下的手机和电脑，可以用浏览器打开下面的地址登录使用。关闭时只有本机能使用。</p>
    ${local ? `<label style="display:block;margin:8px 0"><input type="checkbox" data-change="toggleLAN" ${info.enabled ? "checked" : ""}>允许手机和同学访问</label>` : '<p class="muted">（此开关只能在运行工作台的电脑上修改）</p>'}
    ${info.enabled ? (info.urls.length ? `<div class="row" style="align-items:flex-start;gap:18px">${qr}<div>
        <p><b>访问地址：</b></p>${info.urls.map((u) => `<p><code>${esc(u)}</code></p>`).join("")}
        <p class="muted"><b>安卓手机：</b>扫二维码①，下载并安装“CanDo”App（第一次安装需允许“安装未知应用”）。打开 App 后，扫二维码②并选择用“CanDo”打开，或手动输入上面的访问地址。</p>
        <p class="muted"><b>苹果手机：</b>用相机扫二维码①，在 Safari 中按页面提示打开工作台并登录，点底部“分享” → “添加到主屏幕”，桌面上就有“CanDo”图标，全屏使用和 App 一样。（有 Mac 和苹果开发者账号时，也可以用源码包里的 ios 工程编译原生 App。）</p>
        <p class="muted">打不开？① 确认手机和电脑连的是同一个 Wi-Fi；② 部分校园网禁止设备互访，可改用手机热点让电脑连接；③ 电脑防火墙需允许“CanDo 可为”（安装时已自动设置）。</p></div></div>`
      : `<div class="msg">没有检测到这台电脑的局域网地址，请确认电脑已连接 Wi-Fi 或网线。</div>`) : ""}`;
}
actions.toggleLAN = async (el) => {
  await api("PUT", "/api/settings", { lan_enabled: el.checked });
  toast(el.checked ? "已开启手机访问" : "已关闭手机访问");
  renderLAN();
};

actions.cite = (el) => showCite(el.dataset.cid);

boot();
