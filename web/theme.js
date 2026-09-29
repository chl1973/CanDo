// 外观（浅色 / 深色 / 跟随系统）：在页面画出来之前就设置好，避免闪白。选择只保存在这台设备的浏览器里。
(function () {
  var t = "auto";
  try { t = localStorage.getItem("kyTheme") || "auto"; } catch (e) { t = "auto"; }
  if (t === "light" || t === "dark") document.documentElement.setAttribute("data-theme", t);
})();
