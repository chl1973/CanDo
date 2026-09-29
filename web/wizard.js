// 接入向导：选服务商 → 按提示拿到密钥并粘贴 → 自动识别可用模型、推荐“日常模型 + 难题模型 + 识图模型”、填好参考价 → 保存并自检 → 自动启用
import { $, esc, api, toast, modal, closeModal, actions } from "/core.js";
import { checkTag, checkReport, renderMyModels } from "/models.js";

// 参考价：元 / 百万 tokens（输入, 输出），2026 年 9 月查询，服务商会调整，以官网为准。
export const WIZ = [
  { k: "glm", name: "智谱 GLM", tag: "有免费模型，最省钱", url: "https://open.bigmodel.cn/api/paas/v4", protocol: "openai",
    signup: "https://bigmodel.cn/", keys: "https://bigmodel.cn/usercenter/proj-mgmt/apikeys", hint: "密钥中间有一个“.”", keyCheck: (k) => k.includes("."),
    daily: [/^glm-4-flash-\d+$/i, /^glm-4-flash$/i, /flash/i], strong: [/^glm-4\.\d+$/i, /^glm-4-plus$/i, /^glm-4\.\d/i], vision: [/^glm-4v-flash$/i, /^glm-4v/i],
    fallback: { daily: "glm-4-flash-250414", strong: "", vision: "glm-4v-flash" }, price: { "glm-4-flash-250414": [0, 0], "glm-4-flash": [0, 0], "glm-4v-flash": [0, 0] } },
  { k: "deepseek", name: "DeepSeek", tag: "便宜，中文好（不能看图）", url: "https://api.deepseek.com", protocol: "openai",
    signup: "https://platform.deepseek.com/", keys: "https://platform.deepseek.com/api_keys", hint: "以 sk- 开头", keyCheck: (k) => k.startsWith("sk-"),
    daily: [/flash/i, /chat/i], strong: [/pro/i, /reasoner/i], vision: [],
    fallback: { daily: "deepseek-flash", strong: "deepseek-v4-pro" }, price: { "deepseek-flash": [2, 8], "deepseek-chat": [2, 8], "deepseek-v4-pro": [9, 27] } },
  { k: "qwen", name: "通义千问（阿里云百炼）", tag: "新用户送 100 万 tokens，能看图", url: "https://dashscope.aliyuncs.com/compatible-mode/v1", protocol: "openai",
    signup: "https://bailian.console.aliyun.com/", keys: "https://bailian.console.aliyun.com/?tab=model#/api-key", hint: "以 sk- 开头", keyCheck: (k) => k.startsWith("sk-"),
    daily: [/^qwen-plus$/i, /^qwen-flash$/i, /^qwen-turbo$/i], strong: [/^qwen-max$/i, /^qwen3-max$/i], vision: [/^qwen-vl-max$/i, /^qwen-vl-plus$/i],
    fallback: { daily: "qwen-plus", strong: "qwen-max", vision: "qwen-vl-max" }, price: { "qwen-flash": [0.15, 1.5], "qwen-plus": [0.8, 2], "qwen-max": [2.5, 10] } },
  { k: "kimi", name: "Kimi（月之暗面）", tag: "擅长长文档", url: "https://api.moonshot.cn/v1", protocol: "openai",
    signup: "https://platform.moonshot.cn/", keys: "https://platform.moonshot.cn/console/api-keys", hint: "以 sk- 开头", keyCheck: (k) => k.startsWith("sk-"),
    daily: [/^kimi-k2\.\d+$/i, /^kimi-k2(?!.*code)/i, /^moonshot-v1-32k$/i], strong: [/^kimi-k3$/i, /^kimi-k3/i], vision: [/vision/i],
    fallback: { daily: "kimi-k2.6", strong: "kimi-k3" }, price: { "kimi-k2.6": [6.5, 27], "kimi-k3": [20, 100] } },
  { k: "ollama", name: "本机 Ollama", tag: "完全离线、不花钱，需要电脑配置较高", url: "http://127.0.0.1:11434/v1", protocol: "openai",
    signup: "https://ollama.com/download", keys: "", hint: "不需要密钥，已自动填好", noKey: true,
    daily: [/qwen.*:(7b|8b|14b)/i, /qwen/i, /./], strong: [/:(32b|70b|72b)/i], vision: [/vl|llava/i], fallback: { daily: "qwen2.5:7b" }, price: {} },
  { k: "claude", name: "Claude", tag: "能力强、能看图，国内网络通常连不上", url: "https://api.anthropic.com", protocol: "anthropic",
    signup: "https://console.anthropic.com/", keys: "https://console.anthropic.com/settings/keys", hint: "以 sk-ant- 开头", keyCheck: (k) => k.startsWith("sk-ant-"),
    daily: [/haiku/i], strong: [/sonnet/i, /opus/i], vision: [], selfVision: true, fallback: {}, price: {} },
];

