package main

// 用量与花费：记录每次模型调用的 token、耗时、是否成功、估算费用；按天、人、模型、任务汇总；
// 管理员可给团队模型设每人每月额度；并根据实际数据给出“哪个任务用哪个模型更划算”的建议。

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

type UsageRow struct {
	Day     string  `json:"day"` // 2026-09-27
	UserID  int     `json:"user_id"`
	Model   string  `json:"model"`  // 显示名
	Source  string  `json:"source"` // personal / project / team
	Tier    string  `json:"tier"`   // daily / strong
	Task    string  `json:"task"`
	Calls   int     `json:"calls"`
	Fails   int     `json:"fails"`   // 调用失败（网络、状态码）
	Invalid int     `json:"invalid"` // 返回了内容但没通过校验（格式、引用）
	In      int64   `json:"in"`
	Out     int64   `json:"out"`
	Ms      int64   `json:"ms"`
	Cost    float64 `json:"cost"`
	Est     bool    `json:"est,omitempty"`
}

type UsageCall struct {
	At     time.Time `json:"at"`
	UserID int       `json:"user_id"`
	Model  string    `json:"model"`
	Tier   string    `json:"tier"`
	Task   string    `json:"task"`
	In     int       `json:"in"`
	Out    int       `json:"out"`
	Ms     int       `json:"ms"`
	OK     bool      `json:"ok"`
	Cost   float64   `json:"cost"`
	Route  string    `json:"route,omitempty"`
	Err    string    `json:"err,omitempty"`
}

var taskNames = map[string]string{
	"ask": "资料问答", "compare": "资料对比", "citecheck": "引用核验", "keywords": "拆检索词", "brief": "文献速读",
	"concept": "概念讲解", "sketch": "手绘转图", "ocr": "扫描件识别", "screen": "文献筛选", "research": "调研报告", "writing_plan": "写作骨架", "writing_draft": "AI 起草", "contract": "论文契约", "attack": "审稿人攻击", "rebut": "判定回应", "drift": "偏离检查", "outline": "列提纲", "format": "格式检查", "agent": "本机智能体", "vision": "图片识别", "selfcheck": "兼容性自检", "test": "测试连接",
}

func usageModelName(c ModelCfg) string {
	n := c.Name
	if n == "" {
		n = c.Model
	}
	if !strings.Contains(n, c.Model) {
		n += "（" + c.Model + "）"
	}
	switch c.Source {
	case "team":
		return "团队 · " + n
	default:
		if c.Owner != "" {
			return c.Owner + " · " + n
		}
	}
	return n
}

// recordUsage 由模型调用在结束时回调。
func (a *App) recordUsage(c ModelCfg, u Usage) {
	if c.UserID == 0 {
		return
	}
	day := time.Now().Format("2006-01-02")
	name := usageModelName(c)
	task := c.Task
	if task == "" {
		task = "other"
	}
	tier := c.Tier
	if tier == "" {
		tier = "daily"
	}
	a.store.Update(func(db *DB) error {
		if u.Invalid {
			// 标记最近一次同类调用“未通过校验”
			for i := len(db.Usage) - 1; i >= 0; i-- {
				r := db.Usage[i]
				if r.Day == day && r.UserID == c.UserID && r.Model == name && r.Task == task && r.Tier == tier {
					r.Invalid++
					break
				}
			}
			for i := len(db.UsageCalls) - 1; i >= 0; i-- {
				x := db.UsageCalls[i]
				if x.UserID == c.UserID && x.Model == name && x.Task == task {
					x.OK = false
					if x.Err == "" {
						x.Err = u.Err
					}
					break
				}
			}
			return nil
		}
		cost := c.cost(u.In, u.Out)
		var row *UsageRow
		for i := len(db.Usage) - 1; i >= 0 && i >= len(db.Usage)-400; i-- {
			r := db.Usage[i]
			if r.Day == day && r.UserID == c.UserID && r.Model == name && r.Task == task && r.Tier == tier && r.Source == c.Source {
				row = r
				break
			}
		}
		if row == nil {
			row = &UsageRow{Day: day, UserID: c.UserID, Model: name, Source: c.Source, Tier: tier, Task: task}
			db.Usage = append(db.Usage, row)
		}
		row.Calls++
		if !u.OK {
			row.Fails++
		}
		row.In += int64(u.In)
		row.Out += int64(u.Out)
		row.Ms += int64(u.Ms)
		row.Cost += cost
		row.Est = row.Est || u.Estimated
		db.UsageCalls = append(db.UsageCalls, &UsageCall{At: now(), UserID: c.UserID, Model: name, Tier: tier, Task: task, In: u.In, Out: u.Out,
			Ms: u.Ms, OK: u.OK, Cost: cost, Route: c.Route, Err: u.Err})
		if len(db.UsageCalls) > 500 {
			db.UsageCalls = db.UsageCalls[len(db.UsageCalls)-500:]
		}
		// 只保留约 13 个月的汇总
		if len(db.Usage) > 20000 {
			cut := time.Now().AddDate(0, -13, 0).Format("2006-01-02")
			keep := db.Usage[:0]
			for _, r := range db.Usage {
				if r.Day >= cut {
					keep = append(keep, r)
				}
			}
			db.Usage = keep
		}
		return nil
	})
}

