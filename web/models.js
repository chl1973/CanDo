// 我的 AI 模型：模型配置卡（个人）、兼容性自检、导入导出
import { $, esc, api, toast, modal, closeModal, actions, fmtTime } from "/core.js";

export const PRESETS = [
  ["", "选择服务商（自动填写地址和示例模型名）", "", "", "openai"],
  ["deepseek", "DeepSeek", "https://api.deepseek.com/v1", "deepseek-chat", "openai"],
  ["qwen", "通义千问（阿里云百炼）", "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen-plus", "openai"],
  ["glm", "智谱 GLM", "https://open.bigmodel.cn/api/paas/v4", "glm-4-flash", "openai"],
  ["kimi", "Kimi（月之暗面）", "https://api.moonshot.cn/v1", "moonshot-v1-32k", "openai"],
  ["doubao", "豆包（火山方舟）", "https://ark.cn-beijing.volces.com/api/v3", "", "openai"],
  ["qwen-vl", "通义千问 VL（能看图片）", "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen-vl-max", "openai", true],
  ["glm-v", "智谱 GLM-4V（能看图片）", "https://open.bigmodel.cn/api/paas/v4", "glm-4v-flash", "openai", true],
  ["doubao-v", "豆包视觉（能看图片，模型名填接入点 ID）", "https://ark.cn-beijing.volces.com/api/v3", "", "openai", true],
  ["claude", "Claude（Anthropic 原生接口，能看图片）", "https://api.anthropic.com", "", "anthropic", true],
  ["ollama", "本机 Ollama（离线，需自行安装）", "http://127.0.0.1:11434/v1", "qwen2.5:7b", "openai"],
  ["custom", "其他 OpenAI 兼容接口 / one-api 网关", "", "", "openai"],
];

const SRC = { personal: "我的模型", project: "项目指定", team: "团队默认" };

export function checkTag(c) {
  if (!c) return '<span class="tag warn">未自检</span>';
  if (c.passed && c.warn) return '<span class="tag warn" title="必过项都通过，有提醒项没通过，可以启用">基本通过</span>';
  return c.passed ? '<span class="tag ok">自检通过</span>' : '<span class="tag bad">自检未通过</span>';
}

export function checkReport(c) {
  if (!c) return "";
  return `<div style="margin-top:6px">${c.items.map((it) => `<div>${it.pass ? "✅" : "❌"} <b>${esc(it.name)}</b> <span class="muted">${esc(it.detail)}</span></div>`).join("")}
    <div class="muted">自检时间 ${fmtTime(c.at)}。自检只说明模型遵守规则的能力，不代表回答质量。</div></div>`;
}

let EL = null;

