//go:build e2e

package main

import "os"

// 仅用于端到端测试构建（go build -tags e2e）：把 OpenAlex 指向本地模拟服务，允许下载本地 PDF。
func init() {
	if b := os.Getenv("KY_E2E_OPENALEX"); b != "" {
		openAlexBase = b
		allowPrivateFetch = true
	}
	if b := os.Getenv("KY_E2E_CROSSREF"); b != "" {
		crossrefBase = b
	}
	if b := os.Getenv("KY_E2E_ZOTERO"); b != "" {
		zoteroLocalBase, zoteroWebBase = b, b
		allowPrivateFetch = true
	}
}
