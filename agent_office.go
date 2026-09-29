package main

// 本机智能体的 Office 工具（1.14）：读 Word/Excel/PPT、按段落改 Word、Markdown 生成 Word、分析模板格式、提取 Word 图片、
// 看授权文件夹里的图片、联网搜索；以及模型用错工具调用格式时的“抢救”。
// 所有写入都和其他修改一样：先提交建议，用户确认后才写，写之前备份，可以撤销。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type binFile struct {
	real    string
	data    []byte
	backup  string
	existed bool
	created bool
}

var officeKind = map[string]string{".docx": "docx", ".xlsx": "xlsx", ".xlsm": "xlsx", ".pptx": "pptx"}
var oldOfficeExt = map[string]string{".doc": ".docx", ".xls": ".xlsx", ".ppt": ".pptx", ".wps": ".docx", ".et": ".xlsx", ".dps": ".pptx"}
var imageMime = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif", ".bmp": "image/bmp"}

const officeFileMax = 50 << 20

func shaBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// currentOffice 返回文件内容；如果有还没确认的 Word 修改，返回修改后的版本（连续修改时在上一次的基础上改）
func (a *App) currentOffice(s *agentSession, real string) ([]byte, *agentChange, error) {
	s.mu.Lock()
	for i := len(s.Changes) - 1; i >= 0; i-- {
		c := s.Changes[i]
		if c.real == real && c.Status == "pending" && c.Kind == "office" && len(c.bins) == 1 {
			d := c.bins[0].data
			s.mu.Unlock()
			return d, c, nil
		}
	}
	s.mu.Unlock()
	st, err := os.Stat(real)
	if err != nil {
		return nil, nil, errNotFound("文件不存在：" + filepath.Base(real))
	}
	if st.IsDir() {
		return nil, nil, errBad("这是文件夹，请用 list_dir")
	}
	if st.Size() > officeFileMax {
		return nil, nil, errBad("文件超过 50 MB，助手不读取")
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return nil, nil, errBad("无法打开文件（可能正在 Word/Excel 中打开并被锁定）")
	}
	return b, nil, nil
}

// toolReadOffice 读取 Office 文件的文字；Word 带段落编号（修改时用）
func (a *App) toolReadOffice(s *agentSession, me *Me, clean, real, kind string, start, max int) (string, string, error) {
	data, pend, err := a.currentOffice(s, real)
	if err != nil {
		return "", "", err
	}
	var out string
	switch kind {
	case "docx":
		d, err := parseDocxDoc(data)
		if err != nil {
			return "", "", err
		}
		if max <= 0 || max > 800 {
			max = 400
		}
		if start < 0 {
			start = 0
		}
		out = d.outline(start, max, agentReadChars)
	case "xlsx":
		out, err = xlsxText(data, agentReadChars)
	case "pptx":
		out, err = pptxText(data, agentReadChars)
	}
	if err != nil {
		return "", "", err
	}
	head := clean + "\n"
	if pend != nil {
		head = clean + "（以下是包含待确认修改的版本）\n"
	}
	return head + out, "", nil
}

func officeHint(real string) error {
	ext := strings.ToLower(filepath.Ext(real))
	if n := oldOfficeExt[ext]; n != "" {
		return errBad("这是旧版 " + ext + " 格式，助手读不了。请用户在 Office/WPS 里“另存为 " + n + "”后再试")
	}
	if ext == ".pdf" {
		return errBad("PDF 请上传到“论文库”（会提取文字，扫描件可以 OCR），然后用 search_library 检索；只想看某一页时，可以请用户截图附上")
	}
	return nil
}

// ---------------- 修改 Word ----------------

