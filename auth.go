package main

// 账号与会话。身份只来自服务端会话，不信任客户端传入的用户ID。

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const pbkdf2Iter = 120000

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hLen := prf.Size()
	n := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, n*hLen)
	buf := make([]byte, 4)
	for block := 1; block <= n; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		prf.Write(buf)
		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func newID() string { return randHex(6) }

func hashPassword(pw, salt string) string {
	s, _ := hex.DecodeString(salt)
	return hex.EncodeToString(pbkdf2SHA256([]byte(pw), s, pbkdf2Iter, 32))
}

func checkPassword(u *User, pw string) bool {
	return subtle.ConstantTimeCompare([]byte(hashPassword(pw, u.Salt)), []byte(u.PwHash)) == 1
}

func validPassword(pw string) error {
	if len([]rune(pw)) < 6 {
		return errors.New("密码至少 6 位")
	}
	return nil
}

func tokenHash(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ---- 登录失败限速：同一用户名连续失败 5 次后锁定 5 分钟 ----
type failRec struct {
	n     int
	until time.Time
}

var (
	failMu sync.Mutex
	fails  = map[string]*failRec{}
)

func loginAllowed(name string) bool {
	failMu.Lock()
	defer failMu.Unlock()
	r := fails[name]
	return r == nil || time.Now().After(r.until)
}

func loginFailed(name string) {
	failMu.Lock()
	defer failMu.Unlock()
	r := fails[name]
	if r == nil {
		r = &failRec{}
		fails[name] = r
	}
	r.n++
	if r.n >= 5 {
		r.until = time.Now().Add(5 * time.Minute)
		r.n = 0
	}
}

func loginOK(name string) {
	failMu.Lock()
	delete(fails, name)
	failMu.Unlock()
}

const cookieName = "kyws_session"

// Me 是当前请求的用户快照（在锁外使用）。
type Me struct {
	ID    int
	Name  string
	Role  string
	Token string
}

func (me *Me) IsAdmin() bool   { return me.Role == "admin" }
func (me *Me) IsTeacher() bool { return me.Role == "admin" || me.Role == "teacher" }

func (a *App) currentUser(r *http.Request) *Me {
	tok := ""
	if c, err := r.Cookie(cookieName); err == nil {
		tok = c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		tok = strings.TrimSpace(h[7:])
	}
	if tok == "" {
		return nil
	}
	th := tokenHash(tok)
	var me *Me
	a.store.View(func(db *DB) {
		for _, s := range db.Sessions {
			if s.TokenHash == th {
				u := db.User(s.UserID)
				if u != nil && !u.Disabled {
					me = &Me{ID: u.ID, Name: u.Name, Role: u.Role, Token: tok}
				}
				return
			}
		}
	})
	return me
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
