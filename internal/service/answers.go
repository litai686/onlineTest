package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
)

type answerQuestionPayload struct {
	QuestionID int64             `json:"question_id"`
	Seq        int               `json:"seq"`
	Type       string            `json:"type"`
	Stem       string            `json:"stem"`
	Options    domain.OptionList `json:"options,omitempty"`
	Score      float64           `json:"score"`
	Knowledge  domain.StringList `json:"knowledge,omitempty"`
}

type answerPaperPayload struct {
	PaperTitle string                  `json:"paper_title"`
	Stage      string                  `json:"stage"`
	Grade      string                  `json:"grade"`
	Questions  []answerQuestionPayload `json:"questions"`
}

func (s *Service) FillMissingAnswers(ctx context.Context, paperID int64) (int, []string, error) {
	questions, err := s.Store.ListQuestions(ctx, paperID)
	if err != nil {
		return 0, nil, err
	}
	paper, err := s.Store.GetPaperBasic(ctx, paperID)
	if err != nil {
		return 0, nil, err
	}
	missing := make([]domain.Question, 0)
	for _, q := range questions {
		if len(q.Answer) == 0 {
			missing = append(missing, q)
		}
	}
	if len(missing) == 0 {
		return 0, nil, nil
	}

	provider, err := s.AI.ProviderFor(ctx, "grading")
	if err != nil {
		return 0, nil, fmt.Errorf("未配置可用的 AI 模型，无法自动解析答案")
	}

	filled := 0
	var unresolved []string
	const batchSize = 8
	for start := 0; start < len(missing); start += batchSize {
		end := start + batchSize
		if end > len(missing) {
			end = len(missing)
		}
		batch := missing[start:end]
		items := make([]answerQuestionPayload, 0, len(batch))
		for _, q := range batch {
			items = append(items, answerQuestionPayload{
				QuestionID: q.ID,
				Seq:        q.Seq,
				Type:       q.Type,
				Stem:       q.Stem,
				Options:    q.Options,
				Score:      q.Score,
				Knowledge:  q.Knowledge,
			})
		}
		msgs, err := ai.AnswerMessages(answerPaperPayload{
			PaperTitle: paper.Title,
			Stage:      paper.Stage,
			Grade:      paper.Grade,
			Questions:  items,
		})
		if err != nil {
			return filled, unresolved, err
		}
		res, err := s.AI.Chat(ctx, provider, ai.ChatRequest{
			Messages:       msgs,
			ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
		}, "answer", "")
		if err != nil {
			return filled, unresolved, fmt.Errorf("AI 调用失败：%w", err)
		}
		raw, err := ai.ExtractJSON(res.Content)
		if err != nil {
			return filled, unresolved, fmt.Errorf("AI 返回内容不是合法 JSON")
		}
		results, err := parseAnswerItems(raw)
		if err != nil {
			return filled, unresolved, err
		}
		byID := make(map[int64]ai.AnswerItem, len(results))
		for _, item := range results {
			byID[item.QuestionID] = item
		}
		for _, q := range batch {
			item, ok := byID[q.ID]
			if !ok {
				unresolved = append(unresolved, fmt.Sprintf("第 %d 题", q.Seq))
				continue
			}
			answer, ok := normalizeAIAnswer(q, item.Answer)
			if !ok {
				unresolved = append(unresolved, fmt.Sprintf("第 %d 题", q.Seq))
				continue
			}
			fields := map[string]any{
				"answer":      answer,
				"need_manual": false,
			}
			if q.GradingMode == "manual" && q.NeedManual {
				fields["grading_mode"] = "auto"
			}
			if strings.TrimSpace(q.Analysis) == "" && strings.TrimSpace(item.Analysis) != "" {
				fields["analysis"] = truncateRunes(item.Analysis, 500)
			}
			if item.Confidence > 0 {
				fields["ai_confidence"] = clamp01(item.Confidence)
			}
			if err := s.Store.UpdateQuestion(ctx, q.ID, fields); err != nil {
				return filled, unresolved, err
			}
			filled++
		}
	}
	return filled, unresolved, nil
}

func parseAnswerItems(raw []byte) ([]ai.AnswerItem, error) {
	var result ai.AnswerResult
	if err := json.Unmarshal(raw, &result); err == nil && len(result.Items) > 0 {
		return result.Items, nil
	}
	var arr []ai.AnswerItem
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}
	var alt struct {
		Answers []ai.AnswerItem `json:"answers"`
	}
	if err := json.Unmarshal(raw, &alt); err == nil && len(alt.Answers) > 0 {
		return alt.Answers, nil
	}
	return nil, fmt.Errorf("AI 未返回有效的答案 JSON")
}