func (a *App) toolDocxEdit(s *agentSession, me *Me, p string, rawEdits any, reason string) (string, string, error) {
	clean, real, err := a.agentPath(me, p, true)
	if err != nil {
		return "", "", err
	}
	if officeKind[strings.ToLower(filepath.Ext(real))] != "docx" {
		return "", "", errBad("docx_edit 只能修改 .docx 文件")
	}
	var edits []docxEdit
	b, _ := json.Marshal(rawEdits)
	if err := json.Unmarshal(b, &edits); err != nil || len(edits) == 0 {
		return "", "", errBad(`edits 应为列表，例如 [{"op":"replace","index":3,"text":"新的文字"},{"op":"insert_after","index":5,"text":"新段落"},{"op":"delete","index":7}]`)
	}
	data, prev, err := a.currentOffice(s, real)
	if err != nil {
		return "", "", err
	}
	d, err := parseDocxDoc(data)
	if err != nil {
		return "", "", err
	}
	out, diff, err := d.applyEdits(edits)
	if err != nil {
		return "", "", err
	}
	var warns []string
	if strings.Contains(d.xml, "<w:ins ") || strings.Contains(d.xml, "<w:del ") {
		warns = append(warns, "文档里有修订记录（修订模式），被改动的段落会丢失修订标记")
	}
	for _, e := range edits {
		if e.Op == "replace" && e.Index >= 0 && e.Index < len(d.paras) {
			seg := d.xml[d.paras[e.Index].Start:d.paras[e.Index].End]
			rp := map[string]bool{}
			for _, run := range reFirstRun.FindAllString(seg, -1) {
				rp[reRPr.FindString(run)] = true
			}
			if len(rp) > 1 {
				warns = append(warns, "第 "+itoa(e.Index)+" 段原来有不同格式的文字（例如部分加粗），替换后整段统一为开头文字的格式")
			}
		}
	}
	oldHash := ""
	s.mu.Lock()
	if prev != nil {
		oldHash = prev.oldHash
		prev.Status, prev.Note = "rejected", "已合并到后面的一条修改"
	}
	s.mu.Unlock()
	if prev == nil {
		oldHash = shaBytes(data)
	}
	c := &agentChange{ID: newID(), Kind: "office", Sub: "modify", Path: clean, real: real, Reason: reason, Status: "pending", At: now(),
		Warnings: nonNilS(warns), Diff: diff, oldHash: oldHash, bins: []binFile{{real: real, data: out, existed: true}}}
	if prev != nil {
		c.Diff = append(append([]diffLine{}, prev.Diff...), append([]diffLine{{Op: "~", Text: "……（接着上一条修改）"}}, diff...)...)
	}
	for _, x := range c.Diff {
		switch x.Op {
		case "+":
			c.Added++
		case "-":
			c.Removed++
		}
	}
	s.mu.Lock()
	s.Changes = append(s.Changes, c)
	s.mu.Unlock()
	return "已提交对 " + clean + " 的 " + itoa(len(edits)) + " 处修改（保留原有格式、图片和表格），请用户在右侧确认。改完后段落编号会变化，再改之前请重新 read_file。", c.ID, nil
}

// ---------------- 新建 Word ----------------

func (a *App) toolMakeDocx(s *agentSession, me *Me, p, content, title, lang, reason string) (string, string, error) {
	clean, real, err := a.agentPath(me, p, true)
	if err != nil {
		return "", "", err
	}
	if !strings.EqualFold(filepath.Ext(real), ".docx") {
		return "", "", errBad("文件名要以 .docx 结尾")
	}
	if _, err := os.Stat(real); err == nil {
		return "", "", errBad("文件已存在。修改已有的 Word 请先 read_file 再用 docx_edit（不会弄丢图片、表格和排版）；要新建请换一个文件名")
	}
	s.mu.Lock()
	for _, c := range s.Changes {
		if c.real == real && c.Status == "pending" {
			s.mu.Unlock()
			return "", "", errBad("这个文件已经有一条待确认的修改，请等用户处理后再提交，或换一个文件名")
		}
	}
	s.mu.Unlock()
	if strings.TrimSpace(content) == "" {
		return "", "", errBad("content 是空的")
	}
	if utf8.RuneCountInString(content) > 200000 {
		return "", "", errBad("内容超过 20 万字")
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(real), filepath.Ext(real))
	}
	en := lang == "en"
	data, n, err := markdownDocx(content, title, en)
	if err != nil {
		return "", "", err
	}
	d, err := parseDocxDoc(data)
	if err != nil {
		return "", "", errBad("生成的文档自检失败")
	}
	var diff []diffLine
	for i, p := range d.paras {
		if i >= 300 {
			diff = append(diff, diffLine{Op: "~", Text: "……还有 " + itoa(len(d.paras)-i) + " 段"})
			break
		}
		if t := strings.TrimSpace(p.Text); t != "" {
			diff = append(diff, diffLine{Op: "+", Text: "(" + d.styleName(p.StyleID) + ") " + clipRunes(t, 300), New: i + 1})
		}
	}
	if len(d.tables) > 0 {
		diff = append(diff, diffLine{Op: "~", Text: "另有 " + itoa(len(d.tables)) + " 个表格"})
	}
	c := &agentChange{ID: newID(), Kind: "office", Sub: "create", Path: clean, real: real, Reason: reason, Status: "pending", At: now(),
		Warnings: []string{}, Diff: diff, Added: len(diff), bins: []binFile{{real: real, data: data}}}
	s.mu.Lock()
	s.Changes = append(s.Changes, c)
	s.mu.Unlock()
	return "已生成 Word（" + itoa(n) + " 段，宋体小四、1.5 倍行距、黑体标题），请用户在右侧确认保存到 " + clean, c.ID, nil
}

