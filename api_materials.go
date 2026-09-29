package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxFileBytes = 50 << 20
const maxMaterialsPerUser = 500

var supportedExt = map[string]string{".pdf": "pdf", ".md": "md", ".markdown": "md", ".txt": "txt"}

// canUseMaterial：个人资料仅本人；项目资料为项目成员与指导老师（调用方持有锁）。
func canUseMaterial(db *DB, me *Me, m *Material) bool {
	if m == nil || m.DeletedAt != nil {
		return false
	}
	if m.ProjectID == "" {
		return m.OwnerID == me.ID || m.Shared // 共享到全组的个人论文，组内成员都能看
	}
	p := db.Project(m.ProjectID)
	return p != nil && canSee(me, p)
}

// canEditMaterial：修改、删除、OCR 等写操作。个人论文只有上传者（或管理员）可以改，共享给全组的也一样。
func canEditMaterial(db *DB, me *Me, m *Material) error {
	if !canUseMaterial(db, me, m) {
		return errNotFound("材料不存在、已删除或无权访问")
	}
	if m.ProjectID == "" && m.OwnerID != me.ID && !me.IsAdmin() {
		return errForbidden("这是其他成员共享的论文，只有上传者可以修改")
	}
	return nil
}

func materialView(db *DB, m *Material) map[string]any {
	st := map[string]string{"ready": "可用", "partial": "部分可用", "failed": "失败", "deleted": "已删除"}[m.Status]
	return map[string]any{"id": m.ID, "title": m.Title, "filename": m.Filename, "ftype": m.Ftype, "version": m.Version,
		"size": m.Size, "uploaded_at": m.UploadedAt, "status": m.Status, "status_text": st, "error": m.Error,
		"parse_note": m.ParseNote, "author": m.Author, "source_date": m.SourceDate, "project_id": m.ProjectID,
		"uploader": db.userName(m.OwnerID), "owner_id": m.OwnerID, "chunk_count": m.ChunkCount, "doi": m.DOI, "source_url": m.SourceURL, "zotero_key": m.ZoteroKey, "shared": m.Shared}
}

func (a *App) hListMaterials(w http.ResponseWriter, r *http.Request, me *Me) error {
	pid := r.URL.Query().Get("project_id")
	scope := r.URL.Query().Get("scope") // 个人库：""=我的；group=全组共享；all=我的 + 全组共享
	out := []map[string]any{}
	var err error
	a.store.View(func(db *DB) {
		if pid != "" {
			p := db.Project(pid)
			if p == nil || !canSee(me, p) {
				err = errNotFound("项目不存在或无权访问")
				return
			}
		}
		for i := len(db.Materials) - 1; i >= 0; i-- {
			m := db.Materials[i]
			if m.DeletedAt != nil || m.ProjectID != pid {
				continue
			}
			if pid == "" {
				mine := m.OwnerID == me.ID
				switch scope {
				case "group":
					if !m.Shared {
						continue
					}
				case "all":
					if !mine && !m.Shared {
						continue
					}
				default:
					if !mine {
						continue
					}
				}
			}
			out = append(out, materialView(db, m))
		}
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hUpload(w http.ResponseWriter, r *http.Request, me *Me) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxFileBytes+4<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return errBad("文件过大（单个文件不超过 50 MB）或上传中断")
	}
	defer r.MultipartForm.RemoveAll()
	f, fh, err := r.FormFile("file")
	if err != nil {
		return errBad("请选择文件")
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	ftype, ok := supportedExt[ext]
	if !ok {
		return errBad("不支持的文件类型 " + ext + "。当前支持文字型 PDF、Markdown（.md）、TXT")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return errBad("读取文件失败")
	}
	if len(data) > maxFileBytes {
		return errBad("文件超过 50 MB")
	}
	if len(data) == 0 {
		return errBad("文件为空")
	}
	pid := r.FormValue("project_id")
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(fh.Filename), filepath.Ext(fh.Filename))
	}

	var pieces []Piece
	var status, errMsg, note string
	if ftype == "pdf" {
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return errBad("文件不是有效的 PDF")
		}
		var pages []PDFPage
		if pj := r.FormValue("pages"); pj != "" {
			if json.Unmarshal([]byte(pj), &pages) != nil {
				return errBad("PDF 解析结果格式错误")
			}
		}
		if len(pages) == 0 {
			status, errMsg = "failed", "浏览器未能读取该 PDF（可能已加密或已损坏）"
		} else {
			pieces, status, errMsg, note = ParsePDFPages(pages)
		}
	} else {
		pieces, status, errMsg = ParseText(data)
	}

	m := &Material{ID: newID(), OwnerID: me.ID, ProjectID: pid, Title: title, Filename: filepath.Base(fh.Filename), Ftype: ftype,
		Size: int64(len(data)), UploadedAt: now(), Status: status, Error: errMsg, ParseNote: note,
		Author: strings.TrimSpace(r.FormValue("author")), SourceDate: strings.TrimSpace(r.FormValue("source_date")),
		DOI: strings.TrimSpace(r.FormValue("doi")), SourceURL: strings.TrimSpace(r.FormValue("source_url")),
		ZoteroKey: clipRunes(r.FormValue("zotero_key"), 80)}
	out, err := a.saveMaterial(me, m, data, pieces)
	if err != nil {
		return err
	}
	if logID := r.FormValue("search_log_id"); logID != "" {
		a.recordAdded(me, logID, LoggedPaper{Title: m.Title, DOI: m.DOI, MaterialID: m.ID, Mode: "全文", At: now()}, pid)
	}
	writeJSON(w, 200, out)
	return nil
}

