package store

import (
	"context"
	"errors"

	"onlineTest/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) StartAttempt(ctx context.Context, paperID, studentID int64, allowRetry bool) (*domain.Attempt, bool, error) {
	var out *domain.Attempt
	created := false
	err := s.WithTx(func(tx *Store) error {
		var existing domain.Attempt
		err := tx.db.WithContext(ctx).
			Where("paper_id = ? AND student_id = ? AND status IN ('in_progress','submitted','grading')", paperID, studentID).
			Order("id DESC").First(&existing).Error
		if err == nil {
			out = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var doneCount int64
		if !allowRetry {
			if err := tx.db.WithContext(ctx).Model(&domain.Attempt{}).
				Where("paper_id = ? AND student_id = ?", paperID, studentID).
				Count(&doneCount).Error; err != nil {
				return err
			}
			if doneCount > 0 {
				return ErrRetryNotAllowed
			}
		}

		var maxNo int
		if err := tx.db.WithContext(ctx).Model(&domain.Attempt{}).
			Where("paper_id = ? AND student_id = ?", paperID, studentID).
			Select("COALESCE(MAX(attempt_no), 0)").Scan(&maxNo).Error; err != nil {
			return err
		}
		att := domain.Attempt{
			PaperID:   paperID,
			StudentID: studentID,
			AttemptNo: maxNo + 1,
			Status:    "in_progress",
			StartedAt: tx.db.NowFunc(),
		}
		res := tx.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&att)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			var again domain.Attempt
			if err := tx.db.WithContext(ctx).
				Where("paper_id = ? AND student_id = ? AND attempt_no = ?", paperID, studentID, maxNo+1).
				First(&again).Error; err != nil {
				return err
			}
			out = &again
			return nil
		}
		out = &att
		created = true
		return nil
	})
	return out, created, err
}

var ErrRetryNotAllowed = errors.New("retry not allowed")

func (s *Store) GetAttempt(ctx context.Context, id int64) (*domain.Attempt, error) {
	var a domain.Attempt
	err := s.db.WithContext(ctx).
		Preload("Paper").
		Preload("Paper.Subject").
		Preload("Student").
		First(&a, id).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) UpdateAttempt(ctx context.Context, id int64, fields map[string]any) error {
	res := s.db.WithContext(ctx).Model(&domain.Attempt{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) AttemptAnswers(ctx context.Context, attemptID int64) ([]domain.AttemptAnswer, error) {
	items := make([]domain.AttemptAnswer, 0)
	err := s.db.WithContext(ctx).Where("attempt_id = ?", attemptID).Find(&items).Error
	return items, err
}

func (s *Store) UpsertAnswers(ctx context.Context, rows []domain.AttemptAnswer) error {
	if len(rows) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "attempt_id"}, {Name: "question_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"answer", "updated_at"}),
	}).CreateInBatches(&rows, 50).Error
}

type AttemptFilter struct {
	PaperID   int64
	StudentID int64
	Status    string
	Page      int
	Size      int
}

func (s *Store) ListAttempts(ctx context.Context, f AttemptFilter) ([]domain.Attempt, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Attempt{})
	if f.PaperID > 0 {
		q = q.Where("paper_id = ?", f.PaperID)
	}
	if f.StudentID > 0 {
		q = q.Where("student_id = ?", f.StudentID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]domain.Attempt, 0)
	err := q.Preload("Paper").Preload("Paper.Subject").Preload("Student").
		Order("started_at DESC").Limit(f.Size).Offset((f.Page - 1) * f.Size).Find(&items).Error
	return items, total, err
}

func (s *Store) AttemptSummary(ctx context.Context, paperID int64) (submitted, graded int64, avg float64, err error) {
	type row struct {
		Submitted int64
		Graded    int64
		Avg       float64
	}
	var r row
	err = s.db.WithContext(ctx).Model(&domain.Attempt{}).
		Where("paper_id = ?", paperID).
		Select("COUNT(*) AS submitted, SUM(CASE WHEN status = 'graded' THEN 1 ELSE 0 END) AS graded, COALESCE(AVG(CASE WHEN status = 'graded' THEN score END), 0) AS avg").
		Scan(&r).Error
	return r.Submitted, r.Graded, r.Avg, err
}

func (s *Store) CountAnswers(ctx context.Context, attemptID int64) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.AttemptAnswer{}).Where("attempt_id = ?", attemptID).Count(&n).Error
	return n, err
}

func (s *Store) UpsertSingleAnswer(ctx context.Context, row *domain.AttemptAnswer) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "attempt_id"}, {Name: "question_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"answer":       row.Answer,
			"score":        row.Score,
			"is_correct":   row.IsCorrect,
			"max_score":    row.MaxScore,
			"grading_mode": row.GradingMode,
			"feedback":     row.Feedback,
			"missing":      row.Missing,
			"advice":       row.Advice,
		}),
	}).Create(row).Error
}

func (s *Store) BulkUpsertAnswersFull(ctx context.Context, rows []domain.AttemptAnswer) error {
	if len(rows) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "attempt_id"}, {Name: "question_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"answer", "is_correct", "score", "max_score", "grading_mode",
			"feedback", "missing", "advice", "updated_at",
		}),
	}).CreateInBatches(&rows, 50).Error
}

func (s *Store) GetAnswer(ctx context.Context, attemptID, questionID int64) (*domain.AttemptAnswer, error) {
	var a domain.AttemptAnswer
	if err := s.db.WithContext(ctx).Where("attempt_id = ? AND question_id = ?", attemptID, questionID).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) ResetAnswersForRegrade(ctx context.Context, attemptID, paperID int64) error {
	return s.db.WithContext(ctx).Model(&domain.AttemptAnswer{}).
		Where("attempt_id = ? AND question_id IN (SELECT id FROM questions WHERE paper_id = ? AND type = 'short')", attemptID, paperID).
		Updates(map[string]any{
			"grading_mode": "",
			"score":        nil,
			"is_correct":   nil,
			"feedback":     "",
			"missing":      "[]",
			"advice":       "",
		}).Error
}