// ---------------- 模板格式、提取图片 ----------------

func (a *App) toolDocxFormat(s *agentSession, me *Me, p string) (string, string, error) {
	clean, real, err := a.agentPath(me, p, false)
	if err != nil {
		return "", "", err
	}
	if officeKind[strings.ToLower(filepath.Ext(real))] != "docx" {
		if e := officeHint(real); e != nil {
			return "", "", e
		}
		return "", "", errBad("docx_format 只能分析 .docx 模板")
	}
	data, _, err := a.currentOffice(s, real)
	if err != nil {
		return "", "", err
	}
	sum, err := docxFormatSummary(data)
	if err != nil {
		return "", "", err
	}
	return clean + " 的版式：\n" + sum, "", nil
}

func (a *App) toolDocxImages(s *agentSession, me *Me, p, outDir, reason string) (string, string, error) {
	clean, real, err := a.agentPath(me, p, false)
	if err != nil {
		return "", "", err
	}
	data, _, err := a.currentOffice(s, real)
	if err != nil {
		return "", "", err
	}
	d, err := parseDocxDoc(data)
	if err != nil {
		return "", "", err
	}
	if len(d.images) == 0 {
		return clean + " 里没有图片。", "", nil
	}
	if strings.TrimSpace(outDir) == "" {
		var b strings.Builder
		b.WriteString(clean + " 里有 " + itoa(len(d.images)) + " 张图片：")
		for _, n := range d.images {
			b.WriteString(path.Base(n) + " ")
		}
		b.WriteString("\n要保存出来，请给 out_dir（授权的可修改文件夹）。")
		return b.String(), "", nil
	}
	cleanOut, realOut, err := a.agentPath(me, outDir, true)
	if err != nil {
		return "", "", err
	}
	if st, err := os.Stat(realOut); err == nil && !st.IsDir() {
		return "", "", errBad("out_dir 是一个文件，不是文件夹")
	}
	c := &agentChange{ID: newID(), Kind: "files", Path: cleanOut, real: realOut, Reason: reason, Status: "pending", At: now(), Warnings: []string{}}
	used := map[string]bool{}
	total := 0
	for _, n := range d.images {
		if len(c.bins) >= 100 {
			break
		}
		b := zipRead(d.zr, n)
		total += len(b)
		if total > officeFileMax {
			break
		}
		base := path.Base(n)
		stem, ext := strings.TrimSuffix(base, path.Ext(base)), path.Ext(base)
		name := base
		for i := 1; ; i++ {
			if _, err := os.Stat(filepath.Join(realOut, name)); err != nil && !used[name] {
				break
			}
			name = stem + "_" + itoa(i) + ext
		}
		used[name] = true
		c.bins = append(c.bins, binFile{real: filepath.Join(realOut, name), data: b})
		c.Diff = append(c.Diff, diffLine{Op: "+", Text: name + "（" + itoa((len(b)+1023)/1024) + " KB）", New: len(c.bins)})
	}
	c.Added = len(c.bins)
	s.mu.Lock()
	s.Changes = append(s.Changes, c)
	s.mu.Unlock()
	return "已提交：把 " + itoa(len(c.bins)) + " 张图片保存到 " + cleanOut + "，请用户确认。确认后可以用 recognize_image {\"path\"} 看某一张。", c.ID, nil
}

// imageFromPath 读取授权文件夹里的图片，供 recognize_image 使用
func (a *App) imageFromPath(me *Me, p string) (chatImage, error) {
	_, real, err := a.agentPath(me, p, false)
	if err != nil {
		return chatImage{}, err
	}
	mime := imageMime[strings.ToLower(filepath.Ext(real))]
	if mime == "" {
		return chatImage{}, errBad("只能看 png、jpg、webp、gif、bmp 图片")
	}
	st, err := os.Stat(real)
	if err != nil {
		return chatImage{}, errNotFound("图片不存在")
	}
	if st.Size() > 10<<20 {
		return chatImage{}, errBad("图片超过 10 MB")
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return chatImage{}, errBad("无法读取图片")
	}
	return chatImage{Mime: mime, Data: b}, nil
}

// ---------------- 写入与撤销 ----------------

