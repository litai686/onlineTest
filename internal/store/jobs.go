package store

import (
	"context"
	"time"

	"onlineTest/internal/domain"

	"gorm.io/gorm"
)

func (s *Store) CreateJob(ctx context.Context, job *domain.Job) error {
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *Store) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	var j domain.Job
	if err := s.db.WithContext(ctx).First(&j, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) FindActiveJob(ctx context.Context, typ, refType string, refID int64) (*domain.Job, error) {
	var j domain.Job
	err := s.db.WithContext(ctx).
		Where("type = ? AND ref_type = ? AND ref_id = ? AND status IN ('pending','running')", typ, refType, refID).
		First(&j).Error
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) UpdateJob(ctx context.Context, id string, fields map[string]any) error {
	res := s.db.WithContext(ctx).Model(&domain.Job{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

type JobFilter struct {
	Type   string
	Status string
	Page   int
	Size   int
}

func (s *Store) ListJobs(ctx context.Context, f JobFilter) ([]domain.Job, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Job{})
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]domain.Job, 0)
	err := q.Order("created_at DESC").Limit(f.Size).Offset((f.Page - 1) * f.Size).Find(&items).Error
	return items, total, err
}

func (s *Store) ClaimNextJob(ctx context.Context) (*domain.Job, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var j domain.Job
		err := s.db.WithContext(ctx).
			Where("status = ?", "pending").
			Order("created_at ASC").
			First(&j).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, ErrNoPendingJob
			}
			return nil, err
		}
		now := time.Now().UTC()
		res := s.db.WithContext(ctx).Model(&domain.Job{}).
			Where("id = ? AND status = ?", j.ID, "pending").
			Updates(map[string]any{"status": "running", "started_at": now, "attempts": gorm.Expr("attempts + 1")})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 1 {
			j.Status = "running"
			j.StartedAt = &now
			j.Attempts++
			return &j, nil
		}
	}
	return nil, ErrNoPendingJob
}
