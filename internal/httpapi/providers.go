package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
	"onlineTest/internal/security"
	"onlineTest/internal/store"
)

func providerDTO(p *domain.AIProvider) map[string]any {
	return map[string]any{
		"id":              p.ID,
		"name":            p.Name,
		"base_url":        p.BaseURL,
		"model":           p.Model,
		"api_key_hint":    p.APIKeyHint,
		"supports_vision": p.SupportsVision,
		"supports_json":   p.SupportsJSON,
		"temperature":     p.Temperature,
		"max_tokens":      p.MaxTokens,
		"timeout_sec":     p.TimeoutSec,
		"concurrency":     p.Concurrency,
		"thinking_mode":   p.ThinkingMode,
		"extra_headers":   jsonAny(p.ExtraHeaders),
		"extra_body":      jsonAny(p.ExtraBody),
		"note":            p.Note,
		"status":          p.Status,
		"created_at":      p.CreatedAt,
	}
}

func jsonAny(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return map[string]any{}
	}
	return v
}

func (a *App) roleBindings(w http.ResponseWriter, r *http.Request) map[string]any {
	ctx := r.Context()
	return map[string]any{
		"vision":  a.Store.GetSettingDefault(ctx, "ai.role.vision", ""),
		"grading": a.Store.GetSettingDefault(ctx, "ai.role.grading", ""),
		"chat":    a.Store.GetSettingDefault(ctx, "ai.role.chat", ""),
		"default": a.Store.GetSettingDefault(ctx, "ai.default", ""),
	}
}

