package main

import (
	"net/http"
	"sort"
	"strings"
)

// ---- 权限（调用方需持有锁） ----

func canManage(me *Me, p *Project) bool {
	return me.IsAdmin() || contains(p.Advisors, me.ID)
}

func isMember(me *Me, p *Project) bool {
	return contains(p.Members, me.ID) || contains(p.Advisors, me.ID)
}

func canSee(me *Me, p *Project) bool {
	return me.IsAdmin() || isMember(me, p)
}

func (db *DB) userName(id int) string {
	if u := db.User(id); u != nil {
		return u.Name
	}
	return "（已删除用户）"
}

func stageProgress(p *Project) (done int, current int) {
	current = -1
	for i, s := range p.Stages {
		if s.Status == "done" {
			done++
		} else if current < 0 {
			current = i
		}
	}
	return
}

func projectSummary(db *DB, me *Me, p *Project) map[string]any {
	done, cur := stageProgress(p)
	curName := "全部完成"
	if cur >= 0 {
		curName = p.Stages[cur].Name
	}
	pending := 0
	for _, s := range p.Stages {
		if s.Status == "review" {
			pending++
		}
	}
	return map[string]any{"id": p.ID, "name": p.Name, "kind": p.Kind, "desc": p.Desc, "status": p.Status,
		"stage_total": len(p.Stages), "stage_done": done, "current_stage": curName, "pending_reviews": pending,
		"advisors": namesOf(db, p.Advisors), "member_count": len(p.Members), "manage": canManage(me, p),
		"created_at": p.CreatedAt}
}

func namesOf(db *DB, ids []int) []map[string]any {
	out := []map[string]any{}
	for _, id := range ids {
		out = append(out, map[string]any{"id": id, "name": db.userName(id)})
	}
	return out
}

func (a *App) hListProjects(w http.ResponseWriter, r *http.Request, me *Me) error {
	out := []map[string]any{}
	a.store.View(func(db *DB) {
		for _, p := range db.Projects {
			if canSee(me, p) {
				out = append(out, projectSummary(db, me, p))
			}
		}
	})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i]["status"] != out[j]["status"] {
			return out[i]["status"] == "active"
		}
		return false
	})
	writeJSON(w, 200, out)
	return nil
}

