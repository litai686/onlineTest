package store

import (
	"context"
	"errors"

	"onlineTest/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = gorm.ErrRecordNotFound

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var st domain.Setting
	if err := s.db.WithContext(ctx).Where("key = ?", key).First(&st).Error; err != nil {
		return "", err
	}
	return st.Value, nil
}

func (s *Store) GetSettingDefault(ctx context.Context, key, def string) string {
	v, err := s.GetSetting(ctx, key)
	if err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	st := domain.Setting{Key: key, Value: value}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&st).Error
}

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	var rows []domain.Setting
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

func (s *Store) CreateUser(ctx context.Context, u *domain.User) error {
	return s.db.WithContext(ctx).Create(u).Error
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	var u domain.User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) UpdateUser(ctx context.Context, id int64, fields map[string]any) error {
	res := s.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) TouchLogin(ctx context.Context, id int64) {
	now := s.db.NowFunc()
	_ = s.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", id).Updates(map[string]any{
		"last_login_at": now,
	}).Error
}

type StudentFilter struct {
	Keyword string
	Class   string
	Status  *int
	Page    int
	Size    int
}

func (s *Store) ListStudents(ctx context.Context, f StudentFilter) ([]domain.User, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.User{}).Where("role = ?", "student")
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		q = q.Where("username LIKE ? OR display_name LIKE ?", like, like)
	}
	if f.Class != "" {
		q = q.Where("class_name = ?", f.Class)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]domain.User, 0)
	err := q.Order("created_at DESC").Limit(f.Size).Offset((f.Page - 1) * f.Size).Find(&items).Error
	return items, total, err
}

func (s *Store) CountUsersByRole(ctx context.Context, role string) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.User{}).Where("role = ?", role).Count(&n).Error
	return n, err
}

func (s *Store) DeleteStudent(ctx context.Context, id int64) error {
	return s.WithTx(func(tx *Store) error {
		u, err := tx.GetUserByID(ctx, id)
		if err != nil {
			return err
		}
		if u.Role != "student" {
			return errors.New("not a student")
		}
		if err := tx.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", id).
			Update("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
			return err
		}
		return tx.db.WithContext(ctx).Delete(&domain.User{}, id).Error
	})
}

func (s *Store) CreateSubject(ctx context.Context, sub *domain.Subject) error {
	return s.db.WithContext(ctx).Create(sub).Error
}

func (s *Store) GetSubject(ctx context.Context, id int64) (*domain.Subject, error) {
	var sub domain.Subject
	if err := s.db.WithContext(ctx).First(&sub, id).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) ListSubjects(ctx context.Context, stage string, includeDisabled bool) ([]domain.Subject, error) {
	q := s.db.WithContext(ctx).Model(&domain.Subject{})
	if stage != "" {
		q = q.Where("stage = ?", stage)
	}
	if !includeDisabled {
		q = q.Where("status = ?", 1)
	}
	items := make([]domain.Subject, 0)
	err := q.Order("stage DESC, sort_order ASC, id ASC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	for i := range items {
		var n int64
		if err := s.db.WithContext(ctx).Model(&domain.Paper{}).Where("subject_id = ?", items[i].ID).Count(&n).Error; err != nil {
			return nil, err
		}
		items[i].PaperCount = n
	}
	return items, nil
}

func (s *Store) UpdateSubject(ctx context.Context, id int64, fields map[string]any) error {
	res := s.db.WithContext(ctx).Model(&domain.Subject{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) CountPapersBySubject(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.Paper{}).Where("subject_id = ?", id).Count(&n).Error
	return n, err
}

func (s *Store) DeleteSubject(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Delete(&domain.Subject{}, id).Error
}
