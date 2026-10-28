package store

import (
	"fmt"

	"onlineTest/internal/domain"

	"gorm.io/gorm"
)

var extraDDL = []string{
	`CREATE INDEX IF NOT EXISTS ix_users_role_status ON users (role, status)`,
	`CREATE INDEX IF NOT EXISTS ix_users_class ON users (class_name)`,
	`CREATE INDEX IF NOT EXISTS ix_papers_subject ON papers (subject_id, status, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS ix_papers_status ON papers (status, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS ix_papers_created_by ON papers (created_by, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS ix_questions_paper ON questions (paper_id, seq)`,
	`CREATE INDEX IF NOT EXISTS ix_attempts_student ON attempts (student_id, status, started_at DESC)`,
	`CREATE INDEX IF NOT EXISTS ix_attempts_paper ON attempts (paper_id, status)`,
	`CREATE INDEX IF NOT EXISTS ix_jobs_status ON jobs (status, created_at)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS ux_jobs_active_ref ON jobs (type, ref_type, ref_id) WHERE status IN ('pending', 'running')`,
	`CREATE INDEX IF NOT EXISTS ix_ai_calls_created ON ai_calls (created_at)`,
	`CREATE INDEX IF NOT EXISTS ix_ai_calls_purpose ON ai_calls (purpose, created_at)`,
	`CREATE INDEX IF NOT EXISTS ix_audit_actor ON audit_logs (actor_id, created_at)`,
}

func Migrate(db *gorm.DB) error {
	if err := dropLegacyUsernameCheck(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&domain.Subject{},
		&domain.User{},
		&domain.AIProvider{},
		&domain.File{},
		&domain.Paper{},
		&domain.PaperFile{},
		&domain.Question{},
		&domain.Attempt{},
		&domain.AttemptAnswer{},
		&domain.Job{},
		&domain.AICall{},
		&domain.Setting{},
		&domain.AuditLog{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}
	for _, stmt := range extraDDL {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("extra ddl: %w", err)
		}
	}
	return nil
}

func dropLegacyUsernameCheck(db *gorm.DB) error {
	if !db.Migrator().HasTable(&domain.User{}) {
		return nil
	}
	if !db.Migrator().HasConstraint(&domain.User{}, "chk_users_username") {
		return nil
	}
	return db.Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		defer func() { _ = tx.Exec("PRAGMA foreign_keys = ON").Error }()
		return tx.Migrator().DropConstraint(&domain.User{}, "chk_users_username")
	})
}
