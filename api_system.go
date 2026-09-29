package main

import (
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) hHealth(w http.ResponseWriter, r *http.Request) error {
	setup := false
	var admins []string
	local := isLoopback(r)
	a.store.View(func(db *DB) {
		setup = len(db.Users) > 0
		if local { // 仅在本机登录页提示管理员用户名，方便找回
			for _, u := range db.Users {
				if u.Role == "admin" && !u.Disabled {
					admins = append(admins, u.Username)
				}
			}
		}
	})
	out := map[string]any{"app": "kyws", "version": AppVersion, "setup_done": setup, "local": local}
	if local {
		out["admin_usernames"] = admins
	}
	writeJSON(w, 200, out)
	return nil
}

// 首次使用：在本机创建管理员账号（只能在运行程序的电脑上完成）。
func (a *App) hSetup(w http.ResponseWriter, r *http.Request) error {
	if !isLoopback(r) {
		return errForbidden("首次设置只能在运行工作台的电脑上完成")
	}
	var in struct {
		OrgName                  string `json:"org_name"`
		OrgName2                 string `json:"OrgName"` // 兼容旧写法
		Username, Name, Password string
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.OrgName == "" {
		in.OrgName = in.OrgName2
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" {
		return errBad("请填写登录用户名")
	}
	if err := validPassword(in.Password); err != nil {
		return errBad(err.Error())
	}
	var tok string
	err := a.store.Update(func(db *DB) error {
		if len(db.Users) > 0 {
			return errBad("已完成初始设置，请直接登录")
		}
		salt := randHex(16)
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = in.Username
		}
		u := &User{ID: db.NextUserID, Username: in.Username, Name: name, Role: "admin", Salt: salt, PwHash: hashPassword(in.Password, salt), CreatedAt: now()}
		db.NextUserID++
		db.Users = append(db.Users, u)
		db.Settings.OrgName = strings.TrimSpace(in.OrgName)
		tok = newToken()
		db.Sessions = append(db.Sessions, &Session{TokenHash: tokenHash(tok), UserID: u.ID, CreatedAt: now(), LastSeen: now()})
		return nil
	})
	if err != nil {
		return err
	}
	setCookie(w, tok)
	writeJSON(w, 200, map[string]any{"ok": true, "token": tok})
	return nil
}

func setCookie(w http.ResponseWriter, tok string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 60 * 60 * 24 * 30})
}

