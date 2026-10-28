package service

import (
	"context"

	"onlineTest/internal/domain"
)

type QuestionCheck struct {
	IsCorrect   *bool
	Score       *float64
	MaxScore    float64
	GradingMode string
	Feedback    string
}

func (s *Service) CheckQuestion(ctx context.Context, attemptID int64, q domain.Question, answer domain.StringList) (*QuestionCheck, error) {
	row := domain.AttemptAnswer{
		AttemptID:  attemptID,
		QuestionID: q.ID,
		Answer:     answer,
		MaxScore:   q.Score,
	}
	if err := s.Store.UpsertAnswers(ctx, []domain.AttemptAnswer{row}); err != nil {
		return nil, err
	}
	result := &QuestionCheck{MaxScore: q.Score}
	if q.Type == "short" {
		result.GradingMode = "manual"
		result.Feedback = "主观题参考答案"
		return result, nil
	}
	correct := answersEqual(q, answer)
	score := 0.0
	if correct {
		score = q.Score
	}
	result.IsCorrect = &correct
	result.Score = &score
	result.GradingMode = "local"
	if correct {
		result.Feedback = "回答正确"
	} else {
		result.Feedback = "回答错误"
	}
	return result, nil
}
