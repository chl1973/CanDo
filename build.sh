#!/usr/bin/env bash
# 构建 Windows 安装包（在 Linux 上交叉编译）。依赖：Go 1.22+、NSIS（makensis）、mingw windres（仅图标资源变更时需要）
set -euo pipefail
export LANG=C.UTF-8 LC_ALL=C.UTF-8
cd "$(dirname "$0")"
VERSION=1.14.0
mkdir -p dist
echo "== 测试"
go vet ./...
go test -count=1 ./...
echo "== 安卓 App（内置到电脑程序中，手机可从电脑下载）"
if [ ! -f android/release.keystore ]; then
  echo "（没有 android/release.keystore 安卓签名密钥，沿用 web/download/ 里已有的 APK；要重新打包 App 请把密钥放回原位）"
  cp web/download/keyan-workbench.apk "dist/CanDo_安卓App_v$VERSION.apk"
elif command -v aapt >/dev/null && command -v dalvik-exchange >/dev/null; then
  ./android/build_apk.sh ../web/download/keyan-workbench.apk >/dev/null 2>&1
  cp web/download/keyan-workbench.apk "dist/CanDo_安卓App_v$VERSION.apk"
else
  echo "（未安装安卓打包工具，使用已有的 APK）"
fi
echo "== 编译 Windows 程序"
if [ build/app.rc -nt rsrc_windows_amd64.syso ] || [ build/app.ico -nt rsrc_windows_amd64.syso ]; then
  (cd build && x86_64-w64-mingw32-windres --preprocessor=cpp --preprocessor-arg=-P -c 65001 -i app.rc -O coff -o ../rsrc_windows_amd64.syso)
fi
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui" -o dist/CanDo.exe .
# 说明文件转为带 BOM 的 UTF-8 + CRLF，旧版记事本也能正确显示
printf '\xEF\xBB\xBF' > dist/使用说明.txt
sed 's/$/\r/' build/使用说明.txt >> dist/使用说明.txt
echo "== 打包安装程序"
(cd build && makensis -V2 -DVERSION=$VERSION installer.nsi)
echo "== 免安装版"
rm -rf "dist/CanDo可为_免安装版_v$VERSION" && mkdir "dist/CanDo可为_免安装版_v$VERSION"
cp dist/CanDo.exe dist/使用说明.txt "dist/CanDo可为_免安装版_v$VERSION/"
(cd dist && rm -f "CanDo可为_免安装版_v$VERSION.zip" && zip -qr "CanDo可为_免安装版_v$VERSION.zip" "CanDo可为_免安装版_v$VERSION")
ls -la dist
