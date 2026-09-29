package main

// 管理员后台（总览、备份、打开数据文件夹）与本机密码找回。
// 密码找回的依据：能在这台电脑上运行程序、读取数据目录的人，本来就能访问全部数据，
// 因此“本机操作 + 数据目录中的控制令牌”即视为有权重置。

import (
	"archive/zip"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// ---------------- 密码找回 ----------------

const tempPwChars = "abcdefghjkmnpqrstuvwxyz23456789"

func tempPassword() string {
	b := []byte(randHex(8))
	out := make([]byte, 8)
	for i := range out {
		out[i] = tempPwChars[int(b[i])%len(tempPwChars)]
	}
	return string(out)
}

// resetAdmin 重置最早创建的管理员的密码（调用方持有写锁）。
func resetAdmin(db *DB) (username, password string, err error) {
	var target *User
	for _, u := range db.Users {
		if u.Role == "admin" {
			target = u
			break
		}
	}
	if target == nil {
		return "", "", errors.New("还没有管理员账号，请直接打开工作台完成首次设置")
	}
	password = tempPassword()
	target.Salt = randHex(16)
	target.PwHash = hashPassword(password, target.Salt)
	target.MustChangePw = true
	target.Disabled = false
	keep := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.UserID != target.ID {
			keep = append(keep, s)
		}
	}
	db.Sessions = keep
	loginOK(target.Username)
	return target.Username, password, nil
}

func controlTokenPath(dir string) string { return filepath.Join(dir, "control.token") }

func writeControlToken(dir string) string {
	t := randHex(24)
	os.WriteFile(controlTokenPath(dir), []byte(t), 0o600)
	return t
}

