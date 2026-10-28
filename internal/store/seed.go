package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"onlineTest/internal/domain"
	"onlineTest/internal/security"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var seedSubjects = map[string][]string{
	"junior": {"语文", "数学", "英语", "物理", "化学", "生物", "历史", "地理", "政治"},
	"senior": {"语文", "数学", "英语", "物理", "化学", "生物", "历史", "地理", "政治"},
}

var defaultSettings = map[string]string{
	"system.version":                   "1",
	"ai.default":                       "",
	"ai.role.vision":                   "",
	"ai.role.grading":                  "",
	"ai.role.chat":                     "",
	"ai.role.vision_pages_per_request": "3",
	"ai.grading.questions_per_request": "5",
}

func Seed(ctx context.Context, s *Store, adminPassword string) (string, bool, error) {
	for stage, names := range seedSubjects {
		for i, name := range names {
			sub := domain.Subject{Stage: stage, Name: name, SortOrder: i * 10, Status: 1}
			if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&sub).Error; err != nil {
				return "", false, fmt.Errorf("seed subject %s/%s: %w", stage, name, err)
			}
		}
	}

	for k, v := range defaultSettings {
		st := domain.Setting{Key: k, Value: v}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&st).Error; err != nil {
			return "", false, fmt.Errorf("seed setting %s: %w", k, err)
		}
	}

	n, err := s.CountUsersByRole(ctx, "teacher")
	if err != nil {
		return "", false, err
	}
	if n > 0 {
		return "", false, nil
	}

	generated := false
	pwd := adminPassword
	if pwd == "" {
		pwd, err = security.RandomString(12)
		if err != nil {
			return "", false, err
		}
		generated = true
	}
	hash, err := security.HashPassword(pwd)
	if err != nil {
		return "", false, err
	}
	admin := domain.User{
		Username:      "admin",
		PasswordHash:  hash,
		DisplayName:   "系统管理员",
		Role:          "teacher",
		Status:        1,
		MustChangePwd: generated,
	}
	if err := s.CreateUser(ctx, &admin); err != nil {
		return "", false, fmt.Errorf("seed admin: %w", err)
	}
	slog.Info("initial teacher account created", "username", "admin", "generated_password", generated)
	return pwd, generated, nil
}

var ErrNoPendingJob = errors.New("no pending job")

func (s *Store) ResetRunningJobs(ctx context.Context) error {
	return s.db.WithContext(ctx).Model(&domain.Job{}).
		Where("status = ?", "running").
		Updates(map[string]any{"status": "pending", "started_at": nil, "message": "进程重启，重新排队"}).Error
}

func isUniqueViolation(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		errors.Is(err, gorm.ErrForeignKeyViolated)
}
