package main

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

// 机器可读的接口契约 docs/cando-tools.openapi.json 必须和程序里实际执行的规则一致。
// 改了 extools.go 里的上限、名字规则，或者改了契约，这个测试会提醒两边一起改。
func TestExtToolsOpenAPIContract(t *testing.T) {
	raw, err := os.ReadFile("docs/cando-tools.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("契约不是有效的 JSON：%v", err)
	}
	get := func(path ...string) any {
		var cur any = spec
		for _, k := range path {
			m, ok := cur.(map[string]any)
			if !ok {
				t.Fatalf("契约里找不到 %v（卡在 %s）", path, k)
			}
			cur, ok = m[k]
			if !ok {
				t.Fatalf("契约里找不到 %v（缺 %s）", path, k)
			}
		}
		return cur
	}
	num := func(path ...string) int {
		f, ok := get(path...).(float64)
		if !ok {
			t.Fatalf("%v 不是数字", path)
		}
		return int(f)
	}
	eq := func(what string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s：契约写的是 %v，程序里是 %v", what, got, want)
		}
	}

	eq("协议名", get("x-cando-protocol"), extProtocol)
	eq("协议名的匹配规则", get("components", "schemas", "Manifest", "properties", "protocol", "pattern"), "^"+extProtocol)
	for _, p := range []string{"/tools", "/tools/{name}"} {
		get("paths", p)
	}
	get("paths", "/tools", "get", "responses", "200")
	get("paths", "/tools/{name}", "post", "requestBody")

	// 上限
	lim := func(k string) int { return num("x-cando-limits", k) }
	eq("工具清单大小", lim("manifest_max_bytes"), extManifestMax)
	eq("清单缓存秒数", lim("manifest_cache_seconds"), int(extCacheTTL.Seconds()))
	eq("每个服务的工具数", lim("tools_per_service"), extMaxTools)
	eq("一次调用的输入文件数", lim("input_files_per_call"), extInFilesMax)
	eq("输入文件大小", lim("input_file_max_bytes"), extInFileMax)
	eq("一次交回的文件数", lim("output_files_per_call"), extOutFilesMax)
	eq("响应大小", lim("response_max_bytes"), extResponseMax)
	eq("text 长度", lim("text_max_chars"), extTextMax)
	eq("默认超时", lim("timeout_default_seconds"), extDefaultTimout)
	eq("最长超时", lim("timeout_max_seconds"), extMaxTimeout)

	// 结构里的约束
	S := func(path ...string) []string { return append([]string{"components", "schemas"}, path...) }
	eq("工具名规则", get(S("Tool", "properties", "name", "pattern")...), reExtTool.String())
	eq("路径里的工具名规则", get("paths", "/tools/{name}", "post", "parameters").([]any)[0].(map[string]any)["schema"].(map[string]any)["pattern"], reExtTool.String())
	eq("replaces 的规则", get(S("FileOut", "properties", "replaces", "pattern")...), reExtTool.String())
	eq("工具数上限", num(S("Manifest", "properties", "tools", "maxItems")...), extMaxTools)
	eq("超时上限", num(S("Tool", "properties", "timeout", "maximum")...), extMaxTimeout)
	eq("超时默认值", num(S("Tool", "properties", "timeout", "default")...), extDefaultTimout)
	eq("text 上限", num(S("CallSuccess", "properties", "text", "maxLength")...), extTextMax)
	eq("交回文件数上限", num(S("CallSuccess", "properties", "files", "maxItems")...), extOutFilesMax)

	var blocked []string
	for k := range extBlockedExt {
		blocked = append(blocked, k)
	}
	sort.Strings(blocked)
	var listed []string
	for _, v := range get("x-cando-limits", "rejected_output_extensions").([]any) {
		listed = append(listed, v.(string))
	}
	sort.Strings(listed)
	eq("不保存的扩展名", listed, blocked)

	// 截短的长度：拿超长的清单过一遍真正的解析函数，结果的长度应等于契约里写的 maxLength
	long := func(n int) string {
		b := make([]rune, n)
		for i := range b {
			b[i] = '字'
		}
		return string(b)
	}
	props := map[string]any{}
	for i := 0; i < 30; i++ {
		props["p"+itoa(i)] = map[string]any{"type": "string", "description": long(200), "enum": []any{long(50), "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}}
	}
	props["save_to"] = map[string]any{"type": "string"}
	props["reason"] = map[string]any{"type": "string"}
	man, _ := json.Marshal(map[string]any{"protocol": extProtocol, "service": map[string]any{"name": long(100), "version": long(100)},
		"tools": []any{map[string]any{"name": "t", "title": long(100), "description": long(1000), "example": long(500), "timeout": 99999,
			"params": map[string]any{"type": "object", "properties": props}}}})
	m, err := parseExtManifest(man)
	if err != nil || len(m.Tools) != 1 {
		t.Fatalf("解析失败：%v", err)
	}
	tl := m.Tools[0]
	runes := func(s string) int { // 截短后末尾带一个省略号，不算在长度里
		r := []rune(s)
		if len(r) > 0 && r[len(r)-1] == '…' {
			return len(r) - 1
		}
		return len(r)
	}
	eq("服务名长度", num(S("Service", "properties", "name", "maxLength")...), runes(m.ServiceName))
	eq("服务版本长度", num(S("Service", "properties", "version", "maxLength")...), runes(m.Version))
	eq("标题长度", num(S("Tool", "properties", "title", "maxLength")...), runes(tl.Title))
	eq("说明长度", num(S("Tool", "properties", "description", "maxLength")...), runes(tl.Description))
	eq("示例长度", num(S("Tool", "properties", "example", "maxLength")...), runes(tl.Example))
	eq("超时被改成上限", tl.Timeout, extMaxTimeout)
	eq("参数个数", num(S("ParamsSchema", "properties", "properties", "maxProperties")...), len(tl.Params))
	eq("契约里的参数个数上限", lim("params_per_tool"), len(tl.Params))
	eq("参数说明长度", num(S("ParamSpec", "properties", "description", "maxLength")...), runes(tl.Params[0].Desc))
	eq("可选值个数", num(S("ParamSpec", "properties", "enum", "maxItems")...), len(tl.Params[0].Enum))
	eq("契约里的可选值个数上限", lim("enum_values_per_param"), len(tl.Params[0].Enum))
	var reserved []string
	for _, v := range get("x-cando-limits", "reserved_param_names").([]any) {
		reserved = append(reserved, v.(string))
	}
	for _, p := range tl.Params {
		for _, r := range reserved {
			if p.Name == r {
				t.Errorf("保留的参数名 %s 没有被忽略", r)
			}
		}
	}
	eq("保留的参数名", reserved, []string{"save_to", "reason"})

	// 契约里的示例清单，CanDo 必须能照原样接受
	ex, _ := json.Marshal(get("paths", "/tools", "get", "responses", "200", "content", "application/json", "example"))
	em, err := parseExtManifest(ex)
	if err != nil || len(em.Tools) != 1 {
		t.Fatalf("契约里的示例清单解析失败：%v", err)
	}
	et := em.Tools[0]
	if et.Name != "bar_chart" || et.Title != "画柱状图" || et.Example == "" || !et.MakesFiles || et.Timeout != 60 || len(et.Params) != 2 {
		t.Errorf("契约里的示例清单解析结果不对：%+v", et)
	}
	file := false
	for _, p := range et.Params {
		if p.Name == "data" && p.File {
			file = true
		}
		if p.Name == "values" && !p.Required {
			t.Error("示例里的 values 应该是必填")
		}
	}
	if !file {
		t.Error("示例里的 data 应该被识别为文件参数")
	}
}
