package httpapi

import (
	"net/http"
	"strings"
	"time"

	"onlineTest/internal/service"
	"onlineTest/internal/store"
)

func (a *App) teacherAttempts(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	paperID, _ := intQuery(r, "paper_id")
	studentID, _ := intQuery(r, "student_id")
	items, total, err := a.Store.ListAttempts(r.Context(), store.AttemptFilter{
		PaperID:   int64(paperID),
		StudentID: int64(studentID),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		Page:      page,
		Size:      size,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		att := &items[i]
		row := map[string]any{
			"id":           att.ID,
			"paper_id":     att.PaperID,
			"status":       att.Status,
			"score":        att.Score,
			"full_score":   att.FullScore,
			"submitted_at": att.SubmittedAt,
			"duration_sec": att.DurationSec,
		}
		if att.Paper != nil {
			row["paper_title"] = att.Paper.Title
		}
		if att.Student != nil {
			row["student_id"] = att.Student.ID
			row["student_name"] = att.Student.DisplayName
			row["class_name"] = att.Student.ClassName
		}
		out = append(out, row)
	}
	data := map[string]any{"items": out, "total": total, "page": page, "page_size": size}
	if paperID > 0 {
		submitted, graded, avg, err := a.Store.AttemptSummary(r.Context(), int64(paperID))
		if err == nil {
			data["summary"] = map[string]any{"submitted": submitted, "graded": graded, "avg_score": avg}
		}
	}
	writeJSON(w, http.StatusOK, data)
}

func (a *App) teacherResult(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	attempt, err := a.Store.GetAttempt(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("答卷不存在"))
		return
	}
	data, err := a.resultData(r, attempt, true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	answers, _ := a.Store.AttemptAnswers(r.Context(), attempt.ID)
	var manual []map[string]any
	for _, aa := range answers {
		if aa.GradingMode == "manual" {
			manual = append(manual, map[string]any{
				"question_id": aa.QuestionID,
				"answer":      aa.Answer,
				"max_score":   aa.MaxScore,
				"score":       aa.Score,
			})
		}
	}
	data["manual_review"] = manual
	writeJSON(w, http.StatusOK, data)
}

type manualScoreRequest struct {
	Score    float64 `json:"score"`
	Feedback string  `json:"feedback"`
}

func (a *App) manualScore(w http.ResponseWriter, r *http.Request) {
	attemptID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	questionID, err := pathID(r, "questionId")
	if err != nil {
		writeError(w, r, err)
		return
	}
	attempt, err := a.Store.GetAttempt(r.Context(), attemptID)
	if err != nil {
		writeError(w, r, notFound("答卷不存在"))
		return
	}
	aa, err := a.Store.GetAnswer(r.Context(), attempt.ID, questionID)
	if err != nil {
		writeError(w, r, notFound("答题记录不存在"))
		return
	}
	var req manualScoreRequest
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	if req.Score < 0 || req.Score > aa.MaxScore {
		writeError(w, r, badRequest("分数必须在 0 到满分之间"))
		return
	}
	correct := req.Score >= aa.MaxScore-1e-6
	aa.Score = &req.Score
	aa.IsCorrect = &correct
	aa.GradingMode = "manual"
	aa.Feedback = strings.TrimSpace(req.Feedback)
	if err := a.Store.UpsertSingleAnswer(r.Context(), aa); err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.Svc.FinalizeAttempt(r.Context(), attempt.ID, "", false); err != nil {
		writeError(w, r, err)
		return
	}
	updated, _ := a.Store.GetAttempt(r.Context(), attempt.ID)
	writeJSON(w, http.StatusOK, map[string]any{"attempt": attemptDTO(updated)})
}

func (a *App) regradeAttempt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	attempt, err := a.Store.GetAttempt(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("答卷不存在"))
		return
	}
	if err := a.Store.ResetAnswersForRegrade(r.Context(), attempt.ID, attempt.PaperID); err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.Store.UpdateAttempt(r.Context(), attempt.ID, map[string]any{"status": "grading"}); err != nil {
		writeError(w, r, err)
		return
	}
	job, err := a.Svc.EnqueueJob(r.Context(), "attempt_grade", "attempt", attempt.ID, service.GradePayload{
		AttemptID: attempt.ID,
	}, userFromRequest(r).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "status": "grading"})
}

func (a *App) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := a.Store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("任务不存在"))
		return
	}
	u := userFromRequest(r)
	if u.Role == "student" {
		if job.RefType != "attempt" {
			writeError(w, r, forbidden("无权查看该任务"))
			return
		}
		attempt, err := a.Store.GetAttempt(r.Context(), job.RefID)
		if err != nil || attempt.StudentID != u.ID {
			writeError(w, r, forbidden("无权查看该任务"))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          job.ID,
		"type":        job.Type,
		"ref_type":    job.RefType,
		"ref_id":      job.RefID,
		"status":      job.Status,
		"progress":    job.Progress,
		"message":     job.Message,
		"attempts":    job.Attempts,
		"last_error":  job.LastError,
		"created_at":  job.CreatedAt,
		"started_at":  job.StartedAt,
		"finished_at": job.FinishedAt,
	})
}

func (a *App) listJobs(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	items, total, err := a.Store.ListJobs(r.Context(), store.JobFilter{
		Type:   strings.TrimSpace(r.URL.Query().Get("type")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Page:   page,
		Size:   size,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
}

func (a *App) retryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	old, err := a.Store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("任务不存在"))
		return
	}
	if old.Status != "failed" && old.Status != "canceled" {
		writeError(w, r, invalidState("只有失败或已取消的任务可以重试"))
		return
	}
	var payload any
	if err := jsonUnmarshalAny(old.Payload, &payload); err != nil {
		writeError(w, r, err)
		return
	}
	if old.Type == "paper_parse" {
		_ = a.Store.UpdatePaperParseStatus(r.Context(), old.RefID, "queued", "")
	}
	job, err := a.Svc.EnqueueJob(r.Context(), old.Type, old.RefType, old.RefID, payload, userFromRequest(r).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID})
}

func (a *App) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := a.Store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("任务不存在"))
		return
	}
	if job.Status != "pending" && job.Status != "running" {
		writeError(w, r, invalidState("任务已结束"))
		return
	}
	now := time.Now().UTC()
	if err := a.Store.UpdateJob(r.Context(), id, map[string]any{
		"status":      "canceled",
		"finished_at": now,
		"message":     "已取消",
	}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "canceled"})
}