const W = { target: "me", p: null, key: "", models: [], team: null, onDone: null };

function pick(models, pats) {
  for (const re of pats || []) {
    const hit = models.filter((m) => re.test(m)).sort().reverse();
    if (hit.length) return hit[0];
  }
  return "";
}

export async function openWizard(target = "me", onDone = null) {
  W.target = target;
  W.onDone = onDone;
  let teamNote = "";
  if (target === "me") {
    const me = await api("GET", "/api/me");
    if (me.team_llm_configured) teamNote = `<div class="msg info"><b>老师已经配置了团队模型，你不用做任何设置就能使用。</b>只有想用自己的账号（例如团队额度不够、想用别的模型）时才需要继续。<div style="margin-top:6px"><button type="button" data-act="closeModal">不用了，用团队的</button></div></div>`;
  }
  modal(`<h3>${target === "team" ? "接入团队模型" : "接入我的 AI 模型"}（第 1 步 / 共 3 步）</h3>
    ${teamNote}
    <p>选一个服务商。<span class="muted">不知道选哪个：想省钱选<b>智谱 GLM</b>（有免费模型）；想效果好又便宜选 <b>DeepSeek</b> 或 <b>通义千问</b>。</span></p>
    <div class="wizgrid">${WIZ.map((p) => `<button type="button" class="wizcard" data-act="wizPick" data-k="${p.k}"><b>${esc(p.name)}</b><span class="muted">${esc(p.tag)}</span></button>`).join("")}
      <button type="button" class="wizcard" data-act="wizManual"><b>其他 / 手动填写</b><span class="muted">豆包、one-api 网关等</span></button></div>`);
}

actions.wizManual = () => {
  closeModal();
  if (W.target === "team") return toast("请在“团队默认 AI 模型”表单中手动填写");
  actions.mdlNew();
};
actions.mdlWizard = () => openWizard("me", () => { const el = $("#myModels"); if (el) renderMyModels(el); });
actions.teamWizard = () => openWizard("team", () => window.dispatchEvent(new HashChangeEvent("hashchange")));

actions.wizPick = (el) => {
  W.p = WIZ.find((x) => x.k === el.dataset.k);
  const p = W.p;
  modal(`<h3>${esc(p.name)}（第 2 步 / 共 3 步）</h3>
    ${p.noKey ? `<ol class="steps3"><li>在运行工作台的电脑上<a href="${p.signup}" target="_blank" rel="noopener">下载安装 Ollama</a>。</li>
      <li>打开“命令提示符”，输入 <code>ollama pull qwen2.5:7b</code> 下载模型（约 5 GB）。</li><li>回到这里点“下一步”。</li></ol>` :
    `<ol class="steps3">
      <li><a href="${p.signup}" target="_blank" rel="noopener">打开 ${esc(p.name)} 官网</a>，注册并登录（通常用手机号）。${p.k === "qwen" ? "首次需要开通百炼服务。" : ""}</li>
      <li><a href="${p.keys}" target="_blank" rel="noopener">打开 API Key 页面</a>，点“创建 API Key”，复制那一串字符。<span class="muted">（链接打不开的话，在官网找“API Key”或“API 密钥”）</span></li>
      <li>${p.k === "glm" ? "免费模型不需要充值。" : "按需要充值少量金额（几元就能用很久）。"}把密钥粘贴到下面：</li></ol>`}
    <form data-submit="wizKey">
      <input name="key" ${p.noKey ? 'value="ollama"' : ""} required autocomplete="off" placeholder="粘贴 API Key（${esc(p.hint)}）">
      <p class="muted">密钥加密保存在运行工作台的电脑上，只用于你自己的请求，管理员也看不到。建议在服务商后台给这个密钥设置消费上限。</p>
      <div class="row"><button type="button" data-act="wizBack">← 上一步</button><span class="sp"></span><span id="wizMsg" class="muted"></span><button class="pri" type="submit">验证并继续</button></div>
    </form>`);
};
actions.wizBack = () => openWizard(W.target, W.onDone);

