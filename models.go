package main

// 模型配置卡：每个成员可以接入自己的模型；项目可指定模型；团队有默认模型。
// 使用顺序：项目指定（已通过自检）> 个人启用（已通过自检）> 团队默认。
// 密钥用本机随机密钥（data/secret.key）加密后保存；接口只返回打码后的密钥，管理员也看不到他人的密钥。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CheckItemResult struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

type CheckResult struct {
	Passed   bool              `json:"passed"`
	Items    []CheckItemResult `json:"items"`
	At       time.Time         `json:"at"`
	Warn     bool              `json:"warn,omitempty"`   // 基本通过：必过项都通过，但有提醒项没通过
	Strong   string            `json:"strong,omitempty"` // 同时自检的难题模型名称
	StrongOK bool              `json:"strong_ok,omitempty"`
}

type ModelProfile struct {
	ID          string       `json:"id"`
	OwnerID     int          `json:"owner_id"`
	Name        string       `json:"name"`
	Protocol    string       `json:"protocol"`
	BaseURL     string       `json:"base_url"`
	Model       string       `json:"model"`
	KeyEnc      string       `json:"key_enc"`
	Temperature float64      `json:"temperature"`
	Timeout     int          `json:"timeout"`
	Vision      bool         `json:"vision,omitempty"`
	StrongModel string       `json:"strong_model,omitempty"`
	PriceIn     float64      `json:"price_in,omitempty"`
	PriceOut    float64      `json:"price_out,omitempty"`
	StrongIn    float64      `json:"strong_price_in,omitempty"`
	StrongOut   float64      `json:"strong_price_out,omitempty"`
	Check       *CheckResult `json:"check,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// ---------------- 密钥加密 ----------------

func loadSecret(dir string) []byte {
	p := filepath.Join(dir, "secret.key")
	if b, err := os.ReadFile(p); err == nil && len(b) == 32 {
		return b
	}
	b := make([]byte, 32)
	rand.Read(b)
	os.WriteFile(p, b, 0o600)
	return b
}

func (a *App) enc(plain string) string {
	if plain == "" || len(a.secret) != 32 {
		return plain
	}
	blk, _ := aes.NewCipher(a.secret)
	g, _ := cipher.NewGCM(blk)
	nonce := make([]byte, g.NonceSize())
	rand.Read(nonce)
	return "enc1:" + base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(plain), nil))
}

func (a *App) dec(s string) string {
	if !strings.HasPrefix(s, "enc1:") {
		return s // 旧版本保存的明文
	}
	raw, err := base64.StdEncoding.DecodeString(s[5:])
	if err != nil || len(a.secret) != 32 {
		return ""
	}
	blk, _ := aes.NewCipher(a.secret)
	g, _ := cipher.NewGCM(blk)
	if len(raw) < g.NonceSize() {
		return ""
	}
	out, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	if err != nil {
		return ""
	}
	return string(out)
}

// ---------------- 解析实际使用的模型 ----------------

func (a *App) teamCfgLocked(db *DB) ModelCfg {
	s := db.Settings
	name := s.LLMName
	if name == "" {
		name = "团队默认"
	}
	proto := s.LLMProtocol
	if proto == "" {
		proto = "openai"
	}
	strongOK := s.LLMStrongModel != "" && (s.TeamCheck == nil || s.TeamCheck.Strong != s.LLMStrongModel || s.TeamCheck.StrongOK)
	return ModelCfg{ID: "team", Name: name, Source: "team", Protocol: proto, BaseURL: s.LLMBaseURL, Model: s.LLMModel, Key: a.dec(s.LLMKey), Temperature: 0.1, Vision: s.LLMVision, VisionOK: s.LLMVision && (s.TeamCheck == nil || visionOK(s.TeamCheck)),
		DailyModel: s.LLMModel, StrongModel: s.LLMStrongModel, StrongOK: strongOK, PriceIn: s.LLMPriceIn, PriceOut: s.LLMPriceOut,
		StrongPriceIn: s.LLMStrongPriceIn, StrongPriceOut: s.LLMStrongPriceOut, Tier: "daily", rec: a.recordUsage}
}

func (a *App) teamCfg() ModelCfg {
	var c ModelCfg
	a.store.View(func(db *DB) { c = a.teamCfgLocked(db) })
	return c
}

func (a *App) profileCfg(db *DB, mp *ModelProfile, source string) ModelCfg {
	strongOK := mp.StrongModel != "" && mp.Check != nil && mp.Check.Strong == mp.StrongModel && mp.Check.StrongOK
	return ModelCfg{ID: mp.ID, Name: mp.Name, Source: source, Owner: db.userName(mp.OwnerID), Protocol: mp.Protocol,
		BaseURL: mp.BaseURL, Model: mp.Model, Key: a.dec(mp.KeyEnc), Temperature: mp.Temperature, Timeout: mp.Timeout, Vision: mp.Vision, VisionOK: mp.Vision && visionOK(mp.Check),
		DailyModel: mp.Model, StrongModel: mp.StrongModel, StrongOK: strongOK, PriceIn: mp.PriceIn, PriceOut: mp.PriceOut,
		StrongPriceIn: mp.StrongIn, StrongPriceOut: mp.StrongOut, Tier: "daily", rec: a.recordUsage}
}

func (db *DB) modelProfile(id string) *ModelProfile {
	for _, m := range db.ModelProfiles {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func passed(mp *ModelProfile) bool { return mp != nil && mp.Check != nil && mp.Check.Passed }

// resolveModel 返回此用户在此项目中实际使用的模型。
func (a *App) resolveModel(me *Me, pid string) ModelCfg {
	var c ModelCfg
	a.store.View(func(db *DB) {
		if pid != "" {
			if p := db.Project(pid); p != nil && p.ModelProfileID != "" {
				if p.ModelProfileID == "team" {
					c = a.teamCfgLocked(db)
					c.Source = "project"
					return
				}
				if mp := db.modelProfile(p.ModelProfileID); passed(mp) {
					c = a.profileCfg(db, mp, "project")
					return
				}
			}
		}
		if u := db.User(me.ID); u != nil && u.ActiveModelID != "" {
			if mp := db.modelProfile(u.ActiveModelID); passed(mp) && mp.OwnerID == me.ID {
				c = a.profileCfg(db, mp, "personal")
				return
			}
		}
		c = a.teamCfgLocked(db)
	})
	return c
}

// resolveVision 返回能看图片的模型：当前实际使用的模型能看图就用它；否则用自己已通过自检、标记能看图的模型（启用中的优先）；
// 再否则用团队默认模型（管理员标记为能看图时）。
func (a *App) resolveVision(me *Me, pid string) ModelCfg {
	if c := a.resolveModel(me, pid); c.Configured() && c.VisionOK {
		return c
	}
	var c ModelCfg
	a.store.View(func(db *DB) {
		active := ""
		if u := db.User(me.ID); u != nil {
			active = u.ActiveModelID
		}
		var pick *ModelProfile
		for _, mp := range db.ModelProfiles {
			if mp.OwnerID == me.ID && mp.Vision && visionOK(mp.Check) && (pick == nil || mp.ID == active) {
				pick = mp
			}
		}
		if pick != nil {
			c = a.profileCfg(db, pick, "personal")
			return
		}
		if t := a.teamCfgLocked(db); t.VisionOK && t.Configured() {
			c = t
		}
	})
	return c
}

// ---------------- 接口 ----------------

func (a *App) profileView(db *DB, mp *ModelProfile, me *Me) map[string]any {
	u := db.User(me.ID)
	return map[string]any{"id": mp.ID, "name": mp.Name, "protocol": mp.Protocol, "base_url": mp.BaseURL, "model": mp.Model,
		"key_masked": maskKey(a.dec(mp.KeyEnc)), "temperature": mp.Temperature, "timeout": mp.Timeout, "check": mp.Check, "vision": mp.Vision,
		"strong_model": mp.StrongModel, "price_in": mp.PriceIn, "price_out": mp.PriceOut, "strong_price_in": mp.StrongIn, "strong_price_out": mp.StrongOut,
		"active": u != nil && u.ActiveModelID == mp.ID, "created_at": mp.CreatedAt, "updated_at": mp.UpdatedAt}
}

func (a *App) hListModels(w http.ResponseWriter, r *http.Request, me *Me) error {
	pid := r.URL.Query().Get("project_id")
	eff := a.resolveModel(me, pid)
	var out map[string]any
	a.store.View(func(db *DB) {
		mine := []map[string]any{}
		for _, mp := range db.ModelProfiles {
			if mp.OwnerID == me.ID {
				mine = append(mine, a.profileView(db, mp, me))
			}
		}
		t := a.teamCfgLocked(db)
		out = map[string]any{"mine": mine,
			"team":      map[string]any{"configured": t.Configured(), "name": t.Name, "model": t.Model, "protocol": t.Protocol, "check": db.Settings.TeamCheck, "vision": t.Vision},
			"effective": map[string]any{"label": eff.Label(), "source": eff.Source, "configured": eff.Configured()}}
	})
	writeJSON(w, 200, out)
	return nil
}

type profileIn struct {
	Name        *string  `json:"name"`
	Protocol    *string  `json:"protocol"`
	BaseURL     *string  `json:"base_url"`
	Model       *string  `json:"model"`
	Key         *string  `json:"key"`
	Temperature *float64 `json:"temperature"`
	Timeout     *int     `json:"timeout"`
	Vision      *bool    `json:"vision"`
	StrongModel *string  `json:"strong_model"`
	PriceIn     *float64 `json:"price_in"`
	PriceOut    *float64 `json:"price_out"`
	StrongIn    *float64 `json:"strong_price_in"`
	StrongOut   *float64 `json:"strong_price_out"`
}

func validPrice(p *float64, dst *float64) error {
	if p == nil {
		return nil
	}
	if *p < 0 || *p > 10000 {
		return errBad("价格应在 0 到 10000 元/百万 tokens 之间")
	}
	*dst = *p
	return nil
}

func applyProfile(a *App, mp *ModelProfile, in profileIn) (changed bool, err error) {
	if in.Name != nil {
		mp.Name = strings.TrimSpace(*in.Name)
	}
	if in.Protocol != nil {
		p := strings.TrimSpace(*in.Protocol)
		if p != "openai" && p != "anthropic" {
			return false, errBad("协议类型只能是 openai（OpenAI 兼容）或 anthropic")
		}
		changed = changed || p != mp.Protocol
		mp.Protocol = p
	}
	if in.BaseURL != nil {
		u := strings.TrimRight(strings.TrimSpace(*in.BaseURL), "/")
		if u != "" && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return false, errBad("接口地址需以 http:// 或 https:// 开头")
		}
		changed = changed || u != mp.BaseURL
		mp.BaseURL = u
	}
	if in.Model != nil {
		m := strings.TrimSpace(*in.Model)
		changed = changed || m != mp.Model
		mp.Model = m
	}
	if in.Key != nil && strings.TrimSpace(*in.Key) != "" {
		mp.KeyEnc = a.enc(strings.TrimSpace(*in.Key))
		changed = true
	}
	if in.Temperature != nil {
		if *in.Temperature < 0 || *in.Temperature > 1.5 {
			return false, errBad("温度应在 0 到 1.5 之间（建议 0.1）")
		}
		mp.Temperature = *in.Temperature
	}
	if in.Timeout != nil {
		if *in.Timeout < 10 || *in.Timeout > 600 {
			return false, errBad("超时时间应在 10 到 600 秒之间")
		}
		mp.Timeout = *in.Timeout
	}
	if in.Vision != nil {
		changed = changed || *in.Vision != mp.Vision
		mp.Vision = *in.Vision
	}
	if in.StrongModel != nil {
		sm := strings.TrimSpace(*in.StrongModel)
		if sm == mp.Model {
			sm = ""
		}
		changed = changed || sm != mp.StrongModel
		mp.StrongModel = sm
	}
	for _, x := range []struct {
		p   *float64
		dst *float64
	}{{in.PriceIn, &mp.PriceIn}, {in.PriceOut, &mp.PriceOut}, {in.StrongIn, &mp.StrongIn}, {in.StrongOut, &mp.StrongOut}} {
		if err := validPrice(x.p, x.dst); err != nil {
			return false, err
		}
	}
	if mp.Name == "" {
		mp.Name = mp.Model
	}
	return changed, nil
}

func (a *App) hCreateModel(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in profileIn
	if err := readJSON(r, &in); err != nil {
		return err
	}
	mp := &ModelProfile{ID: newID(), OwnerID: me.ID, Protocol: "openai", Temperature: 0.1, Timeout: 120, CreatedAt: now(), UpdatedAt: now()}
	if _, err := applyProfile(a, mp, in); err != nil {
		return err
	}
	if mp.BaseURL == "" || mp.Model == "" || mp.KeyEnc == "" {
		return errBad("请填写接口地址、模型名称和 API Key")
	}
	err := a.store.Update(func(db *DB) error {
		n := 0
		for _, x := range db.ModelProfiles {
			if x.OwnerID == me.ID {
				n++
			}
		}
		if n >= 20 {
			return errBad("每人最多保存 20 张模型配置卡")
		}
		db.ModelProfiles = append(db.ModelProfiles, mp)
		return nil
	})
	if err != nil {
		return err
	}
	return a.hListModels(w, r, me)
}

func (a *App) withMyProfile(me *Me, id string, fn func(db *DB, mp *ModelProfile) error) error {
	return a.store.Update(func(db *DB) error {
		mp := db.modelProfile(id)
		if mp == nil || mp.OwnerID != me.ID {
			return errNotFound("模型配置不存在或无权访问")
		}
		return fn(db, mp)
	})
}

func (a *App) hUpdateModel(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in profileIn
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.withMyProfile(me, r.PathValue("id"), func(db *DB, mp *ModelProfile) error {
		changed, err := applyProfile(a, mp, in)
		if err != nil {
			return err
		}
		if changed {
			mp.Check = nil // 接口、模型或密钥变了，需要重新自检
		}
		mp.UpdatedAt = now()
		return nil
	})
	if err != nil {
		return err
	}
	return a.hListModels(w, r, me)
}

func (a *App) hDeleteModel(w http.ResponseWriter, r *http.Request, me *Me) error {
	id := r.PathValue("id")
	err := a.withMyProfile(me, id, func(db *DB, mp *ModelProfile) error {
		for i, x := range db.ModelProfiles {
			if x.ID == id {
				db.ModelProfiles = append(db.ModelProfiles[:i], db.ModelProfiles[i+1:]...)
				break
			}
		}
		for _, u := range db.Users {
			if u.ActiveModelID == id {
				u.ActiveModelID = ""
			}
		}
		for _, p := range db.Projects {
			if p.ModelProfileID == id {
				p.ModelProfileID = ""
				db.Log(p.ID, me.ID, "取消项目指定模型", "模型配置卡已被删除")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.hListModels(w, r, me)
}

func (a *App) hActivateModel(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Active bool `json:"active"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.withMyProfile(me, r.PathValue("id"), func(db *DB, mp *ModelProfile) error {
		u := db.User(me.ID)
		if !in.Active {
			if u.ActiveModelID == mp.ID {
				u.ActiveModelID = ""
			}
			return nil
		}
		if !passed(mp) {
			return errBad("请先完成兼容性自检，自检通过后才能启用")
		}
		u.ActiveModelID = mp.ID
		return nil
	})
	if err != nil {
		return err
	}
	return a.hListModels(w, r, me)
}