// markInvalid 记录“模型返回了内容，但没通过校验”。
func markInvalid(c ModelCfg, why string) {
	if c.rec != nil {
		c.rec(c, Usage{Invalid: true, Err: why})
	}
}

func monthOf(t time.Time) string { return t.Format("2006-01") }

// teamSpent 返回某人本月使用团队模型的花费（不含自检和测试连接；调用方持有读锁）。
func teamSpent(db *DB, uid int, month string) float64 {
	s := 0.0
	for _, r := range db.Usage {
		if r.UserID == uid && r.Source == "team" && strings.HasPrefix(r.Day, month) && r.Task != "selfcheck" && r.Task != "test" {
			s += r.Cost
		}
	}
	return s
}

// ---------------- 接口 ----------------

type agg struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Calls   int     `json:"calls"`
	Fails   int     `json:"fails"`
	Invalid int     `json:"invalid"`
	In      int64   `json:"in"`
	Out     int64   `json:"out"`
	Ms      int64   `json:"ms"`
	Cost    float64 `json:"cost"`
	Est     bool    `json:"est"`
}

func (g *agg) add(r *UsageRow) {
	g.Calls += r.Calls
	g.Fails += r.Fails
	g.Invalid += r.Invalid
	g.In += r.In
	g.Out += r.Out
	g.Ms += r.Ms
	g.Cost += r.Cost
	g.Est = g.Est || r.Est
}

func (g *agg) okRate() float64 {
	if g.Calls == 0 {
		return 0
	}
	return float64(g.Calls-g.Fails-g.Invalid) / float64(g.Calls)
}

func sortedAggs(m map[string]*agg) []*agg {
	out := make([]*agg, 0, len(m))
	for _, g := range m {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Calls > out[j].Calls
	})
	return out
}

func (a *App) hUsage(w http.ResponseWriter, r *http.Request, me *Me) error {
	q := r.URL.Query()
	team := q.Get("scope") == "team"
	if team && !me.IsAdmin() {
		return errForbidden("只有管理员可以查看全组用量")
	}
	month := q.Get("month")
	if len(month) != 7 {
		month = monthOf(time.Now())
	}
	total := &agg{}
	byModel, byTask, byUser, byTaskTier := map[string]*agg{}, map[string]*agg{}, map[string]*agg{}, map[string]*agg{}
	daily := map[string]*agg{}
	var recent []map[string]any
	var budget, myTeam float64
	names := map[int]string{}
	a.store.View(func(db *DB) {
		budget = db.Settings.TeamBudget
		myTeam = teamSpent(db, me.ID, month)
		for _, u := range db.Users {
			names[u.ID] = u.Name
		}
		get := func(m map[string]*agg, k, label string) *agg {
			if m[k] == nil {
				m[k] = &agg{Key: k, Label: label}
			}
			return m[k]
		}
		for _, row := range db.Usage {
			if !strings.HasPrefix(row.Day, month) || (!team && row.UserID != me.ID) {
				continue
			}
			total.add(row)
			get(byModel, row.Model+"|"+row.Tier, row.Model+map[string]string{"strong": "（难题模型）"}[row.Tier]).add(row)
			tn := taskNames[row.Task]
			if tn == "" {
				tn = row.Task
			}
			get(byTask, row.Task, tn).add(row)
			get(byTaskTier, row.Task+"|"+row.Tier, tn+" · "+map[string]string{"strong": "难题模型", "daily": "日常模型"}[row.Tier]).add(row)
			get(daily, row.Day, row.Day).add(row)
			if team {
				get(byUser, itoa(row.UserID), names[row.UserID]).add(row)
			}
		}
		for i := len(db.UsageCalls) - 1; i >= 0 && len(recent) < 60; i-- {
			c := db.UsageCalls[i]
			if !team && c.UserID != me.ID {
				continue
			}
			tn := taskNames[c.Task]
			if tn == "" {
				tn = c.Task
			}
			recent = append(recent, map[string]any{"at": c.At, "user": names[c.UserID], "model": c.Model, "tier": c.Tier, "task": tn, "in": c.In, "out": c.Out,
				"ms": c.Ms, "ok": c.OK, "cost": c.Cost, "route": c.Route, "err": c.Err})
		}
	})
	days := []*agg{}
	for _, g := range daily {
		days = append(days, g)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Key < days[j].Key })
	if recent == nil {
		recent = []map[string]any{}
	}
	// 按当前速度估算整月
	projection := 0.0
	if month == monthOf(time.Now()) && total.Cost > 0 {
		n := time.Now()
		dim := time.Date(n.Year(), n.Month()+1, 0, 0, 0, 0, 0, n.Location()).Day()
		projection = total.Cost / float64(n.Day()) * float64(dim)
	}
	out := map[string]any{"month": month, "total": total, "by_model": sortedAggs(byModel), "by_task": sortedAggs(byTask), "daily": days,
		"recent": recent, "budget": budget, "my_team_cost": myTeam, "projection": projection, "insights": usageInsights(byTaskTier, byTask, total)}
	if team {
		out["by_user"] = sortedAggs(byUser)
	}
	writeJSON(w, 200, out)
	return nil
}

