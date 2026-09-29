package main

// 模型接口：任意 OpenAI 兼容的 /chat/completions。未配置时明确返回“未接入”，不伪造回答。

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var ErrLLMUnavailable = errors.New("模型未接入：请在“设置 → AI 模型”中添加自己的模型配置，或请管理员设置团队默认模型")

type LLMError struct{ Msg string }

func (e *LLMError) Error() string { return e.Msg }

// ModelCfg 是一次调用实际使用的模型（已解密密钥）。协议：openai（OpenAI 兼容）/ anthropic。
type ModelCfg struct {
	ID          string
	Name        string
	Source      string // personal / project / team
	Owner       string
	Protocol    string
	BaseURL     string
	Model       string
	Key         string
	Temperature float64
	Timeout     int
	Vision      bool // 能看图片
	VisionOK    bool // 能看图片且看图自检通过

	// 智能分配：同一个接口和密钥下的“难题模型”（可选）
	DailyModel     string // 日常模型（Model 在切换到难题模型后会变）
	StrongModel    string
	StrongOK       bool // 难题模型已通过自检（或未自检的团队模型）
	PriceIn        float64
	PriceOut       float64 // 元 / 百万 tokens
	StrongPriceIn  float64
	StrongPriceOut float64
	Tier           string // daily / strong
	Route          string // 为什么用这个模型
	Task           string
	UserID         int
	Blocked        string // 不为空时拒绝调用（例如团队额度用完）
	rec            func(ModelCfg, Usage)
}

// Usage 是一次模型调用的用量。
type Usage struct {
	In, Out   int
	Ms        int
	OK        bool
	Err       string
	Estimated bool
	Invalid   bool // 只用于标记上一次调用“返回了内容但没通过校验”
}

func (c ModelCfg) cost(in, out int) float64 {
	return float64(in)*c.PriceIn/1e6 + float64(out)*c.PriceOut/1e6
}

func (c ModelCfg) Configured() bool { return c.BaseURL != "" && c.Key != "" && c.Model != "" }

func (c ModelCfg) Label() string {
	if !c.Configured() {
		return "未接入模型"
	}
	src := map[string]string{"personal": "个人", "project": "项目指定", "team": "团队默认"}[c.Source]
	s := c.Name
	if s == "" {
		s = c.Model
	} else if c.Model != "" && !strings.Contains(s, c.Model) {
		s += "（" + c.Model + "）"
	}
	if src != "" && !strings.HasPrefix(s, src) {
		s += " · " + src
	}
	if c.Owner != "" && c.Source != "team" {
		s += " · " + c.Owner
	}
	return s
}

func llmConfigured(s Settings) bool {
	return s.LLMBaseURL != "" && s.LLMKey != "" && s.LLMModel != ""
}

// chatJSON 可在测试中替换。
var chatJSON = realChatJSON

func realChatJSON(c ModelCfg, system, user string) (map[string]any, error) {
	out, err := chatRaw(c, system, []chatMsg{{Role: "user", Text: user}}, 4096)
	if err != nil {
		return nil, err
	}
	m, err := parseJSONLoose(out)
	if err != nil {
		markInvalid(c, "未按格式输出")
	}
	return m, err
}

// FormatError 模型没有按要求输出 JSON；Raw 是原始回复，调用方可以据此重试或直接当作文字回答。
type FormatError struct{ Raw string }

func (e *FormatError) Error() string { return "模型没有按要求的格式回复" }

// chatConv 多轮对话、要求 JSON 输出（本机助手使用）。可在测试中替换。
var chatConv = realChatConv

func realChatConv(c ModelCfg, system string, msgs []chatMsg) (map[string]any, error) {
	out, err := chatRaw(c, system, msgs, 8192)
	if err != nil {
		return nil, err
	}
	m, err := parseJSONLoose(out)
	if err != nil {
		markInvalid(c, "未按格式输出")
		return nil, &FormatError{Raw: out}
	}
	return m, nil
}

// chatVision 发送图片，返回纯文本（图片转 LaTeX 使用）。可在测试中替换。
var chatVision = func(c ModelCfg, system, text string, imgs []chatImage) (string, error) {
	return chatRaw(c, system, []chatMsg{{Role: "user", Text: text, Images: imgs}}, 8192)
}

type chatImage struct {
	Mime string
	Data []byte
}

type chatMsg struct {
	Role   string // user / assistant
	Text   string
	Images []chatImage
}

// chatRaw 调用模型并返回文本。支持 OpenAI 兼容接口与 Anthropic 原生接口，支持图片输入。
var chatRaw = realChatRaw

