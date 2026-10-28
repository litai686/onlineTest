package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
	"onlineTest/internal/filex"
	"onlineTest/internal/security"
	"onlineTest/internal/store"

	"github.com/google/uuid"
	"mime/multipart"
)

var (
	ErrBadRequest = errors.New("bad request")
	ErrForbidden  = errors.New("forbidden")
	ErrConflict   = errors.New("conflict")
)

type Service struct {
	Store     *store.Store
	AI        *ai.Client
	Cipher    *security.Cipher
	DataDir   string
	MaxUpload int64
	Queue     *Queue
}

func New(st *store.Store, client *ai.Client, cipher *security.Cipher, dataDir string, maxUpload int64) *Service {
	return &Service{Store: st, AI: client, Cipher: cipher, DataDir: dataDir, MaxUpload: maxUpload}
}

func (s *Service) SetQueue(q *Queue) { s.Queue = q }

func (s *Service) SaveUpload(ctx context.Context, ownerID int64, fh *multipart.FileHeader, kind string) (*domain.File, error) {
	if kind == "" {
		kind = "paper_source"
	}
	saved, err := filex.Save(s.DataDir, fh, s.MaxUpload)
	if err != nil {
		return nil, err
	}
	f := &domain.File{
		ID:           saved.ID,
		OwnerID:      &ownerID,
		OriginalName: saved.OriginalName,
		StoredPath:   saved.StoredPath,
		Mime:         saved.Mime,
		SizeBytes:    saved.SizeBytes,
		SHA256:       saved.SHA256,
		Kind:         kind,
		Width:        saved.Width,
		Height:       saved.Height,
	}
	if err := s.Store.CreateFile(ctx, f); err != nil {
		_ = filepath.Base(f.StoredPath)
		return nil, err
	}
	return f, nil
}

func (s *Service) EnqueueJob(ctx context.Context, typ, refType string, refID int64, payload any, createdBy int64) (*domain.Job, error) {
	if existing, err := s.Store.FindActiveJob(ctx, typ, refType, refID); err == nil {
		return existing, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	job := &domain.Job{
		ID:        uuid.NewString(),
		Type:      typ,
		RefType:   refType,
		RefID:     refID,
		Payload:   string(raw),
		Status:    "pending",
		Message:   "排队中",
		CreatedBy: &createdBy,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Store.CreateJob(ctx, job); err != nil {
		return nil, err
	}
	if s.Queue != nil {
		s.Queue.Notify()
	}
	return job, nil
}

func (s *Service) ReportProgress(ctx context.Context, jobID string, progress int, message string) {
	_ = s.Store.UpdateJob(ctx, jobID, map[string]any{"progress": progress, "message": message})
}

func (s *Service) RunJob(ctx context.Context, job *domain.Job) error {
	switch job.Type {
	case "paper_parse":
		return s.RunParseJob(ctx, job)
	case "attempt_grade":
		return s.RunGradeJob(ctx, job)
	case "report_regen":
		return s.RunReportJob(ctx, job)
	default:
		return fmt.Errorf("unknown job type %q", job.Type)
	}
}

func (s *Service) FilePath(f *domain.File) string {
	return filepath.Join(s.DataDir, filepath.FromSlash(f.StoredPath))
}

func pagesPerRequest(ctx context.Context, st *store.Store) int {
	v := st.GetSettingDefault(ctx, "ai.role.vision_pages_per_request", "3")
	if n := atoiSafe(v, 3); n >= 1 && n <= 10 {
		return n
	}
	return 3
}

func questionsPerRequest(ctx context.Context, st *store.Store) int {
	v := st.GetSettingDefault(ctx, "ai.grading.questions_per_request", "5")
	if n := atoiSafe(v, 5); n >= 1 && n <= 20 {
		return n
	}
	return 5
}

func atoiSafe(s string, def int) int {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err == nil {
		return n
	}
	return def
}

func (s *Service) RemoveUpload(ctx context.Context, f *domain.File) {
	if f == nil {
		return
	}
	_ = os.Remove(s.FilePath(f))
	_ = s.Store.DeleteFile(ctx, f.ID)
}

func (s *Service) DeletePaperSourceFiles(ctx context.Context, paperID int64) error {
	links, err := s.Store.PaperFiles(ctx, paperID)
	if err != nil {
		return err
	}
	files := make([]*domain.File, 0, len(links))
	for _, link := range links {
		f, err := s.Store.GetFile(ctx, link.FileID)
		if err != nil {
			continue
		}
		files = append(files, f)
	}
	if err := s.Store.WithTx(func(tx *store.Store) error {
		if err := tx.DB().WithContext(ctx).Where("paper_id = ?", paperID).Delete(&domain.PaperFile{}).Error; err != nil {
			return err
		}
		for _, f := range files {
			if err := tx.DB().WithContext(ctx).Delete(&domain.File{}, "id = ?", f.ID).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, f := range files {
		_ = os.Remove(s.FilePath(f))
	}
	return nil
}