// usageInsights 根据实际数据给出建议（“优化测量”）。
func usageInsights(byTaskTier, byTask map[string]*agg, total *agg) []string {
	var tips []string
	f2 := func(x float64) string {
		if x < 0.01 && x > 0 {
			return "不到 0.01"
		}
		return strings.TrimRight(strings.TrimRight(ftoa(x, 2), "0"), ".")
	}
	for task, g := range byTask {
		name := taskNames[task]
		if name == "" || task == "selfcheck" || task == "test" {
			continue
		}
		d, s := byTaskTier[task+"|daily"], byTaskTier[task+"|strong"]
		if d != nil && d.Calls >= 5 && d.okRate() >= 0.95 && s != nil && s.Calls >= 3 && d.Calls > 0 && s.Calls > 0 {
			dc, sc := d.Cost/float64(d.Calls), s.Cost/float64(s.Calls)
			if sc > dc*1.5 {
				tips = append(tips, name+"：日常模型成功率 "+pct(d.okRate())+"，每次约 ¥"+f2(dc)+"；难题模型每次约 ¥"+f2(sc)+"。这类任务一般选“自动”或“快速”就够了。")
			}
		}
		if d != nil && d.Calls >= 5 && d.okRate() < 0.85 {
			if s == nil {
				tips = append(tips, name+"：日常模型有 "+pct(1-d.okRate())+" 的回答没通过校验（格式或引用不合格）。建议在模型设置里填一个“难题模型”，系统会自动升级重试。")
			} else {
				tips = append(tips, name+"：日常模型有 "+pct(1-d.okRate())+" 的回答没通过校验，已自动升级到难题模型重试。")
			}
		}
		if g.Calls >= 5 && g.Ms/int64(g.Calls) > 45000 {
			tips = append(tips, name+"：平均每次要 "+itoa(int(g.Ms/int64(g.Calls)/1000))+" 秒。如果嫌慢，可以换响应更快的日常模型。")
		}
	}
	if total.Est {
		tips = append(tips, "部分服务商没有返回用量，这些调用的 token 数是估算的。")
	}
	if total.Calls > 0 && total.Cost == 0 {
		tips = append(tips, "还没有填写模型价格，所以花费显示为 0。在“设置 → 我的 AI 模型 → 编辑”里填上价格（元/百万 tokens）就能估算花费。")
	}
	sort.Strings(tips)
	if tips == nil {
		tips = []string{}
	}
	return tips
}

func pct(x float64) string { return itoa(int(x*100+0.5)) + "%" }

// money 显示金额：不到 1 分钱时多保留几位。
func money(x float64) string {
	if x > 0 && x < 0.01 {
		return ftoa(x, 4)
	}
	return ftoa(x, 2)
}

func ftoa(x float64, prec int) string {
	p := 1.0
	for i := 0; i < prec; i++ {
		p *= 10
	}
	n := int64(x*p + 0.5)
	s := itoa(int(n / int64(p)))
	frac := n % int64(p)
	fs := itoa(int(frac))
	for len(fs) < prec {
		fs = "0" + fs
	}
	return s + "." + fs
}