func realChatRaw(c ModelCfg, system string, msgs []chatMsg, maxTokens int) (string, error) {
	if !c.Configured() {
		return "", ErrLLMUnavailable
	}
	if c.Blocked != "" {
		return "", &LLMError{c.Blocked}
	}
	start := time.Now()
	var u Usage
	defer func() {
		u.Ms = int(time.Since(start).Milliseconds())
		if u.In == 0 && u.Out == 0 {
			// 服务商没有返回用量时粗略估算（中文约 1.5 字/token，英文约 4 字母/token，这里统一按 3 字节/token）
			n := len(system)
			for _, m := range msgs {
				n += len(m.Text) + 800*len(m.Images)
			}
			u.In, u.Estimated = n/3, true
		}
		if c.rec != nil {
			c.rec(c, u)
		}
	}()
	timeout := time.Duration(c.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	temp := c.Temperature
	if temp < 0 || temp > 1.5 {
		temp = 0.1
	}
	base := strings.TrimRight(c.BaseURL, "/")
	var req *http.Request
	if c.Protocol == "anthropic" {
		var ms []map[string]any
		for _, m := range msgs {
			var parts []map[string]any
			for _, im := range m.Images {
				parts = append(parts, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": im.Mime, "data": base64.StdEncoding.EncodeToString(im.Data)}})
			}
			parts = append(parts, map[string]any{"type": "text", "text": m.Text})
			ms = append(ms, map[string]any{"role": m.Role, "content": parts})
		}
		body, _ := json.Marshal(map[string]any{"model": c.Model, "max_tokens": maxTokens, "temperature": temp, "system": system, "messages": ms})
		url := base + "/v1/messages"
		if strings.HasSuffix(base, "/v1") {
			url = base + "/messages"
		}
		req, _ = http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("x-api-key", c.Key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		ms := []map[string]any{{"role": "system", "content": system}}
		for _, m := range msgs {
			if len(m.Images) == 0 {
				ms = append(ms, map[string]any{"role": m.Role, "content": m.Text})
				continue
			}
			parts := []map[string]any{{"type": "text", "text": m.Text}}
			for _, im := range m.Images {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + im.Mime + ";base64," + base64.StdEncoding.EncodeToString(im.Data)}})
			}
			ms = append(ms, map[string]any{"role": m.Role, "content": parts})
		}
		body, _ := json.Marshal(map[string]any{"model": c.Model, "temperature": temp, "messages": ms})
		req, _ = http.NewRequest("POST", base+"/chat/completions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			u.Err = "超时"
			return "", &LLMError{"模型请求超时，请稍后重试"}
		}
		u.Err = "无法连接"
		return "", &LLMError{"无法连接模型服务，请检查网络或接口地址"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		msg := "模型服务返回错误状态 " + itoa(resp.StatusCode)
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			msg += "（密钥无效或无权限）"
		} else if resp.StatusCode == 402 || resp.StatusCode == 429 {
			msg += "（余额不足或请求过于频繁）"
		} else if resp.StatusCode == 404 {
			msg += "（接口地址或模型名称可能填错了）"
		} else if resp.StatusCode == 400 && hasImages(msgs) {
			msg += "（这个模型可能不支持图片）"
		}
		u.Err = "状态 " + itoa(resp.StatusCode)
		return "", &LLMError{msg}
	}
	var content string
	if c.Protocol == "anthropic" {
		var out struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				In  int `json:"input_tokens"`
				Out int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Content) == 0 {
			u.Err = "返回格式无法识别"
			return "", &LLMError{"模型返回格式无法识别"}
		}
		for _, p := range out.Content {
			content += p.Text
		}
		u.In, u.Out = out.Usage.In, out.Usage.Out
	} else {
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				In  int `json:"prompt_tokens"`
				Out int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Choices) == 0 {
			u.Err = "返回格式无法识别"
			return "", &LLMError{"模型返回格式无法识别"}
		}
		content = out.Choices[0].Message.Content
		u.In, u.Out = out.Usage.In, out.Usage.Out
	}
	u.OK = true
	return content, nil
}

func hasImages(msgs []chatMsg) bool {
	for _, m := range msgs {
		if len(m.Images) > 0 {
			return true
		}
	}
	return false
}

var reFence = regexp.MustCompile("(?s)^```(?:json)?\\s*|\\s*```$")
var reObj = regexp.MustCompile(`(?s)\{.*\}`)

func parseJSONLoose(content string) (map[string]any, error) {
	content = reFence.ReplaceAllString(strings.TrimSpace(content), "")
	var m map[string]any
	if json.Unmarshal([]byte(content), &m) == nil {
		return m, nil
	}
	if s := reObj.FindString(content); s != "" && json.Unmarshal([]byte(s), &m) == nil {
		return m, nil
	}
	return nil, &LLMError{"模型未按要求输出 JSON，无法校验引用，已不展示该回答"}
}

// ---- JSON 取值辅助 ----
func str(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func list(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

func obj(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func strList(v any) []string {
	var out []string
	for _, x := range list(v) {
		if s := str(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}
