package main

// 数据存储：全部数据保存在内存，并在每次修改后原子写入 data.json。
// 片段（chunks）按材料分文件存放，原文件存放在 files/ 目录。
// 面向课题组规模（几十人、几百份资料），不追求大规模并发。

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type User struct {
	ID                int           `json:"id"`
	Username          string        `json:"username"`
	Name              string        `json:"name"`
	Role              string        `json:"role"` // admin / teacher / student
	Salt              string        `json:"salt"`
	PwHash            string        `json:"pw_hash"`
	MustChangePw      bool          `json:"must_change_pw"`
	ActiveModelID     string        `json:"active_model_id,omitempty"`
	Zotero            *ZoteroCloud  `json:"zotero,omitempty"`
	AgentFolders      []AgentFolder `json:"agent_folders,omitempty"`
	OpenAlexKey       string        `json:"openalex_key,omitempty"`
	WebSearchKey      string        `json:"web_search_key,omitempty"` // 加密保存
	WebSearchProvider string        `json:"web_search_provider,omitempty"`
	AgentMemory       string        `json:"agent_memory,omitempty"`
	AgentPolicy       *AgentPolicy  `json:"agent_policy,omitempty"`
	Disabled          bool          `json:"disabled"`
	CreatedAt         time.Time     `json:"created_at"`
}

type Session struct {
	TokenHash string    `json:"token_hash"`
	UserID    int       `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
}

type CheckItem struct {
	Text   string     `json:"text"`
	Done   bool       `json:"done"`
	DoneBy int        `json:"done_by,omitempty"`
	DoneAt *time.Time `json:"done_at,omitempty"`
}

type Submission struct {
	ID          string    `json:"id"`
	UserID      int       `json:"user_id"`
	Content     string    `json:"content"`
	MaterialIDs []string  `json:"material_ids"`
	CreatedAt   time.Time `json:"created_at"`
}

type Review struct {
	ID        string    `json:"id"`
	UserID    int       `json:"user_id"`
	Decision  string    `json:"decision"` // approve / return / comment
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type Stage struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Goal        string       `json:"goal"`
	Guide       string       `json:"guide"`
	Checklist   []CheckItem  `json:"checklist"`
	Status      string       `json:"status"` // todo / doing / review / done / returned
	Due         string       `json:"due"`
	Submissions []Submission `json:"submissions"`
	Reviews     []Review     `json:"reviews"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Retro struct {
	Good     string `json:"good"`
	Pitfalls string `json:"pitfalls"`
	Advice   string `json:"advice"`
}

type Project struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Desc           string     `json:"desc"`
	CreatedBy      int        `json:"created_by"`
	Advisors       []int      `json:"advisors"`
	Members        []int      `json:"members"`
	Status         string     `json:"status"` // active / archived
	Stages         []*Stage   `json:"stages"`
	Retro          *Retro     `json:"retro,omitempty"`
	ShareToLibrary bool       `json:"share_to_library"`
	ModelProfileID string     `json:"model_profile_id,omitempty"` // 项目指定模型：配置卡ID 或 "team"
	CreatedAt      time.Time  `json:"created_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
}

type Activity struct {
	ID        int       `json:"id"`
	ProjectID string    `json:"project_id"`
	UserID    int       `json:"user_id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	At        time.Time `json:"at"`
}

