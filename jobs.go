package main

// 后台任务：耗时的 AI 操作（AI 起草、列提纲、格式检查、问答、速读……）可以放到后台运行。
// 前端带上请求头 X-KY-Async: 1 时，接口立即返回任务编号，处理在后台继续进行：
// 关掉页面、切到别的页面都不影响；同一个人可以同时跑多个任务（每人最多 3 个同时运行，其余排队）。
// 结果保存在内存里（工作台关闭后清空），页面右上角的“后台任务”里可以查看、跳转到结果。

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type bgJob struct {
	ID       string    `json:"id"`
	OwnerID  int       `json:"-"`
	Label    string    `json:"label"`
	Title    string    `json:"title"`
	Link     string    `json:"link"`
	Status   string    `json:"status"` // queued / running / done / error
	Error    string    `json:"error,omitempty"`
	Created  time.Time `json:"created"`
	Started  time.Time `json:"started,omitempty"`
	Finished time.Time `json:"finished,omitempty"`
	code     int
	body     []byte
	seen     bool
}

const (
	jobsPerUser = 3  // 每人同时运行
	jobsGlobal  = 8  // 全部同时运行
	jobsKeep    = 40 // 每人保留最近的任务数
)

var (
	jobsMu   sync.Mutex
	jobsByID = map[string]*bgJob{}
	jobsSem  = make(chan struct{}, jobsGlobal)
	jobsUser = map[int]chan struct{}{}
)

type recorder struct {
	h    http.Header
	code int
	buf  bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.h }
func (r *recorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = 200
	}
	return r.buf.Write(b)
}
func (r *recorder) WriteHeader(c int) {
	if r.code == 0 {
		r.code = c
	}
}

func userSlot(uid int) chan struct{} {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	c := jobsUser[uid]
	if c == nil {
		c = make(chan struct{}, jobsPerUser)
		jobsUser[uid] = c
	}
	return c
}

func hdrText(r *http.Request, k string, max int) string {
	v, _ := url.QueryUnescape(r.Header.Get(k))
	return clipRunes(strings.TrimSpace(v), max)
}

// bg 把一个接口变成“可以放到后台”的接口。
func (a *App) bg(label string, h handler) handler {
	return func(w http.ResponseWriter, r *http.Request, me *Me) error {
		if r.Header.Get("X-KY-Async") != "1" {
			return h(w, r, me)
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 48<<20))
		if err != nil {
			return errBad("读取请求失败")
		}
		j := &bgJob{ID: newID(), OwnerID: me.ID, Label: label, Title: hdrText(r, "X-KY-Title", 80), Link: hdrText(r, "X-KY-Link", 200), Status: "queued", Created: now()}
		if !strings.HasPrefix(j.Link, "#/") {
			j.Link = ""
		}
		jobID := j.ID
		jobsMu.Lock()
		jobsByID[j.ID] = j
		pruneJobsLocked(me.ID)
		jobsMu.Unlock()
		// 请求结束后原请求的上下文会被取消，这里换成独立的上下文
		r2 := r.Clone(context.Background())
		r2.Body = io.NopCloser(bytes.NewReader(body))
		r2.ContentLength = int64(len(body))
		meCopy := *me
		go func() {
			slot := userSlot(meCopy.ID)
			slot <- struct{}{}
			jobsSem <- struct{}{}
			defer func() { <-jobsSem; <-slot }()
			jobsMu.Lock()
			j.Status, j.Started = "running", now()
			jobsMu.Unlock()
			rec := &recorder{h: http.Header{}}
			func() {
				defer func() {
					if p := recover(); p != nil {
						writeJSON(rec, 500, map[string]string{"detail": "处理出错"})
					}
				}()
				if err := h(rec, r2, &meCopy); err != nil {
					writeErr(rec, err)
				}
			}()
			jobsMu.Lock()
			defer jobsMu.Unlock()
			j.code, j.body, j.Finished = rec.code, rec.buf.Bytes(), now()
			if j.code == 0 {
				j.code = 200
			}
			if j.code >= 400 {
				j.Status = "error"
				var m map[string]any
				if json.Unmarshal(j.body, &m) == nil {
					j.Error = str(m["detail"])
				}
				if j.Error == "" {
					j.Error = "处理失败（" + itoa(j.code) + "）"
				}
			} else {
				j.Status = "done"
			}
		}()
		writeJSON(w, 202, map[string]any{"job_id": jobID, "status": "queued"})
		return nil
	}
}

// pruneJobsLocked 每人只保留最近的任务，超过 24 小时的已完成任务删除
func pruneJobsLocked(uid int) {
	var mine []*bgJob
	for id, j := range jobsByID {
		if j.Status != "queued" && j.Status != "running" && time.Since(j.Finished) > 24*time.Hour {
			delete(jobsByID, id)
			continue
		}
		if j.OwnerID == uid {
			mine = append(mine, j)
		}
	}
	sort.Slice(mine, func(i, k int) bool { return mine[i].Created.After(mine[k].Created) })
	for i, j := range mine {
		if i >= jobsKeep && j.Status != "queued" && j.Status != "running" {
			delete(jobsByID, j.ID)
		}
	}
}

func (a *App) hJobs(w http.ResponseWriter, r *http.Request, me *Me) error {
	jobsMu.Lock()
	out := []bgJob{}
	for _, j := range jobsByID {
		if j.OwnerID == me.ID {
			out = append(out, *j)
		}
	}
	jobsMu.Unlock()
	sort.Slice(out, func(i, k int) bool { return out[i].Created.After(out[k].Created) })
	running := 0
	for _, j := range out {
		if j.Status == "queued" || j.Status == "running" {
			running++
		}
	}
	writeJSON(w, 200, map[string]any{"jobs": out, "running": running})
	return nil
}

// hJob 查询单个任务；完成后 result 是接口原本的返回内容
func (a *App) hJob(w http.ResponseWriter, r *http.Request, me *Me) error {
	jobsMu.Lock()
	j := jobsByID[r.PathValue("id")]
	var cp bgJob
	if j != nil && j.OwnerID == me.ID {
		cp = *j
		if j.Status == "done" || j.Status == "error" {
			j.seen = true
		}
	}
	jobsMu.Unlock()
	if j == nil || cp.OwnerID != me.ID {
		return errNotFound("任务不存在（工作台重启后，之前的后台任务会清空）")
	}
	out := map[string]any{"job": cp}
	if cp.Status == "done" && len(cp.body) > 0 {
		out["result"] = json.RawMessage(cp.body)
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hJobDelete(w http.ResponseWriter, r *http.Request, me *Me) error {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	if j := jobsByID[r.PathValue("id")]; j != nil && j.OwnerID == me.ID && j.Status != "running" && j.Status != "queued" {
		delete(jobsByID, j.ID)
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	return nil
}
