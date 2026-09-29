// 右上角“后台任务”：显示正在运行和已完成的 AI 任务；完成时提示，点“查看结果”跳到对应页面
import { $, esc, api, toast, modal, closeModal, actions, fmtTime, WATCHED } from "/core.js";

const J = { list: [], running: 0, timer: 0, known: {} };
const ST = { queued: ["排队中", ""], running: ["进行中", "pri"], done: ["完成", "ok"], error: ["出错", "bad"] };

export function startJobCenter() {
  if (J.started) return;
  J.started = true;
  window.addEventListener("ky-jobs", () => poll(true));
  poll(true);
}

async function poll(soon) {
  clearTimeout(J.timer);
  let r;
  try { r = await api("GET", "/api/jobs"); } catch { J.timer = setTimeout(poll, 20000); return; }
  const first = !J.loaded;
  J.loaded = true;
  for (const j of r.jobs) {
    const was = J.known[j.id];
    J.known[j.id] = j.status;
    if (!first && was && was !== j.status && (j.status === "done" || j.status === "error") && !WATCHED.has(j.id)) {
      toast(`${j.status === "done" ? "✓" : "✗"} ${j.label}${j.title ? "：" + j.title : ""} ${j.status === "done" ? "已完成，点右上角“后台任务”查看" : "没有成功"}`, j.status === "error");
    }
  }
  J.list = r.jobs;
  J.running = r.running;
  render();
  if ($("#jobsModal")) renderModal();
  J.timer = setTimeout(poll, r.running ? 2000 : 20000);
}

function render() {
  const b = $("#jobsBtn");
  if (!b) return;
  b.classList.toggle("hide", !J.list.length);
  b.classList.toggle("busy", J.running > 0);
  $("#jobsN").textContent = J.running ? `${J.running} 个进行中` : `${J.list.length}`;
}

function renderModal() {
  const box = $("#jobsModal");
  if (!box) return;
  box.innerHTML = J.list.length ? J.list.map((j) => `<div class="jobrow"><div class="row"><b>${esc(j.label)}</b><span class="tag ${ST[j.status][1]}">${ST[j.status][0]}</span><span class="sp"></span><span class="muted">${fmtTime(j.created)}</span></div>
      ${j.title ? `<div class="muted">${esc(j.title)}</div>` : ""}${j.error ? `<div class="msg">${esc(j.error)}</div>` : ""}
      <div class="row" style="margin-top:4px">${j.status === "done" && j.link ? `<button class="sm pri" data-act="jobOpen" data-id="${esc(j.id)}">查看结果</button>` : ""}
        ${j.status === "done" || j.status === "error" ? `<button class="sm" data-act="jobDel" data-id="${esc(j.id)}">移除</button>` : '<span class="muted">可以关掉这个页面，任务会在工作台后台继续运行</span>'}</div></div>`).join("")
    : '<p class="muted">没有后台任务。</p>';
}

actions.jobsOpen = () => {
  modal(`<h3>后台任务</h3><p class="muted">AI 起草、列提纲、格式检查、问答、速读等耗时操作都在工作台后台运行：可以离开页面、同时发起多个任务（每人最多 3 个同时运行，其余排队）。工作台程序关闭后，未完成的任务会中断。</p><div id="jobsModal"></div>`);
  renderModal();
};
actions.jobOpen = (el) => {
  const j = J.list.find((x) => x.id === el.dataset.id);
  if (!j) return;
  window.KY_OPEN_JOB = j;
  closeModal();
  if (location.hash === j.link) window.dispatchEvent(new HashChangeEvent("hashchange"));
  else location.hash = j.link;
};
actions.jobDel = async (el) => { await api("DELETE", `/api/jobs/${el.dataset.id}`); poll(true); };
