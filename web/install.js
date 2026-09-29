document.getElementById("addr").textContent = location.host;
// 苹果设备把“苹果手机”一节放在前面；安卓则相反
(function () {
  var ua = navigator.userAgent || "";
  var ios = /iPhone|iPad|iPod/.test(ua) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  var a = document.getElementById("android"), i = document.getElementById("ios");
  if (!ios && a && i) a.parentNode.insertBefore(a, i);
})();