actions.wizKey = async (f) => {
  const p = W.p;
  const key = f.key.value.trim().replace(/^Bearer\s+/i, "");
  if (p.keyCheck && !p.keyCheck(key) && !confirm(`${p.name} 的密钥通常${p.hint}，你粘贴的好像不是。仍然继续吗？`)) return;
  $("#wizMsg").textContent = "正在验证密钥…";
  let r;
  try {
    r = await api("POST", "/api/models/probe", { protocol: p.protocol, base_url: p.url, key });
  } catch (e) { $("#wizMsg").textContent = ""; toast(e.message, true); return; }
  W.key = key;
  W.models = r.models || [];
  step3(r.note);
};

function priceOf(m) { return (W.p.price || {})[m] || ["", ""]; }

function step3(note) {
  const p = W.p, ms = W.models;
  const daily = pick(ms, p.daily) || p.fallback.daily || ms[0] || "";
  const strong = pick(ms.filter((m) => m !== daily), p.strong) || (ms.length ? "" : p.fallback.strong || "");
  const vision = pick(ms, p.vision) || (ms.length ? "" : p.fallback.vision || "");
  const opt = (sel, none) => `${none ? `<option value="">${none}</option>` : ""}${ms.map((m) => `<option ${m === sel ? "selected" : ""}>${esc(m)}</option>`).join("")}${sel && !ms.includes(sel) ? `<option selected>${esc(sel)}</option>` : ""}`;
  const [di, dout] = priceOf(daily), [si, so] = priceOf(strong);
  modal(`<h3>${esc(p.name)}（第 3 步 / 共 3 步）</h3>
    <div class="msg info">密钥有效${ms.length ? `，找到 ${ms.length} 个可用模型，已按用途推荐好，一般不用改` : ""}。${note ? esc(note) : ""}</div>
    <form data-submit="wizSave">
      <label class="f">日常模型（便宜、快：拆检索词、速读、概念讲解、普通问题）</label>
      ${ms.length ? `<select name="daily">${opt(daily)}</select>` : `<input name="daily" required value="${esc(daily)}">`}
      <label class="f">难题模型（可选，更强也更贵：引用核验、资料对比、需要推理的问题、改文件）</label>
      ${ms.length ? `<select name="strong">${opt(strong, "不用难题模型")}</select>` : `<input name="strong" value="${esc(strong)}" placeholder="可不填">`}
      ${p.selfVision ? "" : vision ? `<label style="display:block;margin:10px 0"><input type="checkbox" name="addVision" checked> 同时添加识图模型 <code>${esc(vision)}</code>（用于“图片转 LaTeX”）</label><input type="hidden" name="vision" value="${esc(vision)}">` : ""}
      <details ${di === "" ? "" : ""}><summary class="muted">价格（元 / 百万 tokens，用于估算花费；已填参考价，以官网为准）</summary>
        <div class="grid2">
          <div><label class="f">日常模型 输入价</label><input name="pin" type="number" step="any" min="0" value="${di}"></div>
          <div><label class="f">日常模型 输出价</label><input name="pout" type="number" step="any" min="0" value="${dout}"></div>
          <div><label class="f">难题模型 输入价</label><input name="sin" type="number" step="any" min="0" value="${si}"></div>
          <div><label class="f">难题模型 输出价</label><input name="sout" type="number" step="any" min="0" value="${so}"></div>
        </div></details>
      <div class="row" style="margin-top:12px"><button type="button" data-act="wizBack">← 重新选择</button><span class="sp"></span><span id="wizMsg" class="muted"></span><button class="pri" type="submit">保存并自检</button></div>
    </form>`);
}