type Material struct {
	ID         string     `json:"id"`
	OwnerID    int        `json:"owner_id"`
	ProjectID  string     `json:"project_id"` // 空 = 个人资料
	Title      string     `json:"title"`
	Filename   string     `json:"filename"`
	Ftype      string     `json:"ftype"`
	Version    int        `json:"version"`
	Size       int64      `json:"size"`
	UploadedAt time.Time  `json:"uploaded_at"`
	Status     string     `json:"status"` // ready / partial / failed / deleted
	Error      string     `json:"error"`
	ParseNote  string     `json:"parse_note"`
	Author     string     `json:"author"`
	SourceDate string     `json:"source_date"`
	DOI        string     `json:"doi,omitempty"`
	SourceURL  string     `json:"source_url,omitempty"`
	ZoteroKey  string     `json:"zotero_key,omitempty"`
	Shared     bool       `json:"shared,omitempty"` // 个人论文共享给全组（其他成员只读）
	ChunkCount int        `json:"chunk_count"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

type Chunk struct {
	ID        string `json:"id"`
	Seq       int    `json:"seq"`
	PageIndex int    `json:"page_index,omitempty"`
	PageLabel string `json:"page_label,omitempty"`
	ParaIndex int    `json:"para_index"`
	Text      string `json:"text"`
	OCR       bool   `json:"ocr,omitempty"` // 由识图模型从扫描页识别出的文字
}

type Answer struct {
	ID          string          `json:"id"`
	OwnerID     int             `json:"owner_id"`
	ProjectID   string          `json:"project_id"`
	Kind        string          `json:"kind"`
	Question    string          `json:"question"`
	MaterialIDs []string        `json:"material_ids"`
	Result      json.RawMessage `json:"result"`
	CreatedAt   time.Time       `json:"created_at"`
}

type CiteCheck struct {
	ID        string          `json:"id"`
	OwnerID   int             `json:"owner_id"`
	ProjectID string          `json:"project_id"`
	Title     string          `json:"title"`
	Status    string          `json:"status"` // parsed / running / done / failed
	Progress  int             `json:"progress"`
	Total     int             `json:"total"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"` // CiteData
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Template struct {
	Key       string          `json:"key"`
	Name      string          `json:"name"`
	Desc      string          `json:"desc"`
	Stages    []TemplateStage `json:"stages"`
	BuiltIn   bool            `json:"built_in"`
	CreatedBy int             `json:"created_by,omitempty"`
}

type TemplateStage struct {
	Name      string   `json:"name"`
	Goal      string   `json:"goal"`
	Guide     string   `json:"guide"`
	Checklist []string `json:"checklist"`
}

type Settings struct {
	OrgName     string       `json:"org_name"`
	LLMBaseURL  string       `json:"llm_base_url"`
	LLMKey      string       `json:"llm_key"`
	LLMProtocol string       `json:"llm_protocol,omitempty"`
	LLMName     string       `json:"llm_name,omitempty"`
	LLMVision   bool         `json:"llm_vision,omitempty"`
	TeamCheck   *CheckResult `json:"team_check,omitempty"`
	LLMModel    string       `json:"llm_model"`
	// 智能分配与用量
	LLMStrongModel    string  `json:"llm_strong_model,omitempty"`
	LLMPriceIn        float64 `json:"llm_price_in,omitempty"`
	LLMPriceOut       float64 `json:"llm_price_out,omitempty"`
	LLMStrongPriceIn  float64 `json:"llm_strong_price_in,omitempty"`
	LLMStrongPriceOut float64 `json:"llm_strong_price_out,omitempty"`
	TeamBudget        float64 `json:"team_budget,omitempty"`    // 每人每月使用团队模型的额度（元），0 表示不限
	OpenAlexKey       string  `json:"openalex_key,omitempty"`   // 加密保存
	WebSearchKey      string  `json:"web_search_key,omitempty"` // 加密保存
	WebSearchProvider string  `json:"web_search_provider,omitempty"`
	ContactEmail      string  `json:"contact_email,omitempty"` // Crossref 礼貌访问用
	LANEnabled        bool    `json:"lan_enabled"`
	OnlineCheck       bool    `json:"online_check"` // 引用核验时是否联网查询公开文献数据库
	Port              int     `json:"port"`
}

type DB struct {
	Users         []*User          `json:"users"`
	Sessions      []*Session       `json:"sessions"`
	Projects      []*Project       `json:"projects"`
	Activities    []*Activity      `json:"activities"`
	Materials     []*Material      `json:"materials"`
	Answers       []*Answer        `json:"answers"`
	CiteChecks    []*CiteCheck     `json:"cite_checks"`
	Templates     []*Template      `json:"templates"` // 仅自定义模板；内置模板在代码中
	ModelProfiles []*ModelProfile  `json:"model_profiles"`
	SearchLogs    []*SearchLog     `json:"search_logs"`
	AgentLogs     []*AgentLog      `json:"agent_logs,omitempty"`
	Usage         []*UsageRow      `json:"usage,omitempty"`
	UsageCalls    []*UsageCall     `json:"usage_calls,omitempty"`
	ReadCards     []*ReadCard      `json:"read_cards,omitempty"`
	Skills        []*AgentSkill    `json:"skills,omitempty"`
	ResearchJobs  []*ResearchJob   `json:"research_jobs,omitempty"`
	Drafts        []*WDraft        `json:"drafts,omitempty"`
	Contracts     []*PaperContract `json:"contracts,omitempty"`
	AgentTasks    []*AgentTask     `json:"agent_tasks,omitempty"`
	Settings      Settings         `json:"settings"`
	NextUserID    int              `json:"next_user_id"`
	NextActID     int              `json:"next_act_id"`
}

type Store struct {
	mu     sync.RWMutex
	dir    string
	db     *DB
	cmu    sync.Mutex
	ccache map[string][]Chunk
}

func OpenStore(dir string) (*Store, error) {
	for _, d := range []string{dir, filepath.Join(dir, "files"), filepath.Join(dir, "chunks")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	s := &Store{dir: dir, db: &DB{NextUserID: 1, NextActID: 1, Settings: Settings{OnlineCheck: true}}, ccache: map[string][]Chunk{}}
	b, err := os.ReadFile(filepath.Join(dir, "data.json"))
	if err == nil {
		if err := json.Unmarshal(b, s.db); err != nil {
			return nil, errors.New("数据文件损坏：" + err.Error())
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Update 在写锁内修改数据并持久化。fn 返回错误或保存失败时回滚到修改前的状态（事务语义）。
func (s *Store) Update(fn func(db *DB) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := json.Marshal(s.db)
	if err != nil {
		return err
	}
	rollback := func() {
		fresh := &DB{}
		if json.Unmarshal(snap, fresh) == nil {
			s.db = fresh
		}
	}
	if err := fn(s.db); err != nil {
		rollback()
		return err
	}
	if err := s.saveLocked(); err != nil {
		rollback()
		return err
	}
	return nil
}

func (s *Store) View(fn func(db *DB)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(s.db)
}

func (s *Store) saveLocked() error {
	b, err := json.Marshal(s.db)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, "data.json"), b)
}

func atomicWrite(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

func (s *Store) FilePath(m *Material) string {
	return filepath.Join(s.dir, "files", m.ID+"."+m.Ftype)
}

func (s *Store) SaveChunks(mid string, cs []Chunk) error {
	b, err := json.Marshal(cs)
	if err != nil {
		return err
	}
	s.cmu.Lock()
	defer s.cmu.Unlock()
	if err := atomicWrite(filepath.Join(s.dir, "chunks", mid+".json"), b); err != nil {
		return err
	}
	s.ccache[mid] = cs
	return nil
}

func (s *Store) Chunks(mid string) []Chunk {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	if cs, ok := s.ccache[mid]; ok {
		return cs
	}
	b, err := os.ReadFile(filepath.Join(s.dir, "chunks", mid+".json"))
	if err != nil {
		return nil
	}
	var cs []Chunk
	if json.Unmarshal(b, &cs) != nil {
		return nil
	}
	s.ccache[mid] = cs
	return cs
}

func (s *Store) DeleteChunksAndFile(m *Material) error {
	s.cmu.Lock()
	delete(s.ccache, m.ID)
	s.cmu.Unlock()
	e1 := os.Remove(filepath.Join(s.dir, "chunks", m.ID+".json"))
	e2 := os.Remove(s.FilePath(m))
	if e1 != nil && !os.IsNotExist(e1) {
		return e1
	}
	if e2 != nil && !os.IsNotExist(e2) {
		return e2
	}
	return nil
}

// ---- 查询辅助（调用方需持有锁） ----

func (db *DB) User(id int) *User {
	for _, u := range db.Users {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (db *DB) Project(id string) *Project {
	for _, p := range db.Projects {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (db *DB) Material(id string) *Material {
	for _, m := range db.Materials {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (db *DB) Log(pid string, uid int, action, detail string) {
	db.Activities = append(db.Activities, &Activity{ID: db.NextActID, ProjectID: pid, UserID: uid, Action: action, Detail: detail, At: time.Now()})
	db.NextActID++
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
