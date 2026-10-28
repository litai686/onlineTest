package store

import (
	"context"
	"errors"
	"fmt"

	"onlineTest/internal/domain"

	"gorm.io/gorm"
)

var ErrQuestionInUse = errors.New("question has submitted answers")

func (s *Store) CreateFile(ctx context.Context, f *domain.File) error {
	return s.db.WithContext(ctx).Create(f).Error
}

func (s *Store) GetFile(ctx context.Context, id string) (*domain.File, error) {
	var f domain.File
	if err := s.db.WithContext(ctx).First(&f, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *Store) AttachPaperFile(ctx context.Context, pf *domain.PaperFile) error {
	return s.db.WithContext(ctx).Create(pf).Error
}

func (s *Store) AppendPaperFiles(ctx context.Context, paperID int64, files []domain.PaperFile) error {
	if len(files) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&files).Error
}

func (s *Store) ReplacePaperFiles(ctx context.Context, paperID int64, files []domain.PaperFile) error {
	return s.WithTx(func(tx *Store) error {
		if err := tx.db.WithContext(ctx).Where("paper_id = ?", paperID).Delete(&domain.PaperFile{}).Error; err != nil {
			return err
		}
		if len(files) == 0 {
			return nil
		}
		return tx.db.WithContext(ctx).Create(&files).Error
	})
}

func (s *Store) PaperFiles(ctx context.Context, paperID int64) ([]domain.PaperFile, error) {
	items := make([]domain.PaperFile, 0)
	err := s.db.WithContext(ctx).Where("paper_id = ?", paperID).Order("page_no ASC").Find(&items).Error
	return items, err
}

func (s *Store) StudentCanReadFile(ctx context.Context, fileID string, studentID int64) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Table("paper_files pf").
		Joins("JOIN attempts a ON a.paper_id = pf.paper_id").
		Where("pf.file_id = ? AND a.student_id = ?", fileID, studentID).
		Count(&n).Error
	if err != nil || n > 0 {
		return n > 0, err
	}
	err = s.db.WithContext(ctx).Table("questions q").
		Joins("JOIN attempts a ON a.paper_id = q.paper_id").
		Where("q.source_image = ? AND a.student_id = ?", fileID, studentID).
		Count(&n).Error
	return n > 0, err
}

func (s *Store) ListPapers(ctx context.Context, f PaperFilter) ([]PaperListRow, int64, error) {
	q := s.db.WithContext(ctx).Table("papers").
		Joins("LEFT JOIN subjects ON subjects.id = papers.subject_id")
	if f.SubjectID > 0 {
		q = q.Where("papers.subject_id = ?", f.SubjectID)
	}
	if f.Stage != "" {
		q = q.Where("papers.stage = ?", f.Stage)
	}
	if f.Status != "" {
		q = q.Where("papers.status = ?", f.Status)
	}
	if f.Keyword != "" {
		q = q.Where("papers.title LIKE ?", "%"+f.Keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]PaperListRow, 0)
	err := q.Select(`papers.id, papers.title, papers.subject_id, papers.stage, papers.grade,
			papers.status, papers.parse_status, papers.parse_error, papers.source_type,
			papers.question_count, papers.total_score, papers.duration_min, papers.published_at, papers.created_at,
			COALESCE(subjects.name, '') AS subject_name,
			(SELECT COUNT(*) FROM attempts WHERE attempts.paper_id = papers.id) AS attempt_count,
			(SELECT COUNT(*) FROM attempts WHERE attempts.paper_id = papers.id AND attempts.status = 'graded') AS graded_count`).
		Order("papers.created_at DESC").
		Limit(f.Size).Offset((f.Page - 1) * f.Size).
		Scan(&rows).Error
	return rows, total, err
}

type PaperFilter struct {
	SubjectID int64
	Stage     string
	Status    string
	Keyword   string
	Page      int
	Size      int
}

type PaperListRow struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	SubjectID     int64   `json:"subject_id"`
	SubjectName   string  `json:"subject_name"`
	Stage         string  `json:"stage"`
	Grade         string  `json:"grade"`
	Status        string  `json:"status"`
	ParseStatus   string  `json:"parse_status"`
	ParseError    string  `json:"parse_error"`
	SourceType    string  `json:"source_type"`
	QuestionCount int     `json:"question_count"`
	TotalScore    float64 `json:"total_score"`
	DurationMin   int     `json:"duration_min"`
	PublishedAt   *string `json:"published_at"`
	CreatedAt     string  `json:"created_at"`
	AttemptCount  int64   `json:"attempt_count"`
	GradedCount   int64   `json:"graded_count"`
}