// saveMaterial 写入原件与片段并登记（含归属、版本、数量上限与项目检查）。失败时清理已写入的文件。
func (a *App) saveMaterial(me *Me, m *Material, data []byte, pieces []Piece) (map[string]any, error) {
	chunks := make([]Chunk, 0, len(pieces))
	for i, p := range pieces {
		chunks = append(chunks, Chunk{ID: m.ID + "-" + itoa(i+1), Seq: i + 1, PageIndex: p.PageIndex, PageLabel: p.PageLabel, ParaIndex: p.ParaIndex, Text: p.Text})
	}
	m.ChunkCount = len(chunks)
	if err := os.WriteFile(a.store.FilePath(m), data, 0o600); err != nil {
		return nil, err
	}
	if err := a.store.SaveChunks(m.ID, chunks); err != nil {
		a.store.DeleteChunksAndFile(m)
		return nil, err
	}
	pid, title := m.ProjectID, m.Title
	var out map[string]any
	err := a.store.Update(func(db *DB) error {
		if pid != "" {
			p := db.Project(pid)
			if p == nil || !canSee(me, p) {
				return errNotFound("项目不存在或无权访问")
			}
			if p.Status == "archived" {
				return errBad("项目已归档，不能再上传资料")
			}
		}
		n, ver := 0, 1
		for _, x := range db.Materials {
			if x.OwnerID == me.ID && x.DeletedAt == nil {
				n++
			}
			if x.ProjectID == pid && x.Title == title && (pid != "" || x.OwnerID == me.ID) && x.Version >= ver {
				ver = x.Version + 1
			}
		}
		if n >= maxMaterialsPerUser {
			return errBad("每人最多保存 500 份资料，请先删除不需要的资料")
		}
		m.Version = ver
		db.Materials = append(db.Materials, m)
		if pid != "" {
			db.Log(pid, me.ID, "上传资料", title+" v"+itoa(ver)+"（"+map[string]string{"ready": "可用", "partial": "部分可用", "failed": "解析失败"}[m.Status]+"）")
		}
		out = materialView(db, m)
		return nil
	})
	if err != nil {
		a.store.DeleteChunksAndFile(m)
		return nil, err
	}
	return out, nil
}

func (a *App) getMaterial(me *Me, id string) (Material, error) {
	var m Material
	var err error
	a.store.View(func(db *DB) {
		x := db.Material(id)
		if !canUseMaterial(db, me, x) {
			err = errNotFound("材料不存在、已删除或无权访问")
			return
		}
		m = *x
	})
	return m, err
}