func normalizeAIAnswer(q domain.Question, in []string) (domain.StringList, bool) {
	cleaned := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len([]rune(s)) > 4000 {
			return nil, false
		}
		cleaned = append(cleaned, s)
	}
	if len(cleaned) == 0 {
		return nil, false
	}
	switch q.Type {
	case "single", "multiple":
		valid := make(map[string]bool, len(q.Options))
		byText := make(map[string]string, len(q.Options))
		for _, o := range q.Options {
			key := strings.ToUpper(strings.TrimSpace(o.Key))
			valid[key] = true
			byText[strings.TrimSpace(o.Text)] = key
		}
		seen := map[string]bool{}
		out := make(domain.StringList, 0, len(cleaned))
		for _, s := range cleaned {
			key := strings.ToUpper(s)
			if !valid[key] {
				if mapped, ok := byText[strings.TrimSpace(s)]; ok {
					key = mapped
				} else {
					continue
				}
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
		if q.Type == "single" && len(out) != 1 {
			return nil, false
		}
		if q.Type == "multiple" && len(out) == 0 {
			return nil, false
		}
		return out, true
	case "judge":
		v := normalizeJudge(cleaned[0])
		if v != "true" && v != "false" {
			return nil, false
		}
		return domain.StringList{v}, true
	case "fill":
		return domain.StringList(cleaned), true
	default:
		return domain.StringList(cleaned), true
	}
}

func (s *Service) ReviewQuestionAnswer(ctx context.Context, paperID, questionID int64) (*domain.Question, bool, string, error) {
	q, err := s.Store.GetQuestion(ctx, questionID)
	if err != nil || q.PaperID != paperID {
		return nil, false, "", ErrNotFound
	}
	provider, err := s.AI.ProviderFor(ctx, "grading")
	if err != nil {
		return nil, false, "", fmt.Errorf("未配置可用的 AI 模型")
	}
	msgs, err := ai.ReviewMessages(map[string]any{
		"question": map[string]any{
			"question_id":    q.ID,
			"type":           q.Type,
			"stem":           q.Stem,
			"options":        q.Options,
			"score":          q.Score,
			"knowledge":      q.Knowledge,
			"current_answer": q.Answer,
		},
	})
	if err != nil {
		return nil, false, "", err
	}
	res, err := s.AI.Chat(ctx, provider, ai.ChatRequest{
		Messages:       msgs,
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	}, "answer", "")
	if err != nil {
		return nil, false, "", fmt.Errorf("AI 调用失败：%w", err)
	}
	raw, err := ai.ExtractJSON(res.Content)
	if err != nil {
		return nil, false, "", fmt.Errorf("AI 返回内容不是合法 JSON")
	}
	var review ai.ReviewResult
	if err := json.Unmarshal(raw, &review); err != nil {
		return nil, false, "", fmt.Errorf("AI 返回的复核结果无法解析")
	}

	final := domain.StringList(q.Answer)
	invalidNewAnswer := false
	if len(q.Answer) == 0 || !review.IsCorrect {
		normalized, ok := normalizeAIAnswer(*q, review.Answer)
		if !ok {
			if len(q.Answer) == 0 {
				return nil, false, "", fmt.Errorf("AI 未能给出有效答案")
			}
			invalidNewAnswer = true
		} else {
			final = normalized
		}
	}
	changed := !sameAnswers(q.Answer, final)
	needsManual := invalidNewAnswer || (len(q.Answer) > 0 && !changed && !review.IsCorrect)
	fields := map[string]any{
		"need_manual": needsManual,
	}
	if changed {
		fields["answer"] = final
	}
	if !needsManual && q.NeedManual && q.GradingMode == "manual" {
		fields["grading_mode"] = "auto"
	}
	if strings.TrimSpace(q.Analysis) == "" && strings.TrimSpace(review.Analysis) != "" {
		fields["analysis"] = truncateRunes(review.Analysis, 500)
	}
	if review.Confidence > 0 {
		fields["ai_confidence"] = clamp01(review.Confidence)
	}
	if err := s.Store.UpdateQuestion(ctx, q.ID, fields); err != nil {
		return nil, false, "", err
	}
	updated, err := s.Store.GetQuestion(ctx, q.ID)
	if err != nil {
		return nil, false, "", err
	}
	feedback := strings.TrimSpace(review.Feedback)
	if feedback == "" {
		switch {
		case invalidNewAnswer:
			feedback = "AI 未能给出有效答案，已保留原答案"
		case changed:
			feedback = "AI 已修正答案"
		default:
			feedback = "AI 复核完成，答案正确"
		}
	}
	return updated, changed, feedback, nil
}

func sameAnswers(a, b domain.StringList) bool {
	if len(a) != len(b) {
		return false
	}
	normalizedA := make([]string, 0, len(a))
	normalizedB := make([]string, 0, len(b))
	for _, s := range a {
		normalizedA = append(normalizedA, normalizeAnswer(s))
	}
	for _, s := range b {
		normalizedB = append(normalizedB, normalizeAnswer(s))
	}
	sort.Strings(normalizedA)
	sort.Strings(normalizedB)
	for i := range normalizedA {
		if normalizedA[i] != normalizedB[i] {
			return false
		}
	}
	return true
}