func cleanIDs(db *DB, ids []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, id := range ids {
		if db.User(id) != nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (a *App) hCreateProject(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsTeacher() {
		return errForbidden("只有老师可以创建项目")
	}
	var in struct {
		Name          string `json:"name"`
		Desc          string `json:"desc"`
		TemplateKey   string `json:"template_key"`
		FromProjectID string `json:"from_project_id"`
		Members       []int  `json:"members"`
		Advisors      []int  `json:"advisors"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errBad("请填写项目名称")
	}
	var pid string
	err := a.store.Update(func(db *DB) error {
		var stages []*Stage
		kind := in.TemplateKey
		if in.FromProjectID != "" {
			src := db.Project(in.FromProjectID)
			if src == nil || !(canSee(me, src) || (src.Status == "archived" && src.ShareToLibrary)) {
				return errNotFound("参考项目不存在或无权访问")
			}
			kind = src.Kind
			for _, s := range src.Stages {
				st := &Stage{ID: newID(), Name: s.Name, Goal: s.Goal, Guide: s.Guide, Status: "todo", UpdatedAt: now(), Submissions: []Submission{}, Reviews: []Review{}, Checklist: []CheckItem{}}
				for _, c := range s.Checklist {
					st.Checklist = append(st.Checklist, CheckItem{Text: c.Text})
				}
				stages = append(stages, st)
			}
		} else {
			t := findTemplate(db, in.TemplateKey)
			if t == nil {
				return errBad("请选择流程模板")
			}
			stages = stagesFromTemplate(t)
		}
		advisors := cleanIDs(db, append([]int{me.ID}, in.Advisors...))
		for _, id := range advisors {
			if u := db.User(id); u.Role == "student" {
				return errBad("学生不能担任指导老师：" + u.Name)
			}
		}
		p := &Project{ID: newID(), Name: in.Name, Kind: kind, Desc: strings.TrimSpace(in.Desc), CreatedBy: me.ID,
			Advisors: advisors, Members: cleanIDs(db, in.Members), Status: "active", Stages: stages, CreatedAt: now()}
		if p.Members == nil {
			p.Members = []int{}
		}
		db.Projects = append(db.Projects, p)
		db.Log(p.ID, me.ID, "创建项目", p.Name)
		pid = p.ID
		return nil
	})
	if err != nil {
		return err
	}
	r.SetPathValue("id", pid)
	return a.hGetProject(w, r, me)
}

func (a *App) hGetProject(w http.ResponseWriter, r *http.Request, me *Me) error {
	id := r.PathValue("id")
	var out map[string]any
	var err error
	a.store.View(func(db *DB) {
		p := db.Project(id)
		if p == nil || !canSee(me, p) {
			err = errNotFound("项目不存在或无权访问")
			return
		}
		out = projectSummary(db, me, p)
		stages := []map[string]any{}
		for _, s := range p.Stages {
			subs := []map[string]any{}
			for _, sb := range s.Submissions {
				mats := []map[string]any{}
				for _, mid := range sb.MaterialIDs {
					if m := db.Material(mid); m != nil && m.DeletedAt == nil {
						mats = append(mats, map[string]any{"id": m.ID, "title": m.Title, "version": m.Version})
					} else {
						mats = append(mats, map[string]any{"id": mid, "title": "（材料已删除）", "deleted": true})
					}
				}
				subs = append(subs, map[string]any{"id": sb.ID, "user": db.userName(sb.UserID), "content": sb.Content, "materials": mats, "created_at": sb.CreatedAt})
			}
			revs := []map[string]any{}
			for _, rv := range s.Reviews {
				revs = append(revs, map[string]any{"id": rv.ID, "user": db.userName(rv.UserID), "decision": rv.Decision, "comment": rv.Comment, "created_at": rv.CreatedAt})
			}
			checks := []map[string]any{}
			for _, c := range s.Checklist {
				item := map[string]any{"text": c.Text, "done": c.Done}
				if c.Done {
					item["done_by"] = db.userName(c.DoneBy)
				}
				checks = append(checks, item)
			}
			stages = append(stages, map[string]any{"id": s.ID, "name": s.Name, "goal": s.Goal, "guide": s.Guide, "status": s.Status,
				"due": s.Due, "checklist": checks, "submissions": subs, "reviews": revs, "updated_at": s.UpdatedAt})
		}
		out["stages"] = stages
		out["members"] = namesOf(db, p.Members)
		out["member_ids"] = p.Members
		out["advisor_ids"] = p.Advisors
		out["is_member"] = isMember(me, p)
		out["retro"] = p.Retro
		out["share_to_library"] = p.ShareToLibrary
		out["can_delete"] = me.IsAdmin() || p.CreatedBy == me.ID
		out["model_profile_id"] = p.ModelProfileID
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

// withProject 在写锁内取项目并检查权限，need: "see" | "member" | "manage"。已归档项目只读。
func (a *App) withProject(r *http.Request, me *Me, need string, fn func(db *DB, p *Project) error) error {
	id := r.PathValue("id")
	return a.store.Update(func(db *DB) error {
		p := db.Project(id)
		if p == nil || !canSee(me, p) {
			return errNotFound("项目不存在或无权访问")
		}
		switch need {
		case "manage":
			if !canManage(me, p) {
				return errForbidden("只有指导老师可以执行此操作")
			}
		case "member":
			if !isMember(me, p) && !canManage(me, p) {
				return errForbidden("只有项目成员可以执行此操作")
			}
		}
		if p.Status == "archived" && need != "unarchive" {
			return errBad("项目已归档，如需修改请先取消归档")
		}
		return fn(db, p)
	})
}

func findStage(p *Project, sid string) (int, *Stage) {
	for i, s := range p.Stages {
		if s.ID == sid {
			return i, s
		}
	}
	return -1, nil
}

func (a *App) hUpdateProject(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Name           *string `json:"name"`
		Desc           *string `json:"desc"`
		Members        []int   `json:"members"`
		Advisors       []int   `json:"advisors"`
		ModelProfileID *string `json:"model_profile_id"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.withProject(r, me, "manage", func(db *DB, p *Project) error {
		if in.ModelProfileID != nil {
			id := *in.ModelProfileID
			label := "不指定（成员各自的模型 / 团队默认）"
			if id == "team" {
				label = "团队默认模型"
			} else if id != "" {
				mp := db.modelProfile(id)
				if mp == nil || mp.OwnerID != me.ID {
					return errBad("只能指定你自己的模型配置卡")
				}
				if !passed(mp) {
					return errBad("该模型配置卡还没有通过兼容性自检")
				}
				label = mp.Name + "（" + mp.Model + "，由" + db.userName(me.ID) + "提供）"
			}
			p.ModelProfileID = id
			db.Log(p.ID, me.ID, "设置项目模型", label)
		}
		if in.Name != nil {
			if strings.TrimSpace(*in.Name) == "" {
				return errBad("项目名称不能为空")
			}
			p.Name = strings.TrimSpace(*in.Name)
		}
		if in.Desc != nil {
			p.Desc = strings.TrimSpace(*in.Desc)
		}
		if in.Members != nil {
			p.Members = cleanIDs(db, in.Members)
			if p.Members == nil {
				p.Members = []int{}
			}
			db.Log(p.ID, me.ID, "调整成员", strings.Join(namesStr(db, p.Members), "、"))
		}
		if in.Advisors != nil {
			adv := cleanIDs(db, in.Advisors)
			if len(adv) == 0 {
				return errBad("至少保留一位指导老师")
			}
			for _, id := range adv {
				if db.User(id).Role == "student" {
					return errBad("学生不能担任指导老师")
				}
			}
			p.Advisors = adv
			db.Log(p.ID, me.ID, "调整指导老师", strings.Join(namesStr(db, adv), "、"))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func namesStr(db *DB, ids []int) []string {
	var out []string
	for _, id := range ids {
		out = append(out, db.userName(id))
	}
	return out
}

func (a *App) hAddStage(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Name      string   `json:"name"`
		Goal      string   `json:"goal"`
		Guide     string   `json:"guide"`
		Checklist []string `json:"checklist"`
		After     int      `json:"after"` // 插入到第几个阶段之后；-1 表示最前
	}
	in.After = 1 << 30
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return errBad("请填写阶段名称")
	}
	err := a.withProject(r, me, "manage", func(db *DB, p *Project) error {
		st := &Stage{ID: newID(), Name: strings.TrimSpace(in.Name), Goal: in.Goal, Guide: in.Guide, Status: "todo", UpdatedAt: now(), Submissions: []Submission{}, Reviews: []Review{}, Checklist: []CheckItem{}}
		for _, c := range in.Checklist {
			if strings.TrimSpace(c) != "" {
				st.Checklist = append(st.Checklist, CheckItem{Text: strings.TrimSpace(c)})
			}
		}
		pos := in.After + 1
		if pos < 0 {
			pos = 0
		}
		if pos > len(p.Stages) {
			pos = len(p.Stages)
		}
		p.Stages = append(p.Stages[:pos], append([]*Stage{st}, p.Stages[pos:]...)...)
		db.Log(p.ID, me.ID, "新增阶段", st.Name)
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hEditStage(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Name      *string  `json:"name"`
		Goal      *string  `json:"goal"`
		Guide     *string  `json:"guide"`
		Due       *string  `json:"due"`
		Checklist []string `json:"checklist"` // 提供时整体替换清单文字（保留同名项的勾选状态）
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.withProject(r, me, "manage", func(db *DB, p *Project) error {
		_, s := findStage(p, r.PathValue("sid"))
		if s == nil {
			return errNotFound("阶段不存在")
		}
		if in.Name != nil {
			if strings.TrimSpace(*in.Name) == "" {
				return errBad("阶段名称不能为空")
			}
			s.Name = strings.TrimSpace(*in.Name)
		}
		if in.Goal != nil {
			s.Goal = *in.Goal
		}
		if in.Guide != nil {
			s.Guide = *in.Guide
		}
		if in.Due != nil {
			s.Due = strings.TrimSpace(*in.Due)
		}
		if in.Checklist != nil {
			old := map[string]CheckItem{}
			for _, c := range s.Checklist {
				old[c.Text] = c
			}
			s.Checklist = []CheckItem{}
			for _, t := range in.Checklist {
				t = strings.TrimSpace(t)
				if t == "" {
					continue
				}
				if c, ok := old[t]; ok {
					s.Checklist = append(s.Checklist, c)
				} else {
					s.Checklist = append(s.Checklist, CheckItem{Text: t})
				}
			}
		}
		s.UpdatedAt = now()
		db.Log(p.ID, me.ID, "修改阶段", s.Name)
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hDeleteStage(w http.ResponseWriter, r *http.Request, me *Me) error {
	err := a.withProject(r, me, "manage", func(db *DB, p *Project) error {
		i, s := findStage(p, r.PathValue("sid"))
		if s == nil {
			return errNotFound("阶段不存在")
		}
		if len(s.Submissions) > 0 {
			return errBad("该阶段已有提交记录，为保留过程记录不能删除")
		}
		p.Stages = append(p.Stages[:i], p.Stages[i+1:]...)
		db.Log(p.ID, me.ID, "删除阶段", s.Name)
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hMoveStage(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Dir int `json:"dir"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.withProject(r, me, "manage", func(db *DB, p *Project) error {
		i, s := findStage(p, r.PathValue("sid"))
		if s == nil {
			return errNotFound("阶段不存在")
		}
		j := i + in.Dir
		if in.Dir != 1 && in.Dir != -1 || j < 0 || j >= len(p.Stages) {
			return errBad("无法移动")
		}
		p.Stages[i], p.Stages[j] = p.Stages[j], p.Stages[i]
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hCheckItem(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Index int  `json:"index"`
		Done  bool `json:"done"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.withProject(r, me, "member", func(db *DB, p *Project) error {
		_, s := findStage(p, r.PathValue("sid"))
		if s == nil {
			return errNotFound("阶段不存在")
		}
		if in.Index < 0 || in.Index >= len(s.Checklist) {
			return errBad("清单项不存在")
		}
		c := &s.Checklist[in.Index]
		c.Done = in.Done
		if in.Done {
			t := now()
			c.DoneBy, c.DoneAt = me.ID, &t
		} else {
			c.DoneBy, c.DoneAt = 0, nil
		}
		if s.Status == "todo" && in.Done {
			s.Status = "doing"
		}
		s.UpdatedAt = now()
		verb := "勾选"
		if !in.Done {
			verb = "取消勾选"
		}
		db.Log(p.ID, me.ID, verb, s.Name+"：“"+c.Text+"”")
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hStartStage(w http.ResponseWriter, r *http.Request, me *Me) error {
	err := a.withProject(r, me, "member", func(db *DB, p *Project) error {
		_, s := findStage(p, r.PathValue("sid"))
		if s == nil {
			return errNotFound("阶段不存在")
		}
		if s.Status == "done" {
			return errBad("该阶段已通过")
		}
		s.Status = "doing"
		s.UpdatedAt = now()
		db.Log(p.ID, me.ID, "开始阶段", s.Name)
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hSubmit(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Content       string   `json:"content"`
		MaterialIDs   []string `json:"material_ids"`
		RequestReview bool     `json:"request_review"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" && len(in.MaterialIDs) == 0 {
		return errBad("请填写提交内容或关联材料")
	}
	err := a.withProject(r, me, "member", func(db *DB, p *Project) error {
		_, s := findStage(p, r.PathValue("sid"))
		if s == nil {
			return errNotFound("阶段不存在")
		}
		if s.Status == "done" {
			return errBad("该阶段已通过，如需修改请联系指导老师")
		}
		// 关联材料必须属于本项目
		for _, mid := range in.MaterialIDs {
			m := db.Material(mid)
			if m == nil || m.DeletedAt != nil || m.ProjectID != p.ID {
				return errBad("只能关联本项目资料库中的材料")
			}
		}
		s.Submissions = append(s.Submissions, Submission{ID: newID(), UserID: me.ID, Content: in.Content, MaterialIDs: in.MaterialIDs, CreatedAt: now()})
		if in.RequestReview {
			s.Status = "review"
			db.Log(p.ID, me.ID, "提交审核", s.Name)
		} else {
			if s.Status == "todo" || s.Status == "returned" {
				s.Status = "doing"
			}
			db.Log(p.ID, me.ID, "提交进展", s.Name)
		}
		s.UpdatedAt = now()
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hReview(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.Decision != "approve" && in.Decision != "return" && in.Decision != "comment" {
		return errBad("审核结果无效")
	}
	in.Comment = strings.TrimSpace(in.Comment)
	if in.Decision != "approve" && in.Comment == "" {
		return errBad("退回或评论时请填写意见")
	}
	err := a.withProject(r, me, "manage", func(db *DB, p *Project) error {
		_, s := findStage(p, r.PathValue("sid"))
		if s == nil {
			return errNotFound("阶段不存在")
		}
		s.Reviews = append(s.Reviews, Review{ID: newID(), UserID: me.ID, Decision: in.Decision, Comment: in.Comment, CreatedAt: now()})
		switch in.Decision {
		case "approve":
			s.Status = "done"
			db.Log(p.ID, me.ID, "审核通过", s.Name)
		case "return":
			s.Status = "returned"
			db.Log(p.ID, me.ID, "退回修改", s.Name+"："+in.Comment)
		default:
			db.Log(p.ID, me.ID, "评论", s.Name+"："+in.Comment)
		}
		s.UpdatedAt = now()
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hActivity(w http.ResponseWriter, r *http.Request, me *Me) error {
	id := r.PathValue("id")
	out := []map[string]any{}
	var err error
	a.store.View(func(db *DB) {
		p := db.Project(id)
		if p == nil || !canSee(me, p) {
			err = errNotFound("项目不存在或无权访问")
			return
		}
		for i := len(db.Activities) - 1; i >= 0 && len(out) < 300; i-- {
			ac := db.Activities[i]
			if ac.ProjectID == id {
				out = append(out, map[string]any{"user": db.userName(ac.UserID), "action": ac.Action, "detail": ac.Detail, "at": ac.At})
			}
		}
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hArchive(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Good     string `json:"good"`
		Pitfalls string `json:"pitfalls"`
		Advice   string `json:"advice"`
		Share    bool   `json:"share"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.withProject(r, me, "manage", func(db *DB, p *Project) error {
		p.Retro = &Retro{Good: strings.TrimSpace(in.Good), Pitfalls: strings.TrimSpace(in.Pitfalls), Advice: strings.TrimSpace(in.Advice)}
		p.ShareToLibrary = in.Share
		p.Status = "archived"
		t := now()
		p.ArchivedAt = &t
		msg := "未共享到经验库"
		if in.Share {
			msg = "已共享到经验库"
		}
		db.Log(p.ID, me.ID, "归档项目", msg)
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hUnarchive(w http.ResponseWriter, r *http.Request, me *Me) error {
	err := a.withProject(r, me, "unarchive", func(db *DB, p *Project) error {
		if !canManage(me, p) {
			return errForbidden("只有指导老师可以取消归档")
		}
		p.Status = "active"
		p.ArchivedAt = nil
		db.Log(p.ID, me.ID, "取消归档", "")
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetProject(w, r, me)
}

func (a *App) hDeleteProject(w http.ResponseWriter, r *http.Request, me *Me) error {
	id := r.PathValue("id")
	var mats []*Material
	err := a.store.Update(func(db *DB) error {
		p := db.Project(id)
		if p == nil || !canSee(me, p) {
			return errNotFound("项目不存在或无权访问")
		}
		if !me.IsAdmin() && p.CreatedBy != me.ID {
			return errForbidden("只有创建者或管理员可以删除项目")
		}
		t := now()
		for _, m := range db.Materials {
			if m.ProjectID == id && m.DeletedAt == nil {
				m.DeletedAt = &t
				m.Status = "deleted"
				c := *m
				mats = append(mats, &c)
			}
		}
		keepA := db.Answers[:0]
		for _, an := range db.Answers {
			if an.ProjectID != id {
				keepA = append(keepA, an)
			}
		}
		db.Answers = keepA
		keepC := db.CiteChecks[:0]
		for _, c := range db.CiteChecks {
			if c.ProjectID != id {
				keepC = append(keepC, c)
			}
		}
		db.CiteChecks = keepC
		keepP := db.Projects[:0]
		for _, x := range db.Projects {
			if x.ID != id {
				keepP = append(keepP, x)
			}
		}
		db.Projects = keepP
		keepAct := db.Activities[:0]
		for _, ac := range db.Activities {
			if ac.ProjectID != id {
				keepAct = append(keepAct, ac)
			}
		}
		db.Activities = keepAct
		return nil
	})
	if err != nil {
		return err
	}
	for _, m := range mats {
		a.store.DeleteChunksAndFile(m)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

// ---- 模板 ----

func (a *App) hListTemplates(w http.ResponseWriter, r *http.Request, me *Me) error {
	var out []map[string]any
	a.store.View(func(db *DB) {
		for _, t := range append(append([]*Template{}, builtinTemplates...), db.Templates...) {
			var names []string
			for _, s := range t.Stages {
				names = append(names, s.Name)
			}
			out = append(out, map[string]any{"key": t.Key, "name": t.Name, "desc": t.Desc, "built_in": t.BuiltIn,
				"stages": names, "can_delete": !t.BuiltIn && (me.IsAdmin() || t.CreatedBy == me.ID)})
		}
	})
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hCreateTemplate(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsTeacher() {
		return errForbidden("只有老师可以保存模板")
	}
	var in struct {
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
		Desc      string `json:"desc"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return errBad("请填写模板名称")
	}
	err := a.store.Update(func(db *DB) error {
		p := db.Project(in.ProjectID)
		if p == nil || !(canSee(me, p) || (p.Status == "archived" && p.ShareToLibrary)) {
			return errNotFound("项目不存在或无权访问")
		}
		t := &Template{Key: "c_" + newID(), Name: strings.TrimSpace(in.Name), Desc: strings.TrimSpace(in.Desc), CreatedBy: me.ID}
		for _, s := range p.Stages {
			ts := TemplateStage{Name: s.Name, Goal: s.Goal, Guide: s.Guide}
			for _, c := range s.Checklist {
				ts.Checklist = append(ts.Checklist, c.Text)
			}
			t.Stages = append(t.Stages, ts)
		}
		db.Templates = append(db.Templates, t)
		return nil
	})
	if err != nil {
		return err
	}
	return a.hListTemplates(w, r, me)
}

func (a *App) hDeleteTemplate(w http.ResponseWriter, r *http.Request, me *Me) error {
	key := r.PathValue("key")
	err := a.store.Update(func(db *DB) error {
		for i, t := range db.Templates {
			if t.Key == key {
				if !me.IsAdmin() && t.CreatedBy != me.ID {
					return errForbidden("只有创建者或管理员可以删除模板")
				}
				db.Templates = append(db.Templates[:i], db.Templates[i+1:]...)
				return nil
			}
		}
		return errNotFound("模板不存在（内置模板不能删除）")
	})
	if err != nil {
		return err
	}
	return a.hListTemplates(w, r, me)
}

// ---- 经验库：已归档且共享的项目，全组只读可见（不含资料原文） ----

func (a *App) hLibrary(w http.ResponseWriter, r *http.Request, me *Me) error {
	out := []map[string]any{}
	a.store.View(func(db *DB) {
		for i := len(db.Projects) - 1; i >= 0; i-- {
			p := db.Projects[i]
			if p.Status == "archived" && p.ShareToLibrary {
				s := projectSummary(db, me, p)
				s["archived_at"] = p.ArchivedAt
				s["retro"] = p.Retro
				out = append(out, s)
			}
		}
	})
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hLibraryItem(w http.ResponseWriter, r *http.Request, me *Me) error {
	id := r.PathValue("id")
	var out map[string]any
	var err error
	a.store.View(func(db *DB) {
		p := db.Project(id)
		if p == nil || p.Status != "archived" || !p.ShareToLibrary {
			err = errNotFound("经验库中没有该项目")
			return
		}
		out = projectSummary(db, me, p)
		out["retro"] = p.Retro
		out["archived_at"] = p.ArchivedAt
		out["members"] = namesOf(db, p.Members)
		stages := []map[string]any{}
		for _, s := range p.Stages {
			subs := []map[string]any{}
			for _, sb := range s.Submissions {
				subs = append(subs, map[string]any{"user": db.userName(sb.UserID), "content": sb.Content, "created_at": sb.CreatedAt, "material_count": len(sb.MaterialIDs)})
			}
			revs := []map[string]any{}
			for _, rv := range s.Reviews {
				revs = append(revs, map[string]any{"user": db.userName(rv.UserID), "decision": rv.Decision, "comment": rv.Comment, "created_at": rv.CreatedAt})
			}
			var checks []string
			for _, c := range s.Checklist {
				checks = append(checks, c.Text)
			}
			stages = append(stages, map[string]any{"name": s.Name, "goal": s.Goal, "guide": s.Guide, "status": s.Status, "checklist": checks, "submissions": subs, "reviews": revs})
		}
		out["stages"] = stages
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}
