package main

// 按难度分配模型：同一个接口和密钥下可以配置“日常模型”（便宜、快）和“难题模型”（强、贵）。
// 简单任务（拆检索词、文献速读、概念讲解）用日常模型；难的任务（引用核验、资料对比、改文件、复杂问题）用难题模型；
// 日常模型的回答没通过校验（格式或引用不合格）时，自动用难题模型重试一次。
// 用户也可以手动选择“快速”或“深度”。团队模型可设每人每月额度。

import (
	"strings"
	"time"
	"unicode/utf8"
)

var hardTasks = map[string]string{"citecheck": "引用核验需要逐句比对原文", "research": "综合多篇文献写调研报告", "writing_plan": "搭建论证骨架需要梳理证据", "writing_draft": "按规范起草论文需要较强的写作能力", "compare": "对比两份资料需要细致分析", "agent": "修改文件需要多步推理", "contract": "梳理研究问题和核心主张需要较强推理", "attack": "站在审稿人角度找漏洞需要较强推理", "rebut": "判断回应是否成立需要严谨，不能随便让步", "drift": "对照契约逐句检查需要细致"}

var hardWords = []string{"为什么", "原因", "机制", "推导", "证明", "比较", "对比", "区别", "异同", "矛盾", "是否一致", "评价", "优缺点", "局限", "如何理解",
	"why", "how does", "derive", "prove", "compare", "difference", "contradict"}

// judgeDifficulty 粗略判断难度：任务类型、问题长度、是否需要推理或比较、涉及资料数量。
func judgeDifficulty(task, text string, nMat int) (bool, string) {
	if why, ok := hardTasks[task]; ok {
		return true, why
	}
	switch task {
	case "keywords", "brief", "concept", "vision", "sketch", "ocr", "screen", "selfcheck", "test":
		return false, "这类任务比较简单"
	}
	lt := strings.ToLower(text)
	for _, w := range hardWords {
		if strings.Contains(lt, w) {
			return true, "问题需要推理或比较（含“" + w + "”）"
		}
	}
	if utf8.RuneCountInString(text) > 80 {
		return true, "问题较长、条件较多"
	}
	if nMat >= 4 {
		return true, "涉及 " + itoa(nMat) + " 份资料"
	}
	return false, "普通问题"
}

// modelFor 返回这次调用应使用的模型。effort：auto（默认）/ fast / deep。
func (a *App) modelFor(me *Me, pid, task, effort, text string, nMat int) ModelCfg {
	c := a.resolveModel(me, pid)
	c.Task, c.UserID = task, me.ID
	var hard bool
	var why string
	switch effort {
	case "fast":
		why = "你选择了“快速”"
	case "deep":
		hard, why = true, "你选择了“深度”"
	default:
		hard, why = judgeDifficulty(task, text, nMat)
	}
	c = c.withTier(hard, why)
	a.applyBudget(&c)
	return c
}

func (c ModelCfg) withTier(hard bool, why string) ModelCfg {
	if c.DailyModel == "" {
		c.DailyModel = c.Model
	}
	if hard && c.StrongModel != "" && c.StrongOK {
		c.Model, c.Tier = c.StrongModel, "strong"
		c.PriceIn, c.PriceOut = c.StrongPriceIn, c.StrongPriceOut
		c.Route = "难题 → 难题模型（" + why + "）"
		return c
	}
	c.Model, c.Tier = c.DailyModel, "daily"
	switch {
	case !hard:
		c.Route = "日常模型（" + why + "）"
	case c.StrongModel != "" && !c.StrongOK:
		c.Route = "判断为难题（" + why + "），但难题模型还没通过自检，先用日常模型"
	default:
		c.Route = "判断为难题（" + why + "），没有配置难题模型，用日常模型"
	}
	return c
}

// escalate：日常模型的结果没通过校验时，换难题模型重试。
func (c ModelCfg) escalate() (ModelCfg, bool) {
	if c.Tier == "strong" || c.StrongModel == "" || !c.StrongOK {
		return c, false
	}
	s := c.withTier(true, "")
	s.Route = "日常模型的回答没通过校验，自动改用难题模型重试"
	return s, true
}

func (a *App) applyBudget(c *ModelCfg) {
	if c.Source != "team" || !c.Configured() {
		return
	}
	a.store.View(func(db *DB) {
		b := db.Settings.TeamBudget
		if b > 0 && teamSpent(db, c.UserID, monthOf(time.Now())) >= b {
			c.Blocked = "本月团队 AI 额度（每人 ¥" + money(b) + "）已用完。可以在“设置 → 我的 AI 模型”接入自己的模型，或请老师调整额度"
		}
	})
}
