// 用量与花费：本人 / 全组（管理员）
import { $, $$, esc, api, actions, fmtTime } from "/core.js";

const U = { scope: "me", month: "" };
export const money = (x) => (x > 0 && x < 0.01 ? "¥" + x.toFixed(4) : "¥" + (x || 0).toFixed(2));
const num = (x) => (x >= 10000 ? (x / 10000).toFixed(1) + " 万" : String(x || 0));
const rate = (g) => (g.calls ? Math.round(((g.calls - g.fails - g.invalid) / g.calls) * 100) + "%" : "—");
const avgSec = (g) => (g.calls ? (g.ms / g.calls / 1000).toFixed(1) + " 秒" : "—");
const per = (g) => (g.calls ? money(g.cost / g.calls) : "—");

export async function pageUsage(main, scope) {
  U.scope = scope === "team" ? "team" : "me";
  U.main = main;
  await render();
}

async function render() {
  const main = U.main;
  const me = await api("GET", "/api/me");
  if (U.scope === "team" && me.role !== "admin") U.scope = "me";
  const q = new URLSearchParams({ scope: U.scope });
  if (U.month) q.set("month", U.month);
  const d = await api("GET", `/api/usage?${q}`);
  U.month = d.month;
  const t = d.total;
  const maxDay = Math.max(0.000001, ...d.daily.map((x) => (t.cost > 0 ? x.cost : x.calls)));
  const table = (rows, first) => rows.length ? `<div class="tw"><table><tr><th>${first}</th><th>次数</th><th>成功率</th><th>平均耗时</th><th>tokens（入/出）</th><th>每次约</th><th>合计</th></tr>
    ${rows.map((g) => `<tr><td>${esc(g.label)}</td><td>${g.calls}</td><td>${rate(g)}</td><td>${avgSec(g)}</td><td>${num(g.in)} / ${num(g.out)}${g.est ? "*" : ""}</td><td>${per(g)}</td><td><b>${money(g.cost)}</b></td></tr>`).join("")}</table></div>` : '<p class="muted">暂无</p>';
  const budget = d.budget > 0 && me.model_source === "team" && U.scope === "me"
    ? `<div class="tile"><div class="l">团队模型本月额度</div><div class="v">${money(d.my_team_cost)} / ${money(d.budget)}</div><div class="meter ${d.my_team_cost >= d.budget ? "over" : ""}"><div style="width:${Math.min(100, (d.my_team_cost / d.budget) * 100)}%"></div></div></div>` : "";
  main.innerHTML = `<h2>用量与花费</h2>
    <div class="row">
      <div class="tabs" style="margin:0;border:0"><button class="${U.scope === "me" ? "on" : ""}" data-act="usScope" data-s="me">我的</button>${me.role === "admin" ? `<button class="${U.scope === "team" ? "on" : ""}" data-act="usScope" data-s="team">全组</button>` : ""}</div>
      <span class="sp"></span><input type="month" value="${esc(d.month)}" data-change="usMonth" style="width:auto"></div>
    <div class="card"><div class="tiles">
      <div class="tile"><div class="l">${U.scope === "team" ? "全组" : "我"}本月花费（估算）</div><div class="v">${money(t.cost)}</div></div>
      <div class="tile"><div class="l">调用次数</div><div class="v">${t.calls}</div></div>
      <div class="tile"><div class="l">成功率</div><div class="v">${rate(t)}</div></div>
      <div class="tile"><div class="l">tokens（输入 / 输出）</div><div class="v" style="font-size:17px">${num(t.in)} / ${num(t.out)}</div></div>
      ${d.projection ? `<div class="tile"><div class="l">按目前速度，本月约</div><div class="v">${money(d.projection)}</div></div>` : ""}
      ${budget}
    </div>
    ${d.daily.length ? `<h4>每天${t.cost > 0 ? "花费" : "调用次数"}</h4><div class="bars">${d.daily.map((x) => `<div class="b" style="height:${Math.max(3, ((t.cost > 0 ? x.cost : x.calls) / maxDay) * 100)}%" title="${esc(x.key)}：${x.calls} 次，${money(x.cost)}"></div>`).join("")}</div>` : '<p class="muted">这个月还没有调用记录。</p>'}
    <p class="muted" style="margin-bottom:0">花费 = tokens × 模型设置里填的价格，是估算值，以服务商账单为准。成功率只算“按要求输出、引用合格”的回答。${t.est ? "带 * 的 tokens 是估算的（服务商没有返回用量）。" : ""}</p></div>
    ${d.insights.length ? `<div class="card"><h3>优化建议</h3>${d.insights.map((x) => `<div class="rditem">· ${esc(x)}</div>`).join("")}</div>` : ""}
    <div class="card"><h3>按任务</h3>${table(d.by_task, "任务")}</div>
    <div class="card"><h3>按模型</h3>${table(d.by_model, "模型")}
      <p class="muted">系统会按难度分配：拆检索词、速读、概念讲解用日常模型；引用核验、资料对比、改文件和需要推理的问题用难题模型；日常模型回答不合格时自动用难题模型重试。在“设置 → 我的 AI 模型”里给模型填上“难题模型”即可开启。</p></div>
    ${d.by_user ? `<div class="card"><h3>按成员</h3>${table(d.by_user, "成员")}</div>` : ""}
    <div class="card"><h3>最近的调用</h3>${d.recent.length ? `<div class="tw"><table><tr><th>时间</th>${U.scope === "team" ? "<th>成员</th>" : ""}<th>任务</th><th>模型</th><th>tokens</th><th>耗时</th><th>花费</th><th>结果</th></tr>
      ${d.recent.map((c) => `<tr><td style="white-space:nowrap">${fmtTime(c.at)}</td>${U.scope === "team" ? `<td>${esc(c.user)}</td>` : ""}<td>${esc(c.task)}</td>
        <td title="${esc(c.route || "")}">${esc(c.model)} ${c.tier === "strong" ? '<span class="tag pri">难题</span>' : ""}</td><td>${c.in}/${c.out}</td><td>${(c.ms / 1000).toFixed(1)}s</td><td>${money(c.cost)}</td>
        <td>${c.ok ? '<span class="tag ok">成功</span>' : `<span class="tag bad" title="${esc(c.err || "")}">${esc(c.err || "失败")}</span>`}</td></tr>`).join("")}</table></div>
      <p class="muted">鼠标停在模型名上可以看到“为什么用这个模型”。</p>` : '<p class="muted">暂无</p>'}</div>`;
}

actions.usScope = (el) => { U.scope = el.dataset.s; history.replaceState(null, "", "#/usage/" + U.scope); render(); };
actions.usMonth = (el) => { U.month = el.value; render(); };
