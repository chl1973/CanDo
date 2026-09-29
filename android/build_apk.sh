#!/usr/bin/env bash
# 用 Ubuntu 软件源中的 Android 工具手工打包 APK（不需要 Gradle / Android Studio）
# 依赖：apt install android-sdk-platform-23 aapt dalvik-exchange zipalign apksigner openjdk
set -euo pipefail
cd "$(dirname "$0")"
export LANG=C.UTF-8 LC_ALL=C.UTF-8
SDK=/usr/lib/android-sdk/platforms/android-23/android.jar
OUT=$(realpath -m "${1:-../web/download/keyan-workbench.apk}")
rm -rf build && mkdir -p build/gen build/obj
aapt package -f -m -J build/gen -M AndroidManifest.xml -S res -I "$SDK"
javac -nowarn -Xlint:-options --release 8 -encoding UTF-8 -classpath "$SDK" -d build/obj $(find src build/gen -name '*.java')
dalvik-exchange --dex --output=build/classes.dex build/obj
aapt package -f -M AndroidManifest.xml -S res -I "$SDK" -F build/app.unsigned.apk
(cd build && aapt add app.unsigned.apk classes.dex >/dev/null)
zipalign -f 4 build/app.unsigned.apk build/app.aligned.apk
if [ ! -f release.keystore ]; then
  # 没有签名密钥时不要悄悄新建：新密钥签的 App 不能覆盖安装旧版。确实要新建时加 NEW_KEYSTORE=1
  if [ "${NEW_KEYSTORE:-}" != "1" ]; then
    echo "缺少 android/release.keystore（安卓签名密钥，从私下备份放回）。确实要新建密钥请加 NEW_KEYSTORE=1" >&2
    exit 3
  fi
  keytool -genkeypair -keystore release.keystore -storepass kyws2026 -keypass kyws2026 -alias kyws \
    -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Keyan Workbench, O=Keyan, C=CN" >/dev/null 2>&1
fi
mkdir -p "$(dirname "$OUT")"
apksigner sign --v4-signing-enabled false --ks release.keystore --ks-pass pass:kyws2026 --ks-key-alias kyws --out "$OUT" build/app.aligned.apk
apksigner verify --print-certs "$OUT" | head -3
ls -la "$OUT"
