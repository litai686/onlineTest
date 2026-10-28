package httpapi

import (
	"encoding/json"
	"net/http"

	"onlineTest/internal/domain"
)

func jsonUnmarshalAny(raw string, out any) error {
	if raw == "" {
		raw = "{}"
	}
	return json.Unmarshal([]byte(raw), out)
}

func (a *App) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type counts struct {
		Students, Papers, Published, AttemptsToday, PendingJobs, AICallsToday, TokensToday int64
	}
	var c counts
	db := a.Store.DB().WithContext(ctx)
	db.Model(&domain.User{}).Where("role = 'student'").Count(&c.Students)
	db.Model(&domain.Paper{}).Count(&c.Papers)
	db.Model(&domain.Paper{}).Where("status = 'published'").Count(&c.Published)
	db.Model(&domain.Attempt{}).Where("started_at >= datetime('now', '-1 day')").Count(&c.AttemptsToday)
	db.Model(&domain.Job{}).Where("status IN ('pending','running')").Count(&c.PendingJobs)
	db.Model(&domain.AICall{}).Where("created_at >= datetime('now', '-1 day')").Count(&c.AICallsToday)
	db.Model(&domain.AICall{}).Where("created_at >= datetime('now', '-1 day')").
		Select("COALESCE(SUM(prompt_tokens + completion_tokens), 0)").Scan(&c.TokensToday)

	writeJSON(w, http.StatusOK, map[string]any{
		"students":        c.Students,
		"papers":          c.Papers,
		"published":       c.Published,
		"attempts_today":  c.AttemptsToday,
		"pending_jobs":    c.PendingJobs,
		"ai_calls_today":  c.AICallsToday,
		"ai_tokens_today": c.TokensToday,
	})
}