func (a *App) hGetMaterial(w http.ResponseWriter, r *http.Request, me *Me) error {
	var out map[string]any
	var err error
	a.store.View(func(db *DB) {
		m := db.Material(r.PathValue("id"))
		if !canUseMaterial(db, me, m) {
			err = errNotFound("材料不存在、已删除或无权访问")
			return
		}
		out = materialView(db, m)
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hPatchMaterial(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Author     *string `json:"author"`
		SourceDate *string `json:"source_date"`
		Shared     *bool   `json:"shared"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	var out map[string]any
	err := a.store.Update(func(db *DB) error {
		m := db.Material(r.PathValue("id"))
		if err := canEditMaterial(db, me, m); err != nil {
			return err
		}
		if in.Shared != nil {
			if m.ProjectID != "" {
				return errBad("项目论文本来就对项目成员可见，不需要共享")
			}
			if m.Shared != *in.Shared {
				m.Shared = *in.Shared
			}
		}
		if in.Author != nil {
			m.Author = strings.TrimSpace(*in.Author)
		}
		if in.SourceDate != nil {
			m.SourceDate = strings.TrimSpace(*in.SourceDate)
		}
		out = materialView(db, m)
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hMaterialChunks(w http.ResponseWriter, r *http.Request, me *Me) error {
	m, err := a.getMaterial(me, r.PathValue("id"))
	if err != nil {
		return err
	}
	out := []Hit{}
	for _, c := range a.store.Chunks(m.ID) {
		out = append(out, makeHit(&m, c))
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hMaterialFile(w http.ResponseWriter, r *http.Request, me *Me) error {
	m, err := a.getMaterial(me, r.PathValue("id"))
	if err != nil {
		return err
	}
	f, err := os.Open(a.store.FilePath(&m))
	if err != nil {
		return errNotFound("原文件不存在")
	}
	defer f.Close()
	ct := map[string]string{"pdf": "application/pdf", "md": "text/plain; charset=utf-8", "txt": "text/plain; charset=utf-8"}[m.Ftype]
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(m.Filename))
	io.Copy(w, f)
	return nil
}

func (a *App) hGetChunk(w http.ResponseWriter, r *http.Request, me *Me) error {
	cid := r.PathValue("id")
	i := strings.LastIndex(cid, "-")
	if i < 0 {
		return errNotFound("片段不存在")
	}
	m, err := a.getMaterial(me, cid[:i])
	if err != nil {
		return errNotFound("片段不存在、材料已删除或无权访问")
	}
	for _, c := range a.store.Chunks(m.ID) {
		if c.ID == cid {
			writeJSON(w, 200, makeHit(&m, c))
			return nil
		}
	}
	return errNotFound("片段不存在")
}

// 删除：先标记删除（立即不参与检索），再清理原件与片段；旧回答引用显示“材料已删除”。
func (a *App) hDeleteMaterial(w http.ResponseWriter, r *http.Request, me *Me) error {
	delAnswers := r.URL.Query().Get("delete_answers") == "true"
	var snap Material
	removed := 0
	err := a.store.Update(func(db *DB) error {
		m := db.Material(r.PathValue("id"))
		if err := canEditMaterial(db, me, m); err != nil {
			return err
		}
		if m.ProjectID != "" {
			p := db.Project(m.ProjectID)
			if m.OwnerID != me.ID && !canManage(me, p) {
				return errForbidden("只有上传者或指导老师可以删除项目资料")
			}
			if p.Status == "archived" {
				return errBad("项目已归档，不能删除资料")
			}
			db.Log(p.ID, me.ID, "删除资料", m.Title+" v"+itoa(m.Version))
		}
		t := now()
		m.DeletedAt = &t
		m.Status = "deleted"
		if delAnswers {
			keep := db.Answers[:0]
			for _, an := range db.Answers {
				hit := false
				for _, id := range an.MaterialIDs {
					if id == m.ID {
						hit = true
					}
				}
				if hit && an.OwnerID == me.ID {
					removed++
					continue
				}
				keep = append(keep, an)
			}
			db.Answers = keep
		}
		snap = *m
		return nil
	})
	if err != nil {
		return err
	}
	if err := a.store.DeleteChunksAndFile(&snap); err != nil {
		writeJSON(w, 200, map[string]any{"id": snap.ID, "status": "deleted", "warning": "已禁止检索，但原文件清理失败：" + err.Error()})
		return nil
	}
	writeJSON(w, 200, map[string]any{"id": snap.ID, "status": "deleted", "removed_answers": removed})
	return nil
}
