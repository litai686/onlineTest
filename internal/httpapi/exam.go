package httpapi

import (
	"net/http"
	"strings"
	"time"

	"onlineTest/internal/domain"
	"onlineTest/internal/store"
)

func attemptDTO(a *domain.Attempt) map[string]any {
	return map[string]any{
		"id":               a.ID,
		"paper_id":         a.PaperID,
		"attempt_no":       a.AttemptNo,
		"status":           a.Status,
		"mode":             a.Mode,
		"score":            a.Score,
		"full_score":       a.FullScore,
		"objective_score":  a.ObjectiveScore,
		"subjective_score": a.SubjectiveScore,
		"correct_count":    a.CorrectCount,
		"grade_error":      a.GradeError,
		"started_at":       a.StartedAt,
		"submitted_at":     a.SubmittedAt,
		"graded_at":        a.GradedAt,
		"duration_sec":     a.DurationSec,
	}
}

func attemptQuestionDTO(q *domain.Question) map[string]any {
	options := q.Options
	if options == nil {
		options = domain.OptionList{}
	}
	out := map[string]any{
		"id":         q.ID,
		"seq":        q.Seq,
		"type":       q.Type,
		"stem":       q.Stem,
		"options":    options,
		"score":      q.Score,
		"difficulty": q.Difficulty,
	}
	if q.Type == "fill" {
		blanks := len(q.Answer)
		if blanks < 1 {
			blanks = 1
		}
		out["blanks"] = blanks
	}
	return out
}

