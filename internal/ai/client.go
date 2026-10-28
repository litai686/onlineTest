package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"onlineTest/internal/domain"
	"onlineTest/internal/security"
	"onlineTest/internal/store"

	"github.com/google/uuid"
)

var (
	ErrNoProvider = errors.New("no ai provider configured")
	ErrBadJSON    = errors.New("ai response is not valid json")
)

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    *float64        `json:"temperature,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Stream         bool            `json:"stream"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type ChatResult struct {
	Content string
	Usage   Usage
}

type Client struct {
	store  *store.Store
	cipher *security.Cipher
	http   *http.Client
	mu     sync.Mutex
	sems   map[int64]chan struct{}
}

func NewClient(st *store.Store, cipher *security.Cipher) *Client {
	return &Client{
		store:  st,
		cipher: cipher,
		http:   &http.Client{},
		sems:   make(map[int64]chan struct{}),
	}
}

func (c *Client) ProviderFor(ctx context.Context, role string) (*domain.AIProvider, error) {
	var idStr string
	if role != "" && role != "default" {
		idStr = c.store.GetSettingDefault(ctx, "ai.role."+role, "")
	}
	if idStr == "" {
		idStr = c.store.GetSettingDefault(ctx, "ai.default", "")
	}
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil {
			p, err := c.store.GetProvider(ctx, id)
			if err == nil && p.Status == 1 {
				return p, nil
			}
		}
	}
	items, err := c.store.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Status == 1 {
			return &items[i], nil
		}
	}
	return nil, ErrNoProvider
}

func (c *Client) semaphore(p *domain.AIProvider) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	sem, ok := c.sems[p.ID]
	if !ok {
		n := p.Concurrency
		if n <= 0 {
			n = 1
		}
		sem = make(chan struct{}, n)
		c.sems[p.ID] = sem
	}
	return sem
}

func (c *Client) Chat(ctx context.Context, p *domain.AIProvider, req ChatRequest, purpose, jobID string) (*ChatResult, error) {
	sem := c.semaphore(p)
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	key, err := c.cipher.Decrypt(p.APIKeyCipher)
	if err != nil {
		return nil, fmt.Errorf("decrypt api key: %w", err)
	}

	body := map[string]any{
		"model":    p.Model,
		"messages": req.Messages,
		"stream":   false,
	}
	temp := p.Temperature
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	body["temperature"] = temp
	if req.ResponseFormat != nil {
		body["response_format"] = req.ResponseFormat
	}
	if p.ThinkingMode != "" {
		body["reasoning_effort"] = p.ThinkingMode
	}
	if strings.TrimSpace(p.ExtraBody) != "" && p.ExtraBody != "{}" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(p.ExtraBody), &extra); err == nil {
			for k, v := range extra {
				body[k] = v
			}
		}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(p.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	timeout := time.Duration(p.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		cctx, cancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		result, status, err := c.doChat(cctx, endpoint, p, key, raw)
		cancel()
		latency := time.Since(start).Milliseconds()

		call := domain.AICall{
			ProviderID:    &p.ID,
			ProviderName:  p.Name,
			Model:         p.Model,
			Purpose:       purpose,
			JobID:         jobID,
			PromptVersion: promptVersion,
			RequestDigest: digestRequest(req),
			LatencyMS:     int(latency),
			HTTPStatus:    status,
		}
		if err != nil {
			lastErr = err
			call.Status = "failed"
			call.Error = truncate(err.Error(), 500)
			if attempt < 2 && shouldRetry(status, err) {
				call.Status = "retry"
			}
			c.logCall(call)
			if !shouldRetry(status, err) {
				return nil, err
			}
			continue
		}

		call.Status = "ok"
		call.PromptTokens = result.Usage.PromptTokens
		call.CompletionTokens = result.Usage.CompletionTokens
		c.logCall(call)
		return result, nil
	}
	if lastErr == nil {
		lastErr = errors.New("ai request failed")
	}
	return nil, lastErr
}

func (c *Client) doChat(ctx context.Context, endpoint string, p *domain.AIProvider, key string, raw []byte) (*ChatResult, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "onlineTest/0.1")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if u, err := url.Parse(endpoint); err == nil && strings.Contains(strings.ToLower(u.Host), "opencode.ai") {
		req.Header.Set("x-opencode-session", openCodeSession(p))
	}
	if strings.TrimSpace(p.ExtraHeaders) != "" && p.ExtraHeaders != "{}" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(p.ExtraHeaders), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("请求 AI 服务失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读取 AI 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("AI 服务返回 %d: %s", resp.StatusCode, truncate(string(data), 300))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("解析 AI 响应失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, resp.StatusCode, fmt.Errorf("AI 服务错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, resp.StatusCode, fmt.Errorf("AI 未返回内容")
	}
	return &ChatResult{Content: parsed.Choices[0].Message.Content, Usage: parsed.Usage}, resp.StatusCode, nil
}

func (c *Client) logCall(call domain.AICall) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.store.CreateAICall(ctx, &call); err != nil {
		// logging must never break the request
		_ = err
	}
}

func shouldRetry(status int, err error) bool {
	if status == 429 || status >= 500 {
		return true
	}
	if status == 0 && err != nil {
		return true
	}
	return false
}

func digestRequest(req ChatRequest) string {
	h := sha256.New()
	images := 0
	for _, m := range req.Messages {
		switch content := m.Content.(type) {
		case string:
			h.Write([]byte(content))
		case []ContentPart:
			for _, p := range content {
				if p.Type == "text" {
					h.Write([]byte(p.Text))
				}
				if p.ImageURL != nil {
					images++
				}
			}
		}
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return sum[:16] + "/img:" + strconv.Itoa(images)
}

func openCodeSession(p *domain.AIProvider) string {
	name := fmt.Sprintf("onlineTest-provider-%d-%s", p.ID, p.Model)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func ExtractJSON(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	start := -1
	for i, r := range s {
		if r == '{' || r == '[' {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, ErrBadJSON
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				candidate := []byte(s[start : i+1])
				var tmp any
				if err := json.Unmarshal(candidate, &tmp); err != nil {
					return nil, fmt.Errorf("%w: %v", ErrBadJSON, err)
				}
				return candidate, nil
			}
		}
	}
	return nil, ErrBadJSON
}
