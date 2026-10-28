package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
)

type SubmitPayload struct {
	PendingAI    int
	PendingEquiv []int64
	Job          *domain.Job
}

func (s *Service) SubmitAttempt(ctx context.Context, attemptID, studentID int64) (*SubmitPayload, error) {
	attempt, err := s.Store.GetAttempt(ctx, attemptID)
	if err != nil {
		return nil, ErrNotFound
	}
	if attempt.StudentID != studentID {
		return nil, ErrForbidden
	}
	if attempt.Status != "in_progress" {
		return &SubmitPayload{}, nil
	}
	paper, err := s.Store.GetPaperBasic(ctx, attempt.PaperID)
	if err != nil {
		return nil, err
	}
	questions, err := s.Store.ListQuestions(ctx, paper.ID)
	if err != nil {
		return nil, err
	}
	answers, err := s.Store.AttemptAnswers(ctx, attempt.ID)
	if err != nil {
		return nil, err
	}
	answerMap := make(map[int64]domain.AttemptAnswer, len(answers))
	for _, a := range answers {
		answerMap[a.QuestionID] = a
	}

	now := time.Now().UTC()
	rows := make([]domain.AttemptAnswer, 0, len(questions))
	pendingAI := 0
	var pendingEquiv []int64
	objective := 0.0
	correct := 0
	manualCount := 0

	for _, q := range questions {
		row, ok := answerMap[q.ID]
		if !ok {
			row = domain.AttemptAnswer{AttemptID: attempt.ID, QuestionID: q.ID}
		}
		row.MaxScore = q.Score
		row.GradingMode = ""
		row.Score = nil
		row.IsCorrect = nil
		row.Feedback = ""
		row.Missing = nil
		row.Advice = ""

		switch {
		case q.Type == "short":
			if !paper.AIGrading || q.GradingMode == "manual" {
				row.GradingMode = "manual"
				zero := 0.0
				row.Score = &zero
				manualCount++
			} else {
				pendingAI++
			}
		case q.GradingMode == "manual":
			row.GradingMode = "manual"
			zero := 0.0
			row.Score = &zero
			manualCount++
		default:
			row.GradingMode = "local"
			equal := answersEqual(q, row.Answer)
			row.IsCorrect = &equal
			if equal {
				score := q.Score
				row.Score = &score
				objective += score
				correct++
			} else {
				zero := 0.0
				row.Score = &zero
				if q.Type == "fill" && (containsLatex(q.Answer) || containsLatex(row.Answer)) {
					pendingEquiv = append(pendingEquiv, q.ID)
				}
			}
		}
		rows = append(rows, row)
	}

	if err := s.Store.BulkUpsertAnswersFull(ctx, rows); err != nil {
		return nil, err
	}

	fullScore := 0.0
	for _, q := range questions {
		fullScore += q.Score
	}

	status := "graded"
	if pendingAI > 0 || len(pendingEquiv) > 0 {
		status = "grading"
	}
	duration := int(now.Sub(attempt.StartedAt).Seconds())
	if duration < 0 {
		duration = 0
	}
	fields := map[string]any{
		"status":          status,
		"submitted_at":    now,
		"duration_sec":    duration,
		"full_score":      fullScore,
		"objective_score": objective,
		"score":           objective,
		"correct_count":   correct,
		"grade_error":     "",
	}
	if manualCount > 0 {
		fields["grade_error"] = fmt.Sprintf("%d 道题需人工复核", manualCount)
	}
	if err := s.Store.UpdateAttempt(ctx, attempt.ID, fields); err != nil {
		return nil, err
	}

	out := &SubmitPayload{PendingAI: pendingAI + len(pendingEquiv)}
	if status == "grading" {
		job, err := s.EnqueueJob(ctx, "attempt_grade", "attempt", attempt.ID, GradePayload{
			AttemptID:    attempt.ID,
			PendingEquiv: pendingEquiv,
		}, studentID)
		if err != nil {
			return nil, err
		}
		out.Job = job
	} else {
		if err := s.FinalizeAttempt(ctx, attempt.ID, "", false); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func findQuestion(list []domain.Question, id int64) (domain.Question, bool) {
	for _, q := range list {
		if q.ID == id {
			return q, true
		}
	}
	return domain.Question{}, false
}

type GradePayload struct {
	AttemptID    int64   `json:"attempt_id"`
	PendingEquiv []int64 `json:"pending_equiv"`
}

func (s *Service) RunGradeJob(ctx context.Context, job *domain.Job) error {
	var payload GradePayload
	if err := unmarshalPayload(job.Payload, &payload); err != nil {
		return err
	}
	attempt, err := s.Store.GetAttempt(ctx, payload.AttemptID)
	if err != nil {
		return fmt.Errorf("答卷不存在: %w", err)
	}
	if attempt.Status == "graded" {
		return nil
	}
	questions, err := s.Store.ListQuestions(ctx, attempt.PaperID)
	if err != nil {
		return err
	}
	qmap := make(map[int64]domain.Question, len(questions))
	for _, q := range questions {
		qmap[q.ID] = q
	}
	answers, err := s.Store.AttemptAnswers(ctx, attempt.ID)
	if err != nil {
		return err
	}

	provider, perr := s.AI.ProviderFor(ctx, "grading")
	if perr != nil {
		for i := range answers {
			q := qmap[answers[i].QuestionID]
			if q.Type == "short" && answers[i].GradingMode == "" {
				answers[i].GradingMode = "manual"
				zero := 0.0
				answers[i].Score = &zero
				answers[i].Feedback = "未配置 AI 阅卷模型，待人工复核"
			}
		}
		_ = s.Store.BulkUpsertAnswersFull(ctx, answers)
		return s.FinalizeAttempt(ctx, attempt.ID, "未配置可用的 AI 阅卷模型", false)
	}

	s.ReportProgress(ctx, job.ID, 20, "AI 阅卷中")
	var gradeErr []string

	var pending []domain.AttemptAnswer
	for _, a := range answers {
		q := qmap[a.QuestionID]
		if q.Type == "short" && a.GradingMode == "" {
			pending = append(pending, a)
		}
	}

	batchSize := questionsPerRequest(ctx, s.Store)
	var updates []domain.AttemptAnswer
	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[start:end]
		items := make([]gradeItemPayload, 0, len(batch))
		for _, a := range batch {
			q := qmap[a.QuestionID]
			items = append(items, gradeItemPayload{
				QuestionID:      q.ID,
				Seq:             q.Seq,
				Type:            q.Type,
				Stem:            q.Stem,
				Options:         q.Options,
				ReferenceAnswer: q.Answer,
				Analysis:        q.Analysis,
				MaxScore:        q.Score,
				StudentAnswer:   a.Answer,
				Knowledge:       q.Knowledge,
			})
		}
		results, err := s.gradeBatch(ctx, provider, items, job.ID)
		if err != nil {
			for _, a := range batch {
				a.GradingMode = "manual"
				zero := 0.0
				a.Score = &zero
				a.Feedback = "AI 阅卷失败，待人工复核"
				updates = append(updates, a)
			}
			gradeErr = append(gradeErr, err.Error())
			continue
		}
		for _, a := range batch {
			res, ok := results[a.QuestionID]
			q := qmap[a.QuestionID]
			if !ok {
				a.GradingMode = "manual"
				zero := 0.0
				a.Score = &zero
				a.Feedback = "AI 未返回该题评分，待人工复核"
				updates = append(updates, a)
				continue
			}
			score := clampScore(res.Score, q.Score)
			a.Score = &score
			a.GradingMode = "ai"
			a.Feedback = truncateRunes(strings.TrimSpace(res.Feedback), 200)
			a.Missing = toStringList(res.Missing)
			a.Advice = truncateRunes(strings.TrimSpace(res.Advice), 200)
			correct := math.Abs(score-q.Score) < 1e-6
			a.IsCorrect = &correct
			updates = append(updates, a)
		}
		s.ReportProgress(ctx, job.ID, 20+(start+len(batch))*50/maxInt(len(pending), 1), "AI 阅卷中")
	}

	if len(payload.PendingEquiv) > 0 {
		updates = append(updates, s.checkEquivalence(ctx, provider, payload.PendingEquiv, qmap, answers, job.ID, &gradeErr)...)
	}

	if len(updates) > 0 {
		if err := s.Store.BulkUpsertAnswersFull(ctx, updates); err != nil {
			return err
		}
	}
	s.ReportProgress(ctx, job.ID, 80, "生成学习报告")
	return s.FinalizeAttempt(ctx, attempt.ID, strings.Join(gradeErr, "; "), true)
}

func (s *Service) gradeBatch(ctx context.Context, provider *domain.AIProvider, items []gradeItemPayload, jobID string) (map[int64]ai.GradeItem, error) {
	msgs, err := ai.GradeMessages(map[string]any{
		"instructions": "按参考答案与评分要求逐题评分，输出 JSON 数组。",
		"items":        items,
	})
	if err != nil {
		return nil, err
	}
	res, err := s.AI.Chat(ctx, provider, ai.ChatRequest{
		Messages:       msgs,
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	}, "grade", jobID)
	if err != nil {
		return nil, err
	}
	raw, err := ai.ExtractJSON(res.Content)
	if err != nil {
		return nil, fmt.Errorf("阅卷结果不是合法 JSON: %w", err)
	}
	var list []ai.GradeItem
	if err := jsonUnmarshalFlex(raw, &list); err != nil {
		return nil, err
	}
	out := make(map[int64]ai.GradeItem, len(list))
	for _, item := range list {
		out[item.QuestionID] = item
	}
	return out, nil
}

func (s *Service) checkEquivalence(ctx context.Context, provider *domain.AIProvider, ids []int64, qmap map[int64]domain.Question, answers []domain.AttemptAnswer, jobID string, gradeErr *[]string) []domain.AttemptAnswer {
	var items []map[string]any
	answerMap := make(map[int64]domain.AttemptAnswer)
	for _, a := range answers {
		answerMap[a.QuestionID] = a
	}
	for _, id := range ids {
		q, ok := qmap[id]
		if !ok {
			continue
		}
		a := answerMap[id]
		items = append(items, map[string]any{
			"question_id":      id,
			"stem":             q.Stem,
			"reference_answer": q.Answer,
			"student_answer":   a.Answer,
		})
	}
	if len(items) == 0 {
		return nil
	}
	var updates []domain.AttemptAnswer
	for start := 0; start < len(items); start += 10 {
		end := start + 10
		if end > len(items) {
			end = len(items)
		}
		batch := items[start:end]
		msgs, err := ai.EquivMessages(map[string]any{"items": batch})
		if err != nil {
			continue
		}
		res, err := s.AI.Chat(ctx, provider, ai.ChatRequest{
			Messages:       msgs,
			ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
		}, "equiv", jobID)
		if err != nil {
			*gradeErr = append(*gradeErr, "等价判定失败: "+err.Error())
			continue
		}
		raw, err := ai.ExtractJSON(res.Content)
		if err != nil {
			continue
		}
		var out ai.EquivResult
		if err := jsonUnmarshalFlex(raw, &out); err != nil {
			continue
		}
		for _, item := range out.Items {
			q, ok := qmap[item.QuestionID]
			if !ok {
				continue
			}
			a := answerMap[item.QuestionID]
			score := 0.0
			correct := false
			if item.Equivalent {
				score = q.Score
				correct = true
			}
			a.Score = &score
			a.IsCorrect = &correct
			a.GradingMode = "ai"
			a.Feedback = "AI 等价判定：" + truncateRunes(item.Reason, 100)
			updates = append(updates, a)
		}
	}
	return updates
}

func (s *Service) FinalizeAttempt(ctx context.Context, attemptID int64, aiError string, withAI bool) error {
	attempt, err := s.Store.GetAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	questions, err := s.Store.ListQuestions(ctx, attempt.PaperID)
	if err != nil {
		return err
	}
	qmap := make(map[int64]domain.Question, len(questions))
	for _, q := range questions {
		qmap[q.ID] = q
	}
	answers, err := s.Store.AttemptAnswers(ctx, attempt.ID)
	if err != nil {
		return err
	}

	full, objective, subjective, correct := 0.0, 0.0, 0.0, 0
	wrongKnowledge := map[string]int{}
	knowledgeTotal := map[string]int{}
	var perQuestion []map[string]any

	for _, q := range questions {
		full += q.Score
		for _, k := range q.Knowledge {
			knowledgeTotal[k]++
		}
	}
	// Build a lookup of answers.
	amap := make(map[int64]domain.AttemptAnswer, len(answers))
	for _, a := range answers {
		amap[a.QuestionID] = a
	}
	for _, q := range questions {
		a := amap[q.ID]
		score := 0.0
		if a.Score != nil {
			score = *a.Score
		}
		if q.Type == "short" {
			subjective += score
		} else {
			objective += score
		}
		if a.IsCorrect != nil && *a.IsCorrect {
			correct++
		}
		if score+1e-6 < q.Score {
			for _, k := range q.Knowledge {
				wrongKnowledge[k]++
			}
		}
		perQuestion = append(perQuestion, map[string]any{
			"question_id": q.ID,
			"seq":         q.Seq,
			"score":       round1(score),
			"max":         q.Score,
			"mode":        a.GradingMode,
			"type":        q.Type,
			"comment":     truncateRunes(a.Feedback, 120),
		})
	}
	total := objective + subjective
	accuracy := 0.0
	if len(questions) > 0 {
		accuracy = float64(correct) / float64(len(questions))
	}
	scoreRate := 0.0
	if full > 0 {
		scoreRate = total / full
	}

	weak := make([]map[string]any, 0, len(wrongKnowledge))
	for k, wrong := range wrongKnowledge {
		weak = append(weak, map[string]any{"knowledge": k, "wrong": wrong, "total": knowledgeTotal[k]})
	}
	sort.Slice(weak, func(i, j int) bool {
		return weak[i]["wrong"].(int) > weak[j]["wrong"].(int)
	})
	if len(weak) > 5 {
		weak = weak[:5]
	}

	report := domain.ReportMap{
		"version":      1,
		"generated_by": "local",
		"accuracy":     round3(accuracy),
		"score_rate":   round3(scoreRate),
		"duration_sec": attempt.DurationSec,
		"weak_points":  weak,
		"per_question": perQuestion,
		"summary":      localSummary(total, full, accuracy, weak),
		"suggestions":  localSuggestions(weak),
		"ai_error":     aiError,
	}

	if withAI && total >= 0 {
		if provider, err := s.AI.ProviderFor(ctx, "grading"); err == nil {
			if aiReport, err := s.generateReport(ctx, provider, attempt, questions, amap, total, full, accuracy, weak); err != nil {
				report["ai_error"] = err.Error()
			} else {
				report["summary"] = aiReport.Summary
				report["suggestions"] = aiReport.Suggestions
				report["generated_by"] = "local+ai"
				report["ai_error"] = ""
			}
		}
	}

	now := time.Now().UTC()
	duration := attempt.DurationSec
	gradeError := aiError
	if len(gradeError) > 500 {
		gradeError = truncateRunes(gradeError, 500)
	}
	return s.Store.UpdateAttempt(ctx, attempt.ID, map[string]any{
		"status":           "graded",
		"score":            total,
		"objective_score":  objective,
		"subjective_score": subjective,
		"correct_count":    correct,
		"report":           report,
		"grade_error":      gradeError,
		"graded_at":        now,
		"duration_sec":     duration,
	})
}

func (s *Service) generateReport(ctx context.Context, p *domain.AIProvider, attempt *domain.Attempt, questions []domain.Question, answers map[int64]domain.AttemptAnswer, total, full, accuracy float64, weak []map[string]any) (*ai.ReportResult, error) {
	var wrong []map[string]any
	for _, q := range questions {
		a := answers[q.ID]
		score := 0.0
		if a.Score != nil {
			score = *a.Score
		}
		if score+1e-6 < q.Score {
			wrong = append(wrong, map[string]any{
				"seq":          q.Seq,
				"stem":         truncateRunes(q.Stem, 200),
				"student":      truncateRunes(strings.Join(a.Answer, " | "), 300),
				"score":        score,
				"max":          q.Score,
				"feedback":     a.Feedback,
				"knowledge":    q.Knowledge,
				"correct_text": truncateRunes(strings.Join(q.Answer, " | "), 300),
			})
			if len(wrong) >= 12 {
				break
			}
		}
	}
	payload := map[string]any{
		"paper_title": attempt.Paper.Title,
		"score":       round1(total),
		"full_score":  full,
		"accuracy":    round3(accuracy),
		"weak_points": weak,
		"wrong_items": wrong,
		"grade":       attempt.Paper.Grade,
		"subject":     subjectName(attempt.Paper),
	}
	msgs, err := ai.ReportMessages(payload)
	if err != nil {
		return nil, err
	}
	res, err := s.AI.Chat(ctx, p, ai.ChatRequest{
		Messages:       msgs,
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	}, "report", "")
	if err != nil {
		return nil, err
	}
	raw, err := ai.ExtractJSON(res.Content)
	if err != nil {
		return nil, err
	}
	var out ai.ReportResult
	if err := jsonUnmarshalFlex(raw, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.Summary) == "" {
		return nil, fmt.Errorf("报告内容为空")
	}
	return &out, nil
}

func subjectName(p *domain.Paper) string {
	if p.Subject != nil {
		return p.Subject.Name
	}
	return ""
}

func localSummary(total, full, accuracy float64, weak []map[string]any) string {
	if full <= 0 {
		return "本次作答已完成。"
	}
	names := make([]string, 0, minInt(len(weak), 3))
	for i, w := range weak {
		if i >= 3 {
			break
		}
		names = append(names, fmt.Sprintf("%v", w["knowledge"]))
	}
	base := fmt.Sprintf("本次得分 %.1f/%.1f，正确率 %.0f%%。", total, full, accuracy*100)
	if len(names) == 0 {
		return base + "整体掌握较好，建议保持练习节奏。"
	}
	return base + "需要重点关注：" + strings.Join(names, "、") + "。"
}

func localSuggestions(weak []map[string]any) []string {
	out := []string{
		"逐题复盘错题，先写出对应的概念与公式，再独立重做一遍。",
		"把同类题集中练习 3~5 道，确认稳定后再进入下一知识点。",
	}
	for i, w := range weak {
		if i >= 2 {
			break
		}
		out = append(out, fmt.Sprintf("针对「%v」整理错题成因并做专项练习。", w["knowledge"]))
	}
	return out
}

type gradeItemPayload struct {
	QuestionID      int64             `json:"question_id"`
	Seq             int               `json:"seq"`
	Type            string            `json:"type"`
	Stem            string            `json:"stem"`
	Options         domain.OptionList `json:"options,omitempty"`
	ReferenceAnswer domain.StringList `json:"reference_answer"`
	Analysis        string            `json:"analysis,omitempty"`
	MaxScore        float64           `json:"max_score"`
	StudentAnswer   domain.StringList `json:"student_answer"`
	Knowledge       domain.StringList `json:"knowledge,omitempty"`
}

func answersEqual(q domain.Question, student domain.StringList) bool {
	want := normalizeList(q.Answer)
	got := normalizeList(student)
	switch q.Type {
	case "single", "multiple":
		if len(want) != len(got) || len(want) == 0 {
			return false
		}
		sort.Strings(want)
		sort.Strings(got)
		for i := range want {
			if want[i] != got[i] {
				return false
			}
		}
		return true
	case "judge":
		if len(want) == 0 || len(got) == 0 {
			return false
		}
		return normalizeJudge(want[0]) == normalizeJudge(got[0])
	case "fill":
		if len(want) != len(got) {
			return false
		}
		for i := range want {
			if want[i] != got[i] {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func normalizeList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, normalizeAnswer(s))
	}
	return out
}

func normalizeAnswer(s string) string {
	s = strings.TrimSpace(s)
	hadLatex := strings.Contains(s, "\\")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, "\\left", "")
	s = strings.ReplaceAll(s, "\\right", "")
	s = strings.ReplaceAll(s, "\\dfrac", "\\frac")
	s = strings.ReplaceAll(s, "\\tfrac", "\\frac")
	s = strings.ReplaceAll(s, "\\,", "")
	s = strings.ReplaceAll(s, "\\;", "")
	s = strings.ReplaceAll(s, "\\!", "")
	s = strings.ReplaceAll(s, "\\ ", "")
	s = strings.ReplaceAll(s, "\u3000", " ")
	s = strings.ToLower(s)
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			sb.WriteRune(r - 0xFEE0)
		case r == 0x3002:
			sb.WriteRune('.')
		case r == 0xFF0C:
			sb.WriteRune(',')
		case unicode.IsSpace(r):
			// whitespace removed only for formula-like answers below
			sb.WriteRune(' ')
		default:
			sb.WriteRune(r)
		}
	}
	s = sb.String()
	if hadLatex || strings.ContainsAny(s, "\\^_{}") {
		s = strings.Join(strings.Fields(s), "")
	} else {
		s = strings.Join(strings.Fields(s), " ")
	}
	return s
}

func normalizeJudge(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "true", "t", "1", "对", "正确", "是", "√":
		return "true"
	case "false", "f", "0", "错", "错误", "否", "×":
		return "false"
	}
	return s
}

func containsLatex(in []string) bool {
	for _, s := range in {
		if strings.ContainsAny(s, "\\^_{}") || strings.Contains(s, "$") {
			return true
		}
	}
	return false
}

func clampScore(score, max float64) float64 {
	if score < 0 {
		score = 0
	}
	if score > max {
		score = max
	}
	return math.Round(score*2) / 2
}

func toDomainStringList(in []string) domain.StringList {
	out := make(domain.StringList, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toStringList(in []string) domain.StringList { return toDomainStringList(in) }

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