func (s *Store) GetPaper(ctx context.Context, id int64) (*domain.Paper, error) {
	var p domain.Paper
	err := s.db.WithContext(ctx).
		Preload("Subject").
		Preload("Questions", func(db *gorm.DB) *gorm.DB { return db.Order("seq ASC") }).
		Preload("Files", func(db *gorm.DB) *gorm.DB { return db.Order("page_no ASC") }).
		Preload("Files.File").
		First(&p, id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetPaperBasic(ctx context.Context, id int64) (*domain.Paper, error) {
	var p domain.Paper
	if err := s.db.WithContext(ctx).Preload("Subject").First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreatePaper(ctx context.Context, p *domain.Paper) error {
	return s.db.WithContext(ctx).Create(p).Error
}

func (s *Store) UpdatePaper(ctx context.Context, id int64, fields map[string]any) error {
	res := s.db.WithContext(ctx).Model(&domain.Paper{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) DeletePaper(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Select("Questions", "Files").Delete(&domain.Paper{}, id).Error
}

func (s *Store) CountAttemptsByPaper(ctx context.Context, paperID int64) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.Attempt{}).Where("paper_id = ?", paperID).Count(&n).Error
	return n, err
}

func (s *Store) ListQuestions(ctx context.Context, paperID int64) ([]domain.Question, error) {
	items := make([]domain.Question, 0)
	err := s.db.WithContext(ctx).Where("paper_id = ?", paperID).Order("seq ASC").Find(&items).Error
	return items, err
}

func (s *Store) GetQuestion(ctx context.Context, id int64) (*domain.Question, error) {
	var q domain.Question
	if err := s.db.WithContext(ctx).First(&q, id).Error; err != nil {
		return nil, err
	}
	return &q, nil
}

// CountQuestionsByImage 统计仍引用某张图片的题目数量，用于判断图片是否可以清理。
func (s *Store) CountQuestionsByImage(ctx context.Context, fileID string) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.Question{}).Where("source_image = ?", fileID).Count(&n).Error
	return n, err
}

func (s *Store) CreateQuestion(ctx context.Context, q *domain.Question) error {
	return s.WithTx(func(tx *Store) error {
		var maxSeq int
		if err := tx.db.WithContext(ctx).Model(&domain.Question{}).
			Where("paper_id = ?", q.PaperID).
			Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		q.Seq = maxSeq + 1
		if err := tx.db.WithContext(ctx).Create(q).Error; err != nil {
			return err
		}
		return tx.RecalcPaper(ctx, q.PaperID)
	})
}

func (s *Store) UpdateQuestion(ctx context.Context, id int64, fields map[string]any) error {
	var q domain.Question
	if err := s.db.WithContext(ctx).First(&q, id).Error; err != nil {
		return err
	}
	return s.WithTx(func(tx *Store) error {
		res := tx.db.WithContext(ctx).Model(&domain.Question{}).Where("id = ?", id).Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		return tx.RecalcPaper(ctx, q.PaperID)
	})
}

func (s *Store) DeleteQuestion(ctx context.Context, id int64) error {
	var q domain.Question
	if err := s.db.WithContext(ctx).First(&q, id).Error; err != nil {
		return err
	}
	return s.WithTx(func(tx *Store) error {
		var submittedAnswers int64
		if err := tx.db.WithContext(ctx).Table("attempt_answers aa").
			Joins("JOIN attempts a ON a.id = aa.attempt_id").
			Where("aa.question_id = ? AND a.status <> ?", id, "in_progress").
			Count(&submittedAnswers).Error; err != nil {
			return err
		}
		if submittedAnswers > 0 {
			return ErrQuestionInUse
		}
		if err := tx.db.WithContext(ctx).Where("question_id = ?", id).Delete(&domain.AttemptAnswer{}).Error; err != nil {
			if IsForeignKeyErr(err) {
				return ErrQuestionInUse
			}
			return err
		}
		if err := tx.db.WithContext(ctx).Delete(&domain.Question{}, id).Error; err != nil {
			if IsForeignKeyErr(err) {
				return ErrQuestionInUse
			}
			return err
		}
		var rest []domain.Question
		if err := tx.db.WithContext(ctx).Where("paper_id = ?", q.PaperID).Order("seq ASC").Find(&rest).Error; err != nil {
			return err
		}
		for i := range rest {
			if rest[i].Seq != i+1 {
				if err := tx.db.WithContext(ctx).Model(&domain.Question{}).Where("id = ?", rest[i].ID).Update("seq", i+1).Error; err != nil {
					return err
				}
			}
		}
		return tx.RecalcPaper(ctx, q.PaperID)
	})
}

func (s *Store) ReorderQuestions(ctx context.Context, paperID int64, ids []int64) error {
	return s.WithTx(func(tx *Store) error {
		for i, id := range ids {
			res := tx.db.WithContext(ctx).Model(&domain.Question{}).
				Where("id = ? AND paper_id = ?", id, paperID).
				Update("seq", i+1)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("question %d not in paper %d", id, paperID)
			}
		}
		return tx.RecalcPaper(ctx, paperID)
	})
}

func (s *Store) RecalcPaper(ctx context.Context, paperID int64) error {
	var stat struct {
		Count int
		Total float64
	}
	if err := s.db.WithContext(ctx).Model(&domain.Question{}).
		Where("paper_id = ?", paperID).
		Select("COUNT(*) AS count, COALESCE(SUM(score), 0) AS total").
		Scan(&stat).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&domain.Paper{}).Where("id = ?", paperID).
		Updates(map[string]any{"question_count": stat.Count, "total_score": stat.Total}).Error
}

func (s *Store) UpdatePaperParseStatus(ctx context.Context, paperID int64, status, errMsg string) error {
	return s.db.WithContext(ctx).Model(&domain.Paper{}).Where("id = ?", paperID).
		Updates(map[string]any{"parse_status": status, "parse_error": errMsg}).Error
}

func (s *Store) PaperFileHashes(ctx context.Context, paperID int64) (map[string]bool, error) {
	type row struct {
		SHA256 string `gorm:"column:sha256"`
	}
	var rows []row
	err := s.db.WithContext(ctx).Table("paper_files pf").
		Select("f.sha256 AS sha256").
		Joins("JOIN files f ON f.id = pf.file_id").
		Where("pf.paper_id = ?", paperID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.SHA256 != "" {
			out[r.SHA256] = true
		}
	}
	return out, nil
}

func (s *Store) DeleteFile(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.File{}, "id = ?", id).Error
}

// CountPaperFilesByFile 统计仍引用某个文件的试卷附件数量。
func (s *Store) CountPaperFilesByFile(ctx context.Context, fileID string) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.PaperFile{}).Where("file_id = ?", fileID).Count(&n).Error
	return n, err
}