export async function renderMyModels(el) {
  EL = el;
  const d = await api("GET", "/api/models");
  const eff = d.effective;
  const me = await api("GET", "/api/me");
  const quota = eff.source === "team" && me.team_budget > 0 ? `本月已用团队额度 ¥${me.team_spent < 0.01 && me.team_spent > 0 ? me.team_spent.toFixed(4) : me.team_spent.toFixed(2)} / ¥${me.team_budget < 0.01 ? me.team_budget.toFixed(4) : me.team_budget.toFixed(2)}。` : "";
  el.innerHTML = `<div class="row"><h3 style="margin:0">我的 AI 模型</h3><span class="sp"></span><a class="sm" href="#/usage">用量与花费 →</a></div>
    <div class="msg ${eff.configured ? "info" : ""}" style="margin-top:8px">你现在使用：<b>${esc(eff.label)}</b>${eff.configured ? (eff.source === "personal" ? "（我的模型）" : "") : "。"}
      ${eff.source === "team" && eff.configured ? `<div>这是老师配置的团队模型，<b>你不需要自己接入</b>就能使用。${quota}</div>` : ""}
      ${!eff.configured ? `<div style="margin-top:6px"><button class="pri" data-act="mdlWizard">一步步接入（约 3 分钟）</button></div>` : ""}</div>
    <p class="muted">使用顺序：项目指定的模型 → 你启用的模型 → 团队默认模型。你的 API Key 加密保存在运行工作台的电脑上，只用于你自己的请求，管理员也看不到。建议使用单独的、设置了消费额度上限的 Key。新配置需要先通过“兼容性自检”（约 30–60 秒）才能启用。</p>
    ${d.mine.length ? d.mine.map((m) => `<div class="card" style="margin:10px 0;padding:12px">
      <div class="row"><b>${esc(m.name)}</b>${checkTag(m.check)}${m.vision ? '<span class="tag">能看图片</span>' : ""}${m.active ? '<span class="tag pri">使用中</span>' : ""}<span class="sp"></span>
        <span class="muted">${esc(m.protocol === "anthropic" ? "Anthropic" : "OpenAI 兼容")} · ${esc(m.model)} · Key ${esc(m.key_masked)}</span></div>
      <div class="muted">${esc(m.base_url)}${m.strong_model ? ` · 难题模型 ${esc(m.strong_model)} ${m.check && m.check.strong === m.strong_model ? (m.check.strong_ok ? '<span class="tag ok">难题模型可用</span>' : '<span class="tag bad">难题模型自检未通过</span>') : ""}` : ""}${m.price_in || m.price_out ? ` · ¥${m.price_in}/${m.price_out} 每百万 tokens` : ""}</div>
      ${m.check && !m.check.passed ? checkReport(m.check) + '<p class="muted">没通过“依据原文回答”或“基本连通”，不能启用。可以点“编辑”把模型换成更大的一个再自检。</p>' : ""}
      ${m.check && m.check.passed && m.check.warn ? `<details><summary class="muted">基本通过：可以启用，有提醒项没通过（点开查看）</summary>${checkReport(m.check)}<p class="muted">系统在实际使用时会校验每条引用，无效引用会被丢弃，所以这些提醒不影响使用；但涉及引用核验等严格任务时，建议再配一个能全部通过的“难题模型”。</p></details>` : ""}
      <div class="row" style="margin-top:8px">
        <button class="sm" data-act="mdlCheck" data-id="${esc(m.id)}">兼容性自检</button>
        ${m.active ? `<button class="sm" data-act="mdlActive" data-id="${esc(m.id)}" data-on="0">停用</button>` : `<button class="sm pri" data-act="mdlActive" data-id="${esc(m.id)}" data-on="1" ${m.check && m.check.passed ? "" : "disabled title='自检通过后才能启用'"}>启用</button>`}
        <button class="sm" data-act="mdlEdit" data-id="${esc(m.id)}">编辑</button>
        <a class="sm" href="/api/models/${esc(m.id)}/export" download="model-card.json">导出配置卡</a>
        <button class="sm danger" data-act="mdlDel" data-id="${esc(m.id)}" data-name="${esc(m.name)}">删除</button></div></div>`).join("") : '<p class="muted">还没有自己的模型配置。</p>'}
    <div class="row"><button class="pri" data-act="mdlWizard">一步步接入（推荐）</button><button data-act="mdlNew">手动添加</button>
      <label class="muted" style="cursor:pointer">或导入配置卡：<input type="file" accept=".json" data-change="mdlImport" style="width:auto"></label></div>`;
  window.__models = d.mine;
}

function form(m = {}, act = "mdlCreate") {
  return `<form data-submit="${act}">${m.id ? `<input type="hidden" name="id" value="${esc(m.id)}">` : ""}
    <label class="f">服务商</label><select data-change="mdlPreset">${PRESETS.map(([k, n]) => `<option value="${k}">${esc(n)}</option>`).join("")}</select>
    <div class="grid2">
      <div><label class="f">名称（自己起，方便区分）</label><input name="name" value="${esc(m.name || "")}" placeholder="例如：我的 DeepSeek"></div>
      <div><label class="f">协议类型</label><select name="protocol"><option value="openai" ${m.protocol !== "anthropic" ? "selected" : ""}>OpenAI 兼容（大多数服务商）</option><option value="anthropic" ${m.protocol === "anthropic" ? "selected" : ""}>Anthropic 原生</option></select></div>
    </div>
    <label class="f">接口地址（Base URL）</label><input name="base_url" required value="${esc(m.base_url || "")}" placeholder="https://…/v1">
    <label class="f">模型名称（以服务商文档为准）</label><input name="model" required value="${esc(m.model || "")}">
    <label class="f">API Key ${m.id ? `<span class="muted">（已保存：${esc(m.key_masked)}，不修改请留空）</span>` : ""}</label><input name="key" type="password" autocomplete="off" ${m.id ? "" : "required"} placeholder="${m.id ? "留空表示不修改" : "sk-…"}">
    <div class="grid2">
      <div><label class="f">温度（建议 0.1，越低越稳定）</label><input name="temperature" type="number" step="0.1" min="0" max="1.5" value="${m.temperature ?? 0.1}"></div>
      <div><label class="f">超时（秒）</label><input name="timeout" type="number" min="10" max="600" value="${m.timeout || 120}"></div>
    </div>
    <label class="f">难题模型（可选，同一个接口和密钥下更强的模型；难的任务会自动交给它）</label><input name="strong_model" value="${esc(m.strong_model || "")}" placeholder="例如 deepseek-v4-pro、qwen-max；不填则所有任务都用上面的模型">
    <details><summary class="muted">价格（元 / 百万 tokens，用于估算花费，可不填）</summary><div class="grid2">
      <div><label class="f">输入价</label><input name="price_in" type="number" step="any" min="0" value="${m.price_in ?? ""}"></div>
      <div><label class="f">输出价</label><input name="price_out" type="number" step="any" min="0" value="${m.price_out ?? ""}"></div>
      <div><label class="f">难题模型 输入价</label><input name="strong_price_in" type="number" step="any" min="0" value="${m.strong_price_in ?? ""}"></div>
      <div><label class="f">难题模型 输出价</label><input name="strong_price_out" type="number" step="any" min="0" value="${m.strong_price_out ?? ""}"></div></div></details>
    <label style="display:block;margin:10px 0"><input type="checkbox" name="vision" ${m.vision ? "checked" : ""}> 这个模型能看图片（用于“图片转 LaTeX”，如通义千问 VL、GLM-4V、豆包视觉、Claude；DeepSeek 等纯文字模型不要勾）</label>
    <p class="muted">修改接口地址、模型或 Key 后需要重新自检。</p>
    <div class="row" style="margin-top:10px"><span class="sp"></span><button type="button" data-act="closeModal">取消</button><button class="pri" type="submit">保存</button></div></form>`;
}

