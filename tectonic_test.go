package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 用一个假的 tectonic 程序测试调用方式和结果解析
func TestRunTectonic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("用 shell 脚本模拟")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "tectonic")
	os.WriteFile(fake, []byte(`#!/bin/sh
out=""; file=""; args="$*"
while [ $# -gt 0 ]; do
  case "$1" in --outdir) out="$2"; shift;; -Z) shift;; --*) ;; *) file="$1";; esac; shift
done
stem=$(basename "$file" .tex)
echo "$args" > "$out/args.txt"
if grep -q BAD "$file"; then
  printf '! Undefined control sequence.\nl.3 \\BAD\n' > "$out/$stem.log"
  echo "error: $stem.tex:3: Undefined control sequence" >&2; exit 1
fi
printf 'Output written on %s.pdf (2 pages).\n' "$stem" > "$out/$stem.log"
printf '%%PDF-1.5 fake' > "$out/$stem.pdf"
`), 0o755)
	work, out := filepath.Join(dir, "w"), filepath.Join(dir, "o")
	os.MkdirAll(work, 0o755)
	os.MkdirAll(out, 0o755)
	os.WriteFile(filepath.Join(work, "main.tex"), []byte("\\documentclass{ctexart}\n\\begin{document}\n你好\n\\end{document}\n"), 0o644)
	e := texEngine{Found: true, Dist: "Tectonic", Tectonic: fake}
	res := runTeX(e, work, "main.tex", out, false, 30*time.Second)
	if !res.OK || res.pdf == nil || !strings.Contains(res.Engine, "Tectonic") {
		t.Fatalf("应编译成功：%+v", res)
	}
	args, _ := os.ReadFile(filepath.Join(out, "args.txt"))
	if !strings.Contains(string(args), "--untrusted") || !strings.Contains(string(args), "search-path="+work) {
		t.Fatalf("应以不信任模式运行并加入原文件夹：%s", args)
	}
	os.WriteFile(filepath.Join(work, "bad.tex"), []byte("\\documentclass{article}\n\\begin{document}\n\\BAD\n\\end{document}\n"), 0o644)
	res = runTeX(e, work, "bad.tex", out, false, 30*time.Second)
	if res.OK || len(res.Errors) == 0 {
		t.Fatalf("错误应被识别：%+v", res)
	}
}

func TestTectonicInstall(t *testing.T) {
	if tectonicAssetSuffix() == "" || strings.HasSuffix(tectonicAssetSuffix(), ".zip") {
		t.Skip("本测试模拟 tar.gz 发布包")
	}
	_, srv := newEnv(t)
	tc, _, _, _, _ := setupTeam(t, srv)
	var pkg bytes.Buffer
	gz := gzip.NewWriter(&pkg)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho tectonic 0.15.0\n")
	tw.WriteHeader(&tar.Header{Name: "tectonic", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	gz.Close()
	var fake *httptest.Server
	fake = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "tectonic@0.15.0", "assets": []any{
				map[string]any{"name": "tectonic-0.15.0-" + tectonicAssetSuffix(), "browser_download_url": fake.URL + "/pkg"},
				map[string]any{"name": "tectonic-0.15.0-other.zip", "browser_download_url": fake.URL + "/other"}}})
			return
		}
		w.Write(pkg.Bytes())
	}))
	defer fake.Close()
	oldAPI, oldDir := tectonicAPI, texToolsDir
	tectonicAPI, texToolsDir = fake.URL+"/latest", t.TempDir()
	defer func() { tectonicAPI, texToolsDir = oldAPI, oldDir; findTeX(true) }()
	r := tc.ok("POST", "/api/latex/tectonic", nil)
	p := filepath.Join(texToolsDir, "tectonic", "tectonic")
	if r["path"] != p || r["version"] != "tectonic@0.15.0" {
		t.Fatalf("安装结果不对：%v", r)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, body) {
		t.Fatal("应解压出 tectonic")
	}
	if c := tectonicCandidates(); len(c) == 0 || c[len(c)-1] != p {
		t.Fatalf("应能找到下载的 tectonic：%v", c)
	}
}