func (a *App) listProviders(w http.ResponseWriter, r *http.Request) {
	items, err := a.Store.ListProviders(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, providerDTO(&items[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "roles": a.roleBindings(w, r)})
}

type providerRequest struct {
	Name           string         `json:"name"`
	BaseURL        string         `json:"base_url"`
	APIKey         *string        `json:"api_key"`
	Model          string         `json:"model"`
	SupportsVision *bool          `json:"supports_vision"`
	SupportsJSON   *bool          `json:"supports_json"`
	Temperature    *float64       `json:"temperature"`
	MaxTokens      *int           `json:"max_tokens"`
	TimeoutSec     *int           `json:"timeout_sec"`
	Concurrency    *int           `json:"concurrency"`
	ThinkingMode   *string        `json:"thinking_mode"`
	ExtraHeaders   map[string]any `json:"extra_headers"`
	ExtraBody      map[string]any `json:"extra_body"`
	Note           string         `json:"note"`
	Status         *int           `json:"status"`
	SetRoles       []string       `json:"set_roles"`
}

func (a *App) createProvider(w http.ResponseWriter, r *http.Request) {
	var req providerRequest
	if err := decodeJSON(w, r, &req, 256<<10); err != nil {
		writeError(w, r, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	base := strings.TrimSpace(req.BaseURL)
	model := strings.TrimSpace(req.Model)
	if name == "" || model == "" {
		writeError(w, r, badRequest("名称与模型名不能为空"))
		return
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		writeError(w, r, badRequest("Base URL 必须以 http:// 或 https:// 开头"))
		return
	}
	p := domain.AIProvider{
		Name:           name,
		BaseURL:        strings.TrimRight(base, "/"),
		Model:          model,
		SupportsVision: req.SupportsVision != nil && *req.SupportsVision,
		SupportsJSON:   req.SupportsJSON == nil || *req.SupportsJSON,
		Temperature:    0.2,
		MaxTokens:      4096,
		TimeoutSec:     120,
		Concurrency:    2,
		ExtraHeaders:   "{}",
		ExtraBody:      "{}",
		Status:         1,
		Note:           strings.TrimSpace(req.Note),
	}
	if req.Temperature != nil {
		p.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		p.MaxTokens = *req.MaxTokens
	}
	if req.TimeoutSec != nil && *req.TimeoutSec > 0 {
		p.TimeoutSec = *req.TimeoutSec
	}
	if req.Concurrency != nil && *req.Concurrency > 0 {
		p.Concurrency = *req.Concurrency
	}
	if req.ThinkingMode != nil {
		mode := strings.TrimSpace(*req.ThinkingMode)
		if mode != "" && mode != "none" && mode != "low" && mode != "high" && mode != "max" {
			writeError(w, r, badRequest("思考模式无效"))
			return
		}
		p.ThinkingMode = mode
	}
	if req.Status != nil {
		p.Status = *req.Status
	}
	if req.ExtraHeaders != nil {
		b, _ := json.Marshal(req.ExtraHeaders)
		p.ExtraHeaders = string(b)
	}
	if req.ExtraBody != nil {
		b, _ := json.Marshal(req.ExtraBody)
		p.ExtraBody = string(b)
	}
	if req.APIKey != nil && *req.APIKey != "" {
		enc, err := a.Cipher.Encrypt(*req.APIKey)
		if err != nil {
			writeError(w, r, err)
			return
		}
		p.APIKeyCipher = enc
		p.APIKeyHint = security.MaskKey(*req.APIKey)
	}
	if err := a.Store.CreateProvider(r.Context(), &p); err != nil {
		writeError(w, r, err)
		return
	}
	a.applyRoles(w, r, p.ID, req.SetRoles)
	writeJSON(w, http.StatusCreated, providerDTO(&p))
}

func (a *App) applyRoles(w http.ResponseWriter, r *http.Request, id int64, roles []string) {
	for _, role := range roles {
		key := ""
		switch role {
		case "default":
			key = "ai.default"
		case "vision", "grading", "chat":
			key = "ai.role." + role
		default:
			continue
		}
		if err := a.Store.SetSetting(r.Context(), key, itoa64(id)); err != nil {
			writeError(w, r, err)
			return
		}
	}
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

func (a *App) updateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Store.GetProvider(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("模型配置不存在"))
		return
	}
	_ = p
	var req providerRequest
	if err := decodeJSON(w, r, &req, 256<<10); err != nil {
		writeError(w, r, err)
		return
	}
	fields := map[string]any{}
	if v := strings.TrimSpace(req.Name); v != "" {
		fields["name"] = v
	}
	if v := strings.TrimSpace(req.BaseURL); v != "" {
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			writeError(w, r, badRequest("Base URL 格式不正确"))
			return
		}
		fields["base_url"] = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(req.Model); v != "" {
		fields["model"] = v
	}
	if req.SupportsVision != nil {
		fields["supports_vision"] = *req.SupportsVision
	}
	if req.SupportsJSON != nil {
		fields["supports_json"] = *req.SupportsJSON
	}
	if req.Temperature != nil {
		fields["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		fields["max_tokens"] = *req.MaxTokens
	}
	if req.TimeoutSec != nil && *req.TimeoutSec > 0 {
		fields["timeout_sec"] = *req.TimeoutSec
	}
	if req.Concurrency != nil && *req.Concurrency > 0 {
		fields["concurrency"] = *req.Concurrency
	}
	if req.ThinkingMode != nil {
		mode := strings.TrimSpace(*req.ThinkingMode)
		if mode != "" && mode != "none" && mode != "low" && mode != "high" && mode != "max" {
			writeError(w, r, badRequest("思考模式无效"))
			return
		}
		fields["thinking_mode"] = mode
	}
	if req.Status != nil {
		fields["status"] = *req.Status
	}
	if req.ExtraHeaders != nil {
		b, _ := json.Marshal(req.ExtraHeaders)
		fields["extra_headers"] = string(b)
	}
	if req.ExtraBody != nil {
		b, _ := json.Marshal(req.ExtraBody)
		fields["extra_body"] = string(b)
	}
	if req.APIKey != nil {
		if *req.APIKey == "" {
			fields["api_key_cipher"] = ""
			fields["api_key_hint"] = ""
		} else {
			enc, err := a.Cipher.Encrypt(*req.APIKey)
			if err != nil {
				writeError(w, r, err)
				return
			}
			fields["api_key_cipher"] = enc
			fields["api_key_hint"] = security.MaskKey(*req.APIKey)
		}
	}
	if err := a.Store.UpdateProvider(r.Context(), id, fields); err != nil {
		writeError(w, r, err)
		return
	}
	a.applyRoles(w, r, id, req.SetRoles)
	updated, _ := a.Store.GetProvider(r.Context(), id)
	writeJSON(w, http.StatusOK, providerDTO(updated))
}

func (a *App) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.Store.DeleteProvider(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type providerTestRequest struct {
	ProbeVision bool `json:"probe_vision"`
}

func (a *App) testProvider(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Store.GetProvider(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("模型配置不存在"))
		return
	}
	var req providerTestRequest
	_ = decodeJSON(w, r, &req, 16<<10)
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res, err := a.Svc.AI.Chat(ctx, p, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "user", Content: "请回复两个字：可用"},
		},
	}, "test", "")
	if err != nil {
		writeError(w, r, aiError("连通性测试失败："+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"latency_ms": time.Since(start).Milliseconds(),
		"model":      p.Model,
		"reply":      truncateStr(res.Content, 60),
		"vision_ok":  nil,
	})
}

func (a *App) setRoles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role       string `json:"role"`
		ProviderID int64  `json:"provider_id"`
	}
	if err := decodeJSON(w, r, &req, 16<<10); err != nil {
		writeError(w, r, err)
		return
	}
	key := ""
	switch req.Role {
	case "default":
		key = "ai.default"
	case "vision", "grading", "chat":
		key = "ai.role." + req.Role
	default:
		writeError(w, r, badRequest("无效的角色"))
		return
	}
	value := ""
	if req.ProviderID > 0 {
		if _, err := a.Store.GetProvider(r.Context(), req.ProviderID); err != nil {
			writeError(w, r, notFound("模型配置不存在"))
			return
		}
		value = itoa64(req.ProviderID)
	}
	if err := a.Store.SetSetting(r.Context(), key, value); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a.roleBindings(w, r))
}

func (a *App) listAICalls(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	items, total, err := a.Store.ListAICalls(r.Context(), store.AICallFilter{
		Purpose: r.URL.Query().Get("purpose"),
		Status:  r.URL.Query().Get("status"),
		Page:    page,
		Size:    size,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