func (a *App) applyBins(c *agentChange) error {
	switch c.Kind {
	case "office":
		b := &c.bins[0]
		if c.Sub == "modify" {
			cur, err := os.ReadFile(c.real)
			if err != nil {
				return errBad("读不到原文件（可能正在 Word 中打开并被锁定），没有写入")
			}
			if shaBytes(cur) != c.oldHash {
				c.Status, c.Note = "conflict", "文件在助手读取之后被改动过（或正在 Word 中编辑），为避免覆盖你的修改，没有写入。请保存并关闭 Word 后，让助手重新读取再改。"
				return errBad(c.Note)
			}
			b.backup = filepath.Join(a.store.dir, "agent_backups", time.Now().Format("20060102"), c.ID+"_"+filepath.Base(c.real))
			os.MkdirAll(filepath.Dir(b.backup), 0o700)
			if err := os.WriteFile(b.backup, cur, 0o600); err != nil {
				return errBad("备份原文件失败，没有写入")
			}
		} else if _, err := os.Stat(c.real); err == nil {
			return errBad("目标已经存在，没有执行")
		}
		if err := os.WriteFile(c.real, b.data, 0o644); err != nil {
			return errBad("写入失败（文件可能正在 Word 中打开）：" + err.Error())
		}
		b.created = c.Sub == "create"
	case "files":
		if _, err := os.Stat(c.real); err != nil {
			if err := os.MkdirAll(c.real, 0o755); err != nil {
				return errBad("无法创建文件夹")
			}
			c.dirCreated = true
		}
		for i := range c.bins {
			b := &c.bins[i]
			if _, err := os.Stat(b.real); err == nil {
				continue // 已有同名文件：跳过，不覆盖
			}
			if err := os.WriteFile(b.real, b.data, 0o644); err != nil {
				return errBad("写入失败：" + err.Error())
			}
			b.created = true
		}
	}
	c.Status, c.Note = "applied", ""
	return nil
}

func (a *App) undoBins(c *agentChange) error {
	switch c.Kind {
	case "office":
		b := c.bins[0]
		if b.backup != "" {
			old, err := os.ReadFile(b.backup)
			if err != nil {
				return errBad("找不到备份：" + err.Error())
			}
			if err := os.WriteFile(c.real, old, 0o644); err != nil {
				return errBad("恢复失败（文件可能正在 Word 中打开）")
			}
		} else if b.created {
			os.Remove(c.real)
		}
	case "files":
		for _, b := range c.bins {
			if b.created {
				os.Remove(b.real)
			}
		}
		if c.dirCreated {
			os.Remove(c.real) // 只在文件夹空了时才会成功
		}
	}
	c.Status = "undone"
	return nil
}

// ---------------- 工具调用格式抢救 ----------------

var (
	reInvoke = regexp.MustCompile(`(?s)invoke\s+name\s*=\s*"([^"]+)"[^>]*>(.*?)(?:</[^>]*invoke>|$)`)
	reParam  = regexp.MustCompile(`(?s)parameter\s+name\s*=\s*"([^"]+)"[^>]*>(.*?)</[^>]*parameter>`)
	reFnJSON = regexp.MustCompile(`(?s)\{\s*"name"\s*:\s*"([a-z_]+)"\s*,\s*"(?:arguments|parameters)"\s*:\s*(\{.*\})\s*\}`)
)

// rescueToolMarkup 有的模型（如 DeepSeek）偶尔不按 JSON 输出，而是写出 <invoke name="…"><parameter …> 或
// {"name":…,"arguments":…} 这样的工具调用标记。能识别出来时，转换成我们的格式继续执行（每次只取第一个）。
func rescueToolMarkup(raw string) map[string]any {
	if m := reInvoke.FindStringSubmatch(raw); m != nil && knownTool(m[1]) {
		args := map[string]any{}
		for _, p := range reParam.FindAllStringSubmatch(m[2], -1) {
			v := strings.TrimSpace(p[2])
			var j any
			if (strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{")) && json.Unmarshal([]byte(v), &j) == nil {
				args[p[1]] = j
			} else {
				args[p[1]] = v
			}
		}
		return map[string]any{"tool": m[1], "args": args}
	}
	if m := reFnJSON.FindStringSubmatch(raw); m != nil && knownTool(m[1]) {
		var args map[string]any
		if json.Unmarshal([]byte(m[2]), &args) == nil {
			return map[string]any{"tool": m[1], "args": args}
		}
	}
	return nil
}

func knownTool(t string) bool {
	switch t {
	case "list_folders", "list_dir", "search_files", "read_file", "edit_file", "write_file", "file_op", "check_latex", "compile_latex", "open",
		"run_command", "search_library", "search_papers", "add_paper", "fetch_url", "search_web", "recognize_image", "remember", "forget", "use_skill",
		"docx_edit", "make_docx", "docx_format", "docx_images":
		return true
	}
	return false
}
