package store

import (
	"context"
	"strconv"

	"onlineTest/internal/domain"

	"gorm.io/gorm"
)

func (s *Store) ListProviders(ctx context.Context) ([]domain.AIProvider, error) {
	items := make([]domain.AIProvider, 0)
	err := s.db.WithContext(ctx).Order("id ASC").Find(&items).Error
	return items, err
}

func (s *Store) GetProvider(ctx context.Context, id int64) (*domain.AIProvider, error) {
	var p domain.AIProvider
	if err := s.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateProvider(ctx context.Context, p *domain.AIProvider) error {
	return s.db.WithContext(ctx).Create(p).Error
}

func (s *Store) UpdateProvider(ctx context.Context, id int64, fields map[string]any) error {
	res := s.db.WithContext(ctx).Model(&domain.AIProvider{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) DeleteProvider(ctx context.Context, id int64) error {
	return s.WithTx(func(tx *Store) error {
		if err := tx.db.WithContext(ctx).Delete(&domain.AIProvider{}, id).Error; err != nil {
			return err
		}
		for _, key := range []string{"ai.default", "ai.role.vision", "ai.role.grading", "ai.role.chat"} {
			if v, err := tx.GetSetting(ctx, key); err == nil && v == strconv.FormatInt(id, 10) {
				if err := tx.SetSetting(ctx, key, ""); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Store) CreateAICall(ctx context.Context, call *domain.AICall) error {
	return s.db.WithContext(ctx).Create(call).Error
}

type AICallFilter struct {
	Purpose string
	Status  string
	Page    int
	Size    int
}

func (s *Store) ListAICalls(ctx context.Context, f AICallFilter) ([]domain.AICall, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.AICall{})
	if f.Purpose != "" {
		q = q.Where("purpose = ?", f.Purpose)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]domain.AICall, 0)
	err := q.Order("created_at DESC").Limit(f.Size).Offset((f.Page - 1) * f.Size).Find(&items).Error
	return items, total, err
}