const body = (f) => {
  const d = Object.fromEntries(new FormData(f).entries());
  const out = { name: d.name, protocol: d.protocol, base_url: d.base_url, model: d.model, temperature: Number(d.temperature), timeout: Number(d.timeout), vision: d.vision === "on",
    strong_model: d.strong_model || "", price_in: Number(d.price_in || 0), price_out: Number(d.price_out || 0), strong_price_in: Number(d.strong_price_in || 0), strong_price_out: Number(d.strong_price_out || 0) };
  if (d.key) out.key = d.key;
  return out;
};

actions.mdlPreset = (el) => {
  const p = PRESETS.find((x) => x[0] === el.value);
  if (!p || !p[0]) return;
  const f = el.form;
  if (p[2]) f.base_url.value = p[2];
  f.model.value = p[3] || f.model.value;
  f.protocol.value = p[4];
  f.vision.checked = !!p[5];
  if (!f.name.value) f.name.value = p[1].replace(/（.*/, "");
  if (el.value === "ollama" && !f.key.value) f.key.value = "ollama";
};
actions.mdlNew = () => modal(`<h3>添加我的模型</h3>${form()}`);
actions.mdlCreate = async (f) => {
  await api("POST", "/api/models", body(f));
  closeModal();
  toast("已保存，请点“兼容性自检”");
  renderMyModels(EL);
};
actions.mdlEdit = (el) => modal(`<h3>编辑模型配置</h3>${form((window.__models || []).find((m) => m.id === el.dataset.id), "mdlSave")}`);
actions.mdlSave = async (f) => {
  await api("PATCH", `/api/models/${f.id.value}`, body(f));
  closeModal();
  toast("已保存");
  renderMyModels(EL);
};
actions.mdlDel = async (el) => {
  if (!confirm(`删除模型配置“${el.dataset.name}”？`)) return;
  await api("DELETE", `/api/models/${el.dataset.id}`);
  renderMyModels(EL);
};
actions.mdlActive = async (el) => {
  await api("POST", `/api/models/${el.dataset.id}/activate`, { active: el.dataset.on === "1" });
  toast(el.dataset.on === "1" ? "已启用，之后的问答和核验将使用这个模型" : "已停用");
  renderMyModels(EL);
};
actions.mdlCheck = async (el) => {
  el.disabled = true;
  el.textContent = "自检中（约 30–60 秒）…";
  try {
    const r = await api("POST", `/api/models/${el.dataset.id}/check`, {});
    modal(`<h3>兼容性自检 ${checkTag(r)}</h3>${checkReport(r)}
      ${r.passed ? (r.warn ? '<p><b>基本通过，可以点“启用”使用。</b>有提醒项没通过：系统在使用时会校验每条引用、丢弃无效引用，所以不影响正常使用；做引用核验等严格任务时，建议再配一个能全部通过的“难题模型”。</p>' : '<p>可以点“启用”使用这个模型了。</p>') : '<p class="muted">没通过的原因通常是：模型较小、不擅长按格式输出，或接口/Key 填错。可以换一个更大的模型再试。</p>'}`);
  } finally { renderMyModels(EL); }
};
actions.mdlImport = async (el) => {
  const f = el.files[0];
  if (!f) return;
  let c;
  try { c = JSON.parse(await f.text()); } catch { return toast("不是有效的配置卡文件", true); }
  if (!c.kyws_model_card) return toast("不是 CanDo 可为的模型配置卡", true);
  modal(`<h3>导入配置卡</h3><div class="msg info">配置卡不含 API Key，请填写你自己的 Key。</div>${form({ name: c.name, protocol: c.protocol, base_url: c.base_url, model: c.model, temperature: c.temperature, timeout: c.timeout, vision: c.vision,
    strong_model: c.strong_model, price_in: c.price_in, price_out: c.price_out, strong_price_in: c.strong_price_in, strong_price_out: c.strong_price_out })}`);
  el.value = "";
};
