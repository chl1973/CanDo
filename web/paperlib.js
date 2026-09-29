// 论文库：库中论文（上传、OCR、AI 速读）+ 检索添加（在线检索、Zotero、题录导入）+ 深度调研，放在一处
import { $, $$, esc, actions } from "/core.js";
import { renderMaterials } from "/kb.js";
import { renderPapers } from "/papers.js";
import { renderResearch } from "/research.js";

const PL = { sub: "list", el: null, pid: "", opts: {} };
const SUBS = [["list", "库中论文"], ["group", "全组共享"], ["add", "检索添加"], ["research", "深度调研"]];

export function renderPaperLib(el, pid, opts = {}, sub) {
  PL.el = el; PL.pid = pid || ""; PL.opts = opts;
  if (sub) PL.sub = sub;
  const subs = PL.pid ? SUBS.filter(([k]) => k !== "group") : SUBS; // 项目论文库本来就是共享的
  if (PL.pid && PL.sub === "group") PL.sub = "list";
  el.innerHTML = `<div class="seg" id="plSeg">${subs.map(([k, n]) => [k, k === "list" && !PL.pid ? "我的论文" : n]).map(([k, n]) => `<button class="${PL.sub === k ? "on" : ""}" data-act="plSub" data-s="${k}">${n}</button>`).join("")}</div><div id="plBody"></div>`;
  show();
}

function show() {
  const b = $("#plBody");
  if (!b) return;
  if (PL.sub === "add") return renderPapers(b, PL.pid, { readonly: PL.opts.readonly });
  if (PL.sub === "research") return renderResearch(b, PL.pid, { readonly: PL.opts.readonly });
  if (PL.sub === "group") return renderMaterials(b, "", { ...PL.opts, scope: "group" });
  return renderMaterials(b, PL.pid, PL.opts);
}

actions.plSub = (el) => {
  PL.sub = el.dataset.s;
  $$("#plSeg button").forEach((x) => x.classList.toggle("on", x === el));
  show();
};

// 其他页面收入论文后可以切回“库中论文”
export function showLibList() {
  if (!$("#plSeg")) return;
  PL.sub = "list";
  $$("#plSeg button").forEach((x) => x.classList.toggle("on", x.dataset.s === "list"));
  show();
}
