package main

// 首页总览：一次返回首页需要的数字、待我处理、最近论文和最近草稿，避免首页发多个请求。

import (
	"net/http"
	"sort"
	"time"
)

type homeTodo struct {
	Kind      string `json:"kind"` // review 待审核 / returned 被退回 / due 快到期 / overdue 已逾期
	ProjectID string `json:"project_id"`
	Project   string `json:"project"`
	StageID   string `json:"stage_id"`
	Stage     string `json:"stage"`
	Due       string `json:"due,omitempty"`
	ID        string `json:"id,omitempty"` // 论文契约审阅：契约 ID
	Title     string `json:"title,omitempty"`
	Who       string `json:"who,omitempty"`
}

func (a *App) hHome(w http.ResponseWriter, r *http.Request, me *Me) error {
	stats := map[string]int{"projects": 0, "papers_mine": 0, "papers_shared": 0, "drafts": 0, "pending_reviews": 0}
	todos := []homeTodo{}
	papers := []map[string]any{}
	drafts := []map[string]any{}
	today := time.Now().Format("2006-01-02")
	soon := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	a.store.View(func(db *DB) {
		for _, p := range db.Projects {
			if !canSee(me, p) || p.Status != "active" {
				continue
			}
			stats["projects"]++
			manage := canManage(me, p)
			member := contains(p.Members, me.ID)
			for _, s := range p.Stages {
				t := homeTodo{ProjectID: p.ID, Project: p.Name, StageID: s.ID, Stage: s.Name, Due: s.Due}
				switch {
				case s.Status == "review" && manage:
					stats["pending_reviews"]++
					t.Kind = "review"
				case s.Status == "returned" && member:
					t.Kind = "returned"
				case member && s.Status != "done" && s.Status != "review" && len(s.Due) == 10 && s.Due < today:
					t.Kind = "overdue"
				case member && s.Status != "done" && s.Status != "review" && len(s.Due) == 10 && s.Due <= soon:
					t.Kind = "due"
				default:
					continue
				}
				todos = append(todos, t)
			}
		}
		// 论文契约审阅：请我审阅的（老师）、被老师退回的（作者）
		for _, c := range db.Contracts {
			if c.Reviewer == me.ID && c.ReviewStatus == "requested" && me.IsTeacher() {
				stats["pending_reviews"]++
				todos = append(todos, homeTodo{Kind: "contract_review", ID: c.ID, Title: c.Title, Who: db.userName(c.OwnerID)})
			} else if c.OwnerID == me.ID && c.ReviewStatus == "returned" {
				todos = append(todos, homeTodo{Kind: "contract_returned", ID: c.ID, Title: c.Title, Who: db.userName(c.Reviewer)})
			}
		}
		var ms []*Material
		for _, m := range db.Materials {
			if m.DeletedAt != nil || m.ProjectID != "" {
				continue
			}
			if m.OwnerID == me.ID {
				stats["papers_mine"]++
			} else if m.Shared {
				stats["papers_shared"]++
			} else {
				continue
			}
			ms = append(ms, m)
		}
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].UploadedAt.After(ms[j].UploadedAt) })
		for i, m := range ms {
			if i >= 5 {
				break
			}
			papers = append(papers, materialView(db, m))
		}
		var ds []*WDraft
		for _, d := range db.Drafts {
			if d.OwnerID == me.ID {
				ds = append(ds, d)
			}
		}
		stats["drafts"] = len(ds)
		sort.Slice(ds, func(i, j int) bool { return ds[i].UpdatedAt.After(ds[j].UpdatedAt) })
		for i, d := range ds {
			if i >= 3 {
				break
			}
			name := d.Profile
			if p := profileByKey(d.Profile); p != nil {
				name = p.Name
			}
			drafts = append(drafts, map[string]any{"id": d.ID, "profile": name, "section": d.Section, "idea": clipRunes(d.Idea, 60), "stage": d.Stage, "updated_at": d.UpdatedAt})
		}
	})
	order := map[string]int{"review": 0, "contract_review": 0, "returned": 1, "contract_returned": 1, "overdue": 2, "due": 3}
	sort.SliceStable(todos, func(i, j int) bool { return order[todos[i].Kind] < order[todos[j].Kind] })
	if len(todos) > 12 {
		todos = todos[:12]
	}
	writeJSON(w, 200, map[string]any{"stats": stats, "todos": todos, "recent_papers": papers, "recent_drafts": drafts})
	return nil
}