actions.wizSave = async (f) => {
  const p = W.p;
  const d = Object.fromEntries(new FormData(f).entries());
  const nums = (x) => (x === "" || x === undefined ? 0 : Number(x));
  $("#wizMsg").textContent = "正在保存并自检（约 30–90 秒）…";
  f.querySelector("button[type=submit]").disabled = true;
  try {
    if (W.target === "team") {
      await api("PUT", "/api/settings", { llm_name: p.name, llm_protocol: p.protocol, llm_base_url: p.url, llm_key: W.key, llm_model: d.daily, llm_strong_model: d.strong || "",
        llm_price_in: nums(d.pin), llm_price_out: nums(d.pout), llm_strong_price_in: nums(d.sin), llm_strong_price_out: nums(d.sout), llm_vision: !!p.selfVision });
      const ck = await api("POST", "/api/models/team/check", {});
      finish([{ name: p.name + "（团队）", check: ck }]);
      return;
    }
    const results = [];
    const body = { name: p.name, protocol: p.protocol, base_url: p.url, model: d.daily, key: W.key, strong_model: d.strong || "",
      price_in: nums(d.pin), price_out: nums(d.pout), strong_price_in: nums(d.sin), strong_price_out: nums(d.sout), vision: !!p.selfVision };
    // 同一服务商已经接过：更新原来那张，不重复添加
    const cur = await api("GET", "/api/models");
    const old = cur.mine.find((m) => m.base_url === p.url && !m.name.endsWith(" 识图"));
    const r = old ? await api("PATCH", `/api/models/${old.id}`, body) : await api("POST", "/api/models", body);
    const mp = old ? r.mine.find((m) => m.id === old.id) : r.mine[r.mine.length - 1];
    const ck = await api("POST", `/api/models/${mp.id}/check`, {});
    if (ck.passed) await api("POST", `/api/models/${mp.id}/activate`, { active: true });
    results.push({ name: p.name, check: ck, activated: ck.passed });
    if (d.addVision && d.vision) {
      const rv = await api("POST", "/api/models", { name: p.name + " 识图", protocol: p.protocol, base_url: p.url, model: d.vision, key: W.key, vision: true, ...Object.fromEntries([["price_in", nums((p.price[d.vision] || [])[0])], ["price_out", nums((p.price[d.vision] || [])[1])]]) });
      const vp = rv.mine[rv.mine.length - 1];
      results.push({ name: p.name + " 识图", check: await api("POST", `/api/models/${vp.id}/check`, {}) });
    }
    finish(results);
  } catch (e) {
    $("#wizMsg").textContent = "";
    f.querySelector("button[type=submit]").disabled = false;
    toast(e.message, true);
  }
};

function finish(results) {
  const ok = results[0].check.passed;
  modal(`<h3>${ok ? "接入完成" : "已保存，但自检没有通过"}</h3>
    ${results.map((x) => `<h4>${esc(x.name)} ${checkTag(x.check)}${x.activated ? ' <span class="tag pri">已启用</span>' : ""}</h4>${x.check.strong ? `<p class="muted">难题模型 ${esc(x.check.strong)}：${x.check.strong_ok ? "自检通过，难题会自动交给它" : "自检没有全部通过，暂时只用日常模型"}</p>` : ""}${x.check.passed && !x.check.warn ? "" : checkReport(x.check)}${x.check.passed && x.check.warn ? '<p class="muted">基本通过：已启用。上面没通过的是提醒项，系统在使用时会校验每条引用、丢弃无效引用，不影响正常使用。</p>' : ""}`).join("")}
    ${ok ? `<p>现在可以使用问答、速读、引用核验等功能了。${W.target === "team" ? "组员不需要再做任何设置。可以在下方给每人设置每月额度。" : ""}</p>` : '<p class="muted">没通过的原因通常是模型较小、不擅长按格式输出，或网络问题。可以换一个日常模型再试（设置 → 我的 AI 模型 → 编辑）。</p>'}
    <div class="row"><span class="sp"></span><button class="pri" data-act="wizDone">完成</button></div>`);
}
actions.wizDone = () => { closeModal(); if (W.onDone) W.onDone(); };