// 本机控制接口：仅限本机 + 数据目录中的令牌（程序运行时由“忘记管理员密码”工具调用）。
func (a *App) hLocalControl(w http.ResponseWriter, r *http.Request) error {
	if !isLoopback(r) {
		return errForbidden("仅限本机")
	}
	var in struct {
		Token  string `json:"token"`
		Action string `json:"action"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if a.controlToken == "" || subtle.ConstantTimeCompare([]byte(in.Token), []byte(a.controlToken)) != 1 {
		return errForbidden("令牌无效")
	}
	if in.Action != "reset-admin" {
		return errBad("未知操作")
	}
	var user, pw string
	err := a.store.Update(func(db *DB) error {
		var e error
		user, pw, e = resetAdmin(db)
		if e != nil {
			return errBad(e.Error())
		}
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]string{"username": user, "password": pw})
	return nil
}

// runResetAdmin 由命令行参数 -reset-admin 调用。
func runResetAdmin(dataDir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dataDir, "data.json")); err != nil {
		return "", errors.New("还没有任何数据，请直接打开工作台完成首次设置。")
	}
	store, err := OpenStore(dataDir)
	if err != nil {
		return "", err
	}
	port := 0
	store.View(func(db *DB) { port = db.Settings.Port })
	if port > 0 && probe(port) {
		// 程序正在运行：交给运行中的程序处理，避免两处同时写数据
		tok, err := os.ReadFile(controlTokenPath(dataDir))
		if err != nil {
			return "", errors.New("工作台正在运行，但无法读取控制令牌。请先在任务管理器中结束“CanDo”后再试。")
		}
		body, _ := json.Marshal(map[string]string{"token": strings.TrimSpace(string(tok)), "action": "reset-admin"})
		req, _ := http.NewRequest("POST", "http://127.0.0.1:"+itoa(port)+"/api/local/control", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-KY", "1")
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			return "", errors.New("无法连接正在运行的工作台：" + err.Error())
		}
		defer resp.Body.Close()
		var out map[string]string
		json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != 200 {
			return "", errors.New("重置失败：" + out["detail"])
		}
		return resetMsg(out["username"], out["password"]), nil
	}
	var user, pw string
	err = store.Update(func(db *DB) error {
		var e error
		user, pw, e = resetAdmin(db)
		return e
	})
	if err != nil {
		return "", err
	}
	return resetMsg(user, pw), nil
}

func resetMsg(user, pw string) string {
	return "管理员密码已重置。\n\n用户名：" + user + "\n临时密码：" + pw + "\n\n请打开工作台用这个临时密码登录，登录后在“设置”中改成自己的密码。\n（其他成员的账号和所有数据不受影响）"
}

// ---------------- 后台总览 ----------------

func dirSize(dir string) (int64, int) {
	var total int64
	n := 0
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
				n++
			}
		}
		return nil
	})
	return total, n
}

func (a *App) hAdminOverview(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() {
		return errForbidden("只有管理员可以查看后台")
	}
	size, nfiles := dirSize(a.store.dir)
	out := map[string]any{"data_dir": a.store.dir, "data_size": size, "file_count": nfiles, "local": isLoopback(r), "version": AppVersion}
	a.store.View(func(db *DB) {
		roles := map[string]int{}
		disabled := 0
		for _, u := range db.Users {
			roles[u.Role]++
			if u.Disabled {
				disabled++
			}
		}
		matCount, matSize, failed := 0, int64(0), 0
		perProj := map[string]int{}
		for _, m := range db.Materials {
			if m.DeletedAt != nil {
				continue
			}
			matCount++
			matSize += m.Size
			if m.Status == "failed" {
				failed++
			}
			perProj[m.ProjectID]++
		}
		lastAct := map[string]time.Time{}
		for _, ac := range db.Activities {
			if ac.At.After(lastAct[ac.ProjectID]) {
				lastAct[ac.ProjectID] = ac.At
			}
		}
		projects := []map[string]any{}
		active, archived, pending := 0, 0, 0
		for _, p := range db.Projects {
			s := projectSummary(db, me, p)
			s["members"] = namesStr(db, p.Members)
			s["material_count"] = perProj[p.ID]
			if t, ok := lastAct[p.ID]; ok {
				s["last_activity"] = t
			}
			projects = append(projects, s)
			if p.Status == "active" {
				active++
			} else {
				archived++
			}
			pending += s["pending_reviews"].(int)
		}
		sort.SliceStable(projects, func(i, j int) bool {
			ti, _ := projects[i]["last_activity"].(time.Time)
			tj, _ := projects[j]["last_activity"].(time.Time)
			return ti.After(tj)
		})
		pname := map[string]string{}
		for _, p := range db.Projects {
			pname[p.ID] = p.Name
		}
		recent := []map[string]any{}
		for i := len(db.Activities) - 1; i >= 0 && len(recent) < 50; i-- {
			ac := db.Activities[i]
			recent = append(recent, map[string]any{"project_id": ac.ProjectID, "project": pname[ac.ProjectID], "user": db.userName(ac.UserID), "action": ac.Action, "detail": ac.Detail, "at": ac.At})
		}
		checks := 0
		for _, c := range db.CiteChecks {
			if c.Status == "done" {
				checks++
			}
		}
		personal := perProj[""]
		out["stats"] = map[string]any{"users": len(db.Users), "admins": roles["admin"], "teachers": roles["teacher"], "students": roles["student"], "disabled": disabled,
			"projects_active": active, "projects_archived": archived, "pending_reviews": pending, "materials": matCount, "materials_size": matSize,
			"materials_failed": failed, "personal_materials": personal, "answers": len(db.Answers), "cite_checks": len(db.CiteChecks), "cite_checks_done": checks,
			"templates_custom": len(db.Templates), "llm_configured": llmConfigured(db.Settings), "lan_enabled": db.Settings.LANEnabled}
		out["projects"] = projects
		out["recent"] = recent
	})
	writeJSON(w, 200, out)
	return nil
}

// 下载备份：打包整个数据目录（账号、项目、资料原文件）。
func (a *App) hAdminBackup(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() {
		return errForbidden("只有管理员可以下载备份")
	}
	var snapshot []byte
	a.store.View(func(db *DB) { snapshot, _ = json.Marshal(db) })
	name := "CanDo可为备份_" + time.Now().Format("20060102_1504") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "attachment; filename=\"backup.zip\"; filename*=UTF-8''"+url.PathEscape(name))
	zw := zip.NewWriter(w)
	defer zw.Close()
	f, _ := zw.Create("data/data.json")
	f.Write(snapshot)
	for _, sub := range []string{"files", "chunks"} {
		root := filepath.Join(a.store.dir, sub)
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasSuffix(p, ".tmp") {
				return nil
			}
			src, e := os.Open(p)
			if e != nil {
				return nil
			}
			defer src.Close()
			dst, e := zw.Create("data/" + sub + "/" + filepath.Base(p))
			if e == nil {
				io.Copy(dst, src)
			}
			return nil
		})
	}
	readme, _ := zw.Create("恢复说明.txt")
	readme.Write([]byte("恢复方法：\r\n1. 在任务管理器中结束 CanDo。\r\n2. 把压缩包中的 data 文件夹复制到 %LOCALAPPDATA%\\KeyanWorkbench\\ 下（替换原有 data 文件夹）。\r\n3. 重新打开工作台。\r\n"))
	return nil
}

func (a *App) hAdminOpenDir(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !me.IsAdmin() || !isLoopback(r) {
		return errForbidden("只能由管理员在运行工作台的电脑上打开")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", a.store.dir)
	case "darwin":
		cmd = exec.Command("open", a.store.dir)
	default:
		cmd = exec.Command("xdg-open", a.store.dir)
	}
	if err := cmd.Start(); err != nil {
		return errBad("无法打开文件夹：" + err.Error())
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}