func (a *App) hLogin(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Username, Password string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Username)
	if !loginAllowed(name) {
		return &apiError{429, "连续输错次数过多，请 5 分钟后再试"}
	}
	var u *User
	a.store.View(func(db *DB) {
		for _, x := range db.Users {
			if x.Username == name {
				c := *x
				u = &c
			}
		}
	})
	if u == nil || u.Disabled || !checkPassword(u, in.Password) {
		loginFailed(name)
		return &apiError{401, "用户名或密码错误"}
	}
	loginOK(name)
	tok := newToken()
	err := a.store.Update(func(db *DB) error {
		// 清理 30 天未使用的会话
		keep := db.Sessions[:0]
		for _, s := range db.Sessions {
			if time.Since(s.LastSeen) < 30*24*time.Hour {
				keep = append(keep, s)
			}
		}
		db.Sessions = append(keep, &Session{TokenHash: tokenHash(tok), UserID: u.ID, CreatedAt: now(), LastSeen: now()})
		return nil
	})
	if err != nil {
		return err
	}
	setCookie(w, tok)
	writeJSON(w, 200, map[string]any{"ok": true, "token": tok})
	return nil
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request, me *Me) error {
	th := tokenHash(me.Token)
	a.store.Update(func(db *DB) error {
		keep := db.Sessions[:0]
		for _, s := range db.Sessions {
			if s.TokenHash != th {
				keep = append(keep, s)
			}
		}
		db.Sessions = keep
		return nil
	})
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (a *App) hMe(w http.ResponseWriter, r *http.Request, me *Me) error {
	eff := a.resolveModel(me, "")
	var out map[string]any
	a.store.View(func(db *DB) {
		u := db.User(me.ID)
		out = map[string]any{"id": u.ID, "username": u.Username, "name": u.Name, "role": u.Role,
			"must_change_pw": u.MustChangePw, "org_name": db.Settings.OrgName,
			"llm_configured": eff.Configured(), "team_llm_configured": llmConfigured(db.Settings), "llm_model": eff.Label(),
			"lan_enabled": db.Settings.LANEnabled, "local": isLoopback(r), "version": AppVersion,
			"model_source": eff.Source, "strong_model": eff.StrongModel, "team_budget": db.Settings.TeamBudget, "team_spent": teamSpent(db, me.ID, monthOf(time.Now()))}
	})
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hChangePassword(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct{ Old, New string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if err := validPassword(in.New); err != nil {
		return errBad(err.Error())
	}
	err := a.store.Update(func(db *DB) error {
		u := db.User(me.ID)
		if !checkPassword(u, in.Old) {
			return errBad("原密码不正确")
		}
		u.Salt = randHex(16)
		u.PwHash = hashPassword(in.New, u.Salt)
		u.MustChangePw = false
		// 使其他设备上的登录失效
		th := tokenHash(me.Token)
		keep := db.Sessions[:0]
		for _, s := range db.Sessions {
			if s.UserID != me.ID || s.TokenHash == th {
				keep = append(keep, s)
			}
		}
		db.Sessions = keep
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

// 修改自己的显示姓名和登录用户名
func (a *App) hPatchMe(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Name     *string `json:"name"`
		Username *string `json:"username"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	err := a.store.Update(func(db *DB) error {
		u := db.User(me.ID)
		if in.Name != nil {
			n := strings.TrimSpace(*in.Name)
			if n == "" {
				return errBad("姓名不能为空")
			}
			u.Name = n
		}
		if in.Username != nil {
			n := strings.TrimSpace(*in.Username)
			if n == "" {
				return errBad("用户名不能为空")
			}
			for _, x := range db.Users {
				if x.ID != u.ID && x.Username == n {
					return errBad("用户名 " + n + " 已被使用")
				}
			}
			u.Username = n
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.hMe(w, r, me)
}

func userView(u *User) map[string]any {
	return map[string]any{"id": u.ID, "username": u.Username, "name": u.Name, "role": u.Role, "disabled": u.Disabled, "created_at": u.CreatedAt}
}

func (a *App) hListUsers(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsTeacher() {
		return errForbidden("只有老师可以查看成员列表")
	}
	var out []map[string]any
	a.store.View(func(db *DB) {
		for _, u := range db.Users {
			out = append(out, userView(u))
		}
	})
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hCreateUser(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct{ Username, Name, Role, Password string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if !me.IsTeacher() {
		return errForbidden("只有老师可以添加成员")
	}
	if in.Role == "" {
		in.Role = "student"
	}
	if in.Role != "student" && in.Role != "teacher" && in.Role != "admin" {
		return errBad("角色无效")
	}
	if in.Role != "student" && !me.IsAdmin() {
		return errForbidden("只有管理员可以添加老师或管理员")
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" {
		return errBad("请填写登录用户名")
	}
	if err := validPassword(in.Password); err != nil {
		return errBad("初始" + err.Error())
	}
	var out map[string]any
	err := a.store.Update(func(db *DB) error {
		for _, u := range db.Users {
			if u.Username == in.Username {
				return errBad("用户名 " + in.Username + " 已存在")
			}
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = in.Username
		}
		salt := randHex(16)
		u := &User{ID: db.NextUserID, Username: in.Username, Name: name, Role: in.Role, Salt: salt, PwHash: hashPassword(in.Password, salt), MustChangePw: true, CreatedAt: now()}
		db.NextUserID++
		db.Users = append(db.Users, u)
		out = userView(u)
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hUpdateUser(w http.ResponseWriter, r *http.Request, me *Me) error {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var in struct {
		Name     *string
		Role     *string
		Disabled *bool
		Password *string
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	var out map[string]any
	err := a.store.Update(func(db *DB) error {
		u := db.User(id)
		if u == nil {
			return errNotFound("用户不存在")
		}
		// 老师只能管理学生；管理员可管理所有人
		if !me.IsAdmin() && !(me.IsTeacher() && u.Role == "student") {
			return errForbidden("无权修改该用户")
		}
		if in.Role != nil {
			if !me.IsAdmin() {
				return errForbidden("只有管理员可以修改角色")
			}
			if *in.Role != "student" && *in.Role != "teacher" && *in.Role != "admin" {
				return errBad("角色无效")
			}
			if u.ID == me.ID && *in.Role != "admin" {
				return errBad("不能取消自己的管理员身份")
			}
			u.Role = *in.Role
		}
		if in.Disabled != nil {
			if u.ID == me.ID {
				return errBad("不能停用自己")
			}
			u.Disabled = *in.Disabled
			if u.Disabled {
				keep := db.Sessions[:0]
				for _, s := range db.Sessions {
					if s.UserID != u.ID {
						keep = append(keep, s)
					}
				}
				db.Sessions = keep
			}
		}
		if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
			u.Name = strings.TrimSpace(*in.Name)
		}
		if in.Password != nil {
			if err := validPassword(*in.Password); err != nil {
				return errBad(err.Error())
			}
			u.Salt = randHex(16)
			u.PwHash = hashPassword(*in.Password, u.Salt)
			u.MustChangePw = u.ID != me.ID
		}
		out = userView(u)
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	rs := []rune(k)
	if len(rs) <= 8 {
		return "已填写"
	}
	return string(rs[:3]) + "…" + string(rs[len(rs)-4:])
}

func (a *App) hGetSettings(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() {
		return errForbidden("只有管理员可以查看设置")
	}
	s := a.settings()
	writeJSON(w, 200, map[string]any{"org_name": s.OrgName, "llm_base_url": s.LLMBaseURL, "llm_model": s.LLMModel,
		"llm_key_masked": maskKey(a.dec(s.LLMKey)), "llm_key_set": s.LLMKey != "", "lan_enabled": s.LANEnabled,
		"llm_protocol": map[bool]string{true: s.LLMProtocol, false: "openai"}[s.LLMProtocol != ""], "llm_name": s.LLMName, "llm_vision": s.LLMVision, "team_check": s.TeamCheck,
		"llm_strong_model": s.LLMStrongModel, "llm_price_in": s.LLMPriceIn, "llm_price_out": s.LLMPriceOut,
		"llm_strong_price_in": s.LLMStrongPriceIn, "llm_strong_price_out": s.LLMStrongPriceOut, "team_budget": s.TeamBudget,
		"online_check": s.OnlineCheck, "llm_configured": llmConfigured(s)})
	return nil
}

func (a *App) hPutSettings(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() {
		return errForbidden("只有管理员可以修改设置")
	}
	var in struct {
		OrgName     *string  `json:"org_name"`
		LLMBaseURL  *string  `json:"llm_base_url"`
		LLMModel    *string  `json:"llm_model"`
		LLMKey      *string  `json:"llm_key"`
		LLMProtocol *string  `json:"llm_protocol"`
		LLMName     *string  `json:"llm_name"`
		LLMVision   *bool    `json:"llm_vision"`
		LLMStrong   *string  `json:"llm_strong_model"`
		PriceIn     *float64 `json:"llm_price_in"`
		PriceOut    *float64 `json:"llm_price_out"`
		StrongIn    *float64 `json:"llm_strong_price_in"`
		StrongOut   *float64 `json:"llm_strong_price_out"`
		TeamBudget  *float64 `json:"team_budget"`
		ClearKey    bool     `json:"clear_key"`
		LANEnabled  *bool    `json:"lan_enabled"`
		OnlineCheck *bool    `json:"online_check"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.LANEnabled != nil && !isLoopback(r) {
		return errForbidden("手机访问开关只能在运行工作台的电脑上修改")
	}
	err := a.store.Update(func(db *DB) error {
		s := &db.Settings
		before := s.LLMBaseURL + "|" + s.LLMModel + "|" + s.LLMProtocol + "|" + strconv.FormatBool(s.LLMVision)
		if in.OrgName != nil {
			s.OrgName = strings.TrimSpace(*in.OrgName)
		}
		if in.LLMBaseURL != nil {
			u := strings.TrimRight(strings.TrimSpace(*in.LLMBaseURL), "/")
			if u != "" && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				return errBad("接口地址需以 http:// 或 https:// 开头")
			}
			s.LLMBaseURL = u
		}
		if in.LLMModel != nil {
			s.LLMModel = strings.TrimSpace(*in.LLMModel)
		}
		if in.LLMKey != nil && strings.TrimSpace(*in.LLMKey) != "" {
			s.LLMKey = a.enc(strings.TrimSpace(*in.LLMKey))
			s.TeamCheck = nil
		}
		if in.LLMProtocol != nil {
			if *in.LLMProtocol != "openai" && *in.LLMProtocol != "anthropic" {
				return errBad("协议类型无效")
			}
			s.LLMProtocol = *in.LLMProtocol
		}
		if in.LLMName != nil {
			s.LLMName = strings.TrimSpace(*in.LLMName)
		}
		if in.ClearKey {
			s.LLMKey = ""
			s.TeamCheck = nil
		}
		if in.LLMVision != nil {
			s.LLMVision = *in.LLMVision
		}
		if in.LLMStrong != nil {
			sm := strings.TrimSpace(*in.LLMStrong)
			if sm == s.LLMModel {
				sm = ""
			}
			if sm != s.LLMStrongModel {
				s.TeamCheck = nil
			}
			s.LLMStrongModel = sm
		}
		for _, x := range []struct {
			p   *float64
			dst *float64
		}{{in.PriceIn, &s.LLMPriceIn}, {in.PriceOut, &s.LLMPriceOut}, {in.StrongIn, &s.LLMStrongPriceIn}, {in.StrongOut, &s.LLMStrongPriceOut}} {
			if err := validPrice(x.p, x.dst); err != nil {
				return err
			}
		}
		if in.TeamBudget != nil {
			if *in.TeamBudget < 0 || *in.TeamBudget > 100000 {
				return errBad("额度应在 0 到 100000 元之间（0 表示不限）")
			}
			s.TeamBudget = *in.TeamBudget
		}
		if before != s.LLMBaseURL+"|"+s.LLMModel+"|"+s.LLMProtocol+"|"+strconv.FormatBool(s.LLMVision) {
			s.TeamCheck = nil
		}
		if in.LANEnabled != nil {
			s.LANEnabled = *in.LANEnabled
		}
		if in.OnlineCheck != nil {
			s.OnlineCheck = *in.OnlineCheck
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.hGetSettings(w, r, me)
}

func (a *App) hTestLLM(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() {
		return errForbidden("只有管理员可以测试模型")
	}
	start := time.Now()
	tcfg := a.teamCfg()
	tcfg.UserID, tcfg.Task = me.ID, "test"
	out, err := chatJSON(tcfg, "你是接口连通性测试助手。只输出 JSON。", `请原样输出：{"ok": true}`)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return nil
	}
	_ = out
	writeJSON(w, 200, map[string]any{"ok": true, "message": "连接成功，用时 " + strconv.FormatFloat(time.Since(start).Seconds(), 'f', 1, 64) + " 秒"})
	return nil
}

func lanIPs() []string {
	var ips []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, ad := range addrs {
			ipn, ok := ad.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	// 私有网段优先（192.168 > 10 > 172）
	rank := func(s string) int {
		switch {
		case strings.HasPrefix(s, "192.168."):
			return 0
		case strings.HasPrefix(s, "10."):
			return 1
		case strings.HasPrefix(s, "172."):
			return 2
		}
		return 3
	}
	sort.SliceStable(ips, func(i, j int) bool { return rank(ips[i]) < rank(ips[j]) })
	return ips
}

func (a *App) hLANInfo(w http.ResponseWriter, r *http.Request, me *Me) error {
	var urls []string
	for _, ip := range lanIPs() {
		urls = append(urls, "http://"+ip+":"+strconv.Itoa(a.port)+"/")
	}
	writeJSON(w, 200, map[string]any{"enabled": a.settings().LANEnabled, "urls": urls, "port": a.port})
	return nil
}

func (a *App) hQuit(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() || !isLoopback(r) {
		return errForbidden("只能由管理员在运行工作台的电脑上退出程序")
	}
	log.Printf("管理员 %s 在设置中点击了“退出工作台程序”", me.Name)
	writeJSON(w, 200, map[string]bool{"ok": true})
	go func() {
		time.Sleep(300 * time.Millisecond)
		select {
		case <-a.quit:
		default:
			close(a.quit)
		}
	}()
	return nil
}