func (a *App) listStudentPapers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFromRequest(r)
	page, size := pageParams(r)
	subjectID, _ := intQuery(r, "subject_id")
	rows, total, err := a.Store.ListPapers(ctx, store.PaperFilter{
		SubjectID: int64(subjectID),
		Status:    "published",
		Page:      page,
		Size:      size,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	attempts, _, err := a.Store.ListAttempts(ctx, store.AttemptFilter{StudentID: u.ID, Page: 1, Size: 200})
	if err != nil {
		writeError(w, r, err)
		return
	}
	latest := make(map[int64]*domain.Attempt, len(attempts))
	for i := range attempts {
		att := &attempts[i]
		if prev, ok := latest[att.PaperID]; !ok || att.ID > prev.ID {
			latest[att.PaperID] = att
		}
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":             row.ID,
			"title":          row.Title,
			"subject_name":   row.SubjectName,
			"grade":          row.Grade,
			"question_count": row.QuestionCount,
			"total_score":    row.TotalScore,
			"duration_min":   row.DurationMin,
			"published_at":   row.PublishedAt,
		}
		if att, ok := latest[row.ID]; ok {
			item["my_attempt"] = map[string]any{
				"id":           att.ID,
				"status":       att.Status,
				"score":        att.Score,
				"submitted_at": att.SubmittedAt,
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
}

func (a *App) startAttempt(w http.ResponseWriter, r *http.Request) {
	paperID, err := pathID(r, "paperId")
	if err != nil {
		writeError(w, r, err)
		return
	}
	ctx := r.Context()
	u := userFromRequest(r)
	paper, err := a.Store.GetPaperBasic(ctx, paperID)
	if err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	if paper.Status != "published" {
		writeError(w, r, invalidState("试卷未发布"))
		return
	}
	attempt, created, err := a.Store.StartAttempt(ctx, paperID, u.ID, paper.AllowRetry)
	if err != nil {
		if err == store.ErrRetryNotAllowed {
			writeError(w, r, invalidState("该试卷不允许重做"))
			return
		}
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	questions, err := a.Store.ListQuestions(ctx, paperID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	answers, err := a.Store.AttemptAnswers(ctx, attempt.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	qout := make([]map[string]any, 0, len(questions))
	for i := range questions {
		qout = append(qout, attemptQuestionDTO(&questions[i]))
	}
	writeJSON(w, status, map[string]any{
		"attempt":   attemptDTO(attempt),
		"paper":     map[string]any{"id": paper.ID, "title": paper.Title, "duration_min": paper.DurationMin, "total_score": paper.TotalScore},
		"questions": qout,
		"answers":   answers,
	})
}

func (a *App) getStudentAttempt(w http.ResponseWriter, r *http.Request) {
	attempt, err := a.attemptForStudent(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	questions, err := a.Store.ListQuestions(r.Context(), attempt.PaperID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	answers, err := a.Store.AttemptAnswers(r.Context(), attempt.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	qout := make([]map[string]any, 0, len(questions))
	for i := range questions {
		qout = append(qout, attemptQuestionDTO(&questions[i]))
	}
	paper := map[string]any{"id": attempt.PaperID, "title": "", "duration_min": 0, "total_score": attempt.FullScore}
	if attempt.Paper != nil {
		paper["title"] = attempt.Paper.Title
		paper["duration_min"] = attempt.Paper.DurationMin
		paper["total_score"] = attempt.Paper.TotalScore
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attempt":     attemptDTO(attempt),
		"paper":       paper,
		"questions":   qout,
		"answers":     answers,
		"server_time": time.Now().UTC(),
	})
}

func (a *App) attemptForStudent(r *http.Request) (*domain.Attempt, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return nil, err
	}
	attempt, err := a.Store.GetAttempt(r.Context(), id)
	if err != nil {
		return nil, notFound("答卷不存在")
	}
	if attempt.StudentID != userFromRequest(r).ID {
		return nil, forbidden("无权访问该答卷")
	}
	return attempt, nil
}

func (a *App) setAttemptMode(w http.ResponseWriter, r *http.Request) {
	attempt, err := a.attemptForStudent(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if attempt.Status != "in_progress" {
		writeError(w, r, invalidState("答卷已提交，不能切换模式"))
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(w, r, &req, 16<<10); err != nil {
		writeError(w, r, err)
		return
	}
	if req.Mode != "exam" && req.Mode != "practice" {
		writeError(w, r, badRequest("作答模式无效"))
		return
	}
	if err := a.Store.UpdateAttempt(r.Context(), attempt.ID, map[string]any{"mode": req.Mode}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": req.Mode})
}

func (a *App) checkPracticeQuestion(w http.ResponseWriter, r *http.Request) {
	attempt, err := a.attemptForStudent(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if attempt.Status != "in_progress" {
		writeError(w, r, invalidState("答卷已提交，不能查看答案"))
		return
	}
	if attempt.Mode != "practice" {
		writeError(w, r, invalidState("请先切换到边做边看答案模式"))
		return
	}
	qid, err := pathID(r, "qid")
	if err != nil {
		writeError(w, r, err)
		return
	}
	q, err := a.Store.GetQuestion(r.Context(), qid)
	if err != nil || q.PaperID != attempt.PaperID {
		writeError(w, r, notFound("题目不存在"))
		return
	}
	var req struct {
		Answer domain.StringList `json:"answer"`
	}
	if err := decodeJSON(w, r, &req, 2<<20); err != nil {
		writeError(w, r, err)
		return
	}
	total := 0
	answered := false
	for _, s := range req.Answer {
		total += len([]rune(s))
		if strings.TrimSpace(s) != "" {
			answered = true
		}
	}
	if total > 16000 {
		writeError(w, r, badRequest("单题答案过长（上限 16000 字）"))
		return
	}
	if !answered {
		writeError(w, r, badRequest("请先作答本题"))
		return
	}
	result, err := a.Svc.CheckQuestion(r.Context(), attempt.ID, *q, req.Answer)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"question_id":    q.ID,
		"type":           q.Type,
		"answer":         req.Answer,
		"correct_answer": q.Answer,
		"is_correct":     result.IsCorrect,
		"graded":         result.IsCorrect != nil,
		"score":          result.Score,
		"max_score":      result.MaxScore,
		"analysis":       q.Analysis,
		"feedback":       result.Feedback,
	})
}

type saveAnswersRequest struct {
	Answers []struct {
		QuestionID int64             `json:"question_id"`
		Answer     domain.StringList `json:"answer"`
	} `json:"answers"`
}

func (a *App) saveAnswers(w http.ResponseWriter, r *http.Request) {
	attempt, err := a.attemptForStudent(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if attempt.Status != "in_progress" {
		writeError(w, r, invalidState("答卷已提交，不能修改"))
		return
	}
	var req saveAnswersRequest
	if err := decodeJSON(w, r, &req, 2<<20); err != nil {
		writeError(w, r, err)
		return
	}
	questions, err := a.Store.ListQuestions(r.Context(), attempt.PaperID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	qmap := make(map[int64]domain.Question, len(questions))
	for _, q := range questions {
		qmap[q.ID] = q
	}
	rows := make([]domain.AttemptAnswer, 0, len(req.Answers))
	for _, item := range req.Answers {
		q, ok := qmap[item.QuestionID]
		if !ok {
			writeError(w, r, badRequest("题目不属于该试卷"))
			return
		}
		total := 0
		for _, s := range item.Answer {
			total += len([]rune(s))
		}
		if total > 16000 {
			writeError(w, r, badRequest("单题答案过长（上限 16000 字）"))
			return
		}
		rows = append(rows, domain.AttemptAnswer{
			AttemptID:  attempt.ID,
			QuestionID: item.QuestionID,
			Answer:     item.Answer,
			MaxScore:   q.Score,
		})
	}
	if err := a.Store.UpsertAnswers(r.Context(), rows); err != nil {
		writeError(w, r, err)
		return
	}
	remaining := 0
	if attempt.Paper != nil && attempt.Paper.DurationMin > 0 {
		deadline := attempt.StartedAt.Add(time.Duration(attempt.Paper.DurationMin) * time.Minute)
		remaining = int(deadline.Sub(time.Now()).Seconds())
		if remaining < 0 {
			remaining = 0
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved":         len(rows),
		"saved_at":      time.Now().UTC(),
		"remaining_sec": remaining,
	})
}

func (a *App) submitAttempt(w http.ResponseWriter, r *http.Request) {
	attempt, err := a.attemptForStudent(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if attempt.Status == "in_progress" {
		var req struct {
			Confirm bool `json:"confirm"`
		}
		_ = decodeJSON(w, r, &req, 16<<10)
	}
	outcome, err := a.Svc.SubmitAttempt(r.Context(), attempt.ID, userFromRequest(r).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	updated, err := a.Store.GetAttempt(r.Context(), attempt.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	data := map[string]any{
		"status":          updated.Status,
		"score":           updated.Score,
		"objective_score": updated.ObjectiveScore,
		"pending":         outcome.PendingAI,
	}
	if outcome.Job != nil {
		data["job_id"] = outcome.Job.ID
	}
	writeJSON(w, http.StatusOK, data)
}

func (a *App) studentResult(w http.ResponseWriter, r *http.Request) {
	attempt, err := a.attemptForStudent(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	data, err := a.resultData(r, attempt, false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (a *App) studentHistory(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	u := userFromRequest(r)
	paperID, _ := intQuery(r, "paper_id")
	items, total, err := a.Store.ListAttempts(r.Context(), store.AttemptFilter{
		StudentID: u.ID,
		PaperID:   int64(paperID),
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
		title := ""
		if att.Paper != nil {
			title = att.Paper.Title
		}
		out = append(out, map[string]any{
			"id":           att.ID,
			"paper_id":     att.PaperID,
			"paper_title":  title,
			"status":       att.Status,
			"score":        att.Score,
			"full_score":   att.FullScore,
			"submitted_at": att.SubmittedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": total, "page": page, "page_size": size})
}

func (a *App) resultData(r *http.Request, attempt *domain.Attempt, isTeacher bool) (map[string]any, error) {
	ctx := r.Context()
	paper, err := a.Store.GetPaperBasic(ctx, attempt.PaperID)
	if err != nil {
		return nil, err
	}
	questions, err := a.Store.ListQuestions(ctx, attempt.PaperID)
	if err != nil {
		return nil, err
	}
	answers, err := a.Store.AttemptAnswers(ctx, attempt.ID)
	if err != nil {
		return nil, err
	}
	amap := make(map[int64]domain.AttemptAnswer, len(answers))
	for _, a := range answers {
		amap[a.QuestionID] = a
	}
	show := paper.ShowAnswer || isTeacher
	qout := make([]map[string]any, 0, len(questions))
	for i := range questions {
		q := &questions[i]
		item := attemptQuestionDTO(q)
		aa, ok := amap[q.ID]
		item["my_answer"] = domain.StringList{}
		if ok {
			item["my_answer"] = aa.Answer
			item["score"] = aa.Score
			item["max_score"] = aa.MaxScore
			item["is_correct"] = aa.IsCorrect
			item["grading_mode"] = aa.GradingMode
			item["feedback"] = aa.Feedback
			item["missing"] = aa.Missing
			item["advice"] = aa.Advice
		}
		if show {
			item["correct_answer"] = q.Answer
			item["analysis"] = q.Analysis
		} else {
			item["correct_answer"] = nil
		}
		qout = append(qout, item)
	}
	data := map[string]any{
		"attempt":   attemptDTO(attempt),
		"paper":     map[string]any{"id": paper.ID, "title": paper.Title, "show_answer": paper.ShowAnswer, "total_score": paper.TotalScore},
		"questions": qout,
		"report":    attempt.Report,
	}
	if isTeacher && attempt.Student != nil {
		data["student"] = userDTO(attempt.Student)
	}
	return data, nil
}