func (a *App) hCheckModel(w http.ResponseWriter, r *http.Request, me *Me) error {
	id := r.PathValue("id")
	var cfg ModelCfg
	var err error
	a.store.View(func(db *DB) {
		mp := db.modelProfile(id)
		if mp == nil || mp.OwnerID != me.ID {
			err = errNotFound("模型配置不存在或无权访问")
			return
		}
		cfg = a.profileCfg(db, mp, "personal")
	})
	if err != nil {
		return err
	}
	cfg.UserID, cfg.Task = me.ID, "selfcheck"
	res := runSelfCheckAll(cfg)
	err = a.withMyProfile(me, id, func(db *DB, mp *ModelProfile) error {
		mp.Check = &res
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, res)
	return nil
}

func (a *App) hCheckTeamModel(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() {
		return errForbidden("只有管理员可以自检团队默认模型")
	}
	cfg := a.teamCfg()
	cfg.UserID, cfg.Task = me.ID, "selfcheck"
	res := runSelfCheckAll(cfg)
	a.store.Update(func(db *DB) error { db.Settings.TeamCheck = &res; return nil })
	writeJSON(w, 200, res)
	return nil
}

// 导出配置卡（不含密钥），便于组员之间分享、下一届沿用。
func (a *App) hExportModel(w http.ResponseWriter, r *http.Request, me *Me) error {
	var out map[string]any
	var err error
	a.store.View(func(db *DB) {
		mp := db.modelProfile(r.PathValue("id"))
		if mp == nil || mp.OwnerID != me.ID {
			err = errNotFound("模型配置不存在或无权访问")
			return
		}
		out = map[string]any{"kyws_model_card": 1, "name": mp.Name, "protocol": mp.Protocol, "base_url": mp.BaseURL,
			"model": mp.Model, "temperature": mp.Temperature, "timeout": mp.Timeout, "vision": mp.Vision,
			"strong_model": mp.StrongModel, "price_in": mp.PriceIn, "price_out": mp.PriceOut, "strong_price_in": mp.StrongIn, "strong_price_out": mp.StrongOut,
			"note": "本文件不含 API Key。导入后请填写你自己的 Key，并完成兼容性自检。"}
	})
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="model-card.json"`)
	w.Write(b)
	return nil
}

// hProbeModels 用填好的接口地址和密钥查询服务商提供的模型列表（接入向导用），同时验证密钥是否有效。
func (a *App) hProbeModels(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Protocol string `json:"protocol"`
		BaseURL  string `json:"base_url"`
		Key      string `json:"key"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return errBad("接口地址需以 http:// 或 https:// 开头")
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return errBad("请粘贴 API Key")
	}
	if strings.ContainsAny(key, " \n\t") || strings.Contains(key, "：") {
		return errBad("API Key 里不应有空格或换行，请只复制密钥本身")
	}
	var req *http.Request
	if in.Protocol == "anthropic" {
		u := base + "/v1/models"
		if strings.HasSuffix(base, "/v1") {
			u = base + "/models"
		}
		req, _ = http.NewRequest("GET", u, nil)
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req, _ = http.NewRequest("GET", base+"/models", nil)
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return errBad("连不上 " + base + "：请检查运行工作台的电脑能否上网，或接口地址是否正确")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return errBad("服务商说这个 API Key 无效：请确认复制完整（不要多复制空格），并且是在这个服务商创建的")
	case resp.StatusCode == 404:
		// 少数服务商不提供模型列表，但密钥可能是对的
		writeJSON(w, 200, map[string]any{"models": []string{}, "note": "这个服务商不提供模型列表，请手动填写模型名称"})
		return nil
	case resp.StatusCode != 200:
		return errBad("服务商返回错误状态 " + itoa(resp.StatusCode) + "，请稍后再试")
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 4<<20)).Decode(&out)
	models := []string{}
	for _, d := range out.Data {
		if d.ID != "" && len(models) < 300 {
			models = append(models, d.ID)
		}
	}
	writeJSON(w, 200, map[string]any{"models": models})
	return nil
}
