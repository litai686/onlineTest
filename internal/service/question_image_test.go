package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"onlineTest/internal/domain"
	"onlineTest/internal/store"
)

func newImageTestService(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	st := store.New(db)
	if _, _, err := store.Seed(ctx, st, "Admin12345"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return New(st, nil, nil, dir, 1<<20), st, dir
}

func createImageFile(t *testing.T, svc *Service, st *store.Store, dir string) *domain.File {
	t.Helper()
	rel := "uploads/2026/10/figure.png"
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("png"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	f := &domain.File{
		ID: "file-img", OriginalName: "figure.png", StoredPath: rel,
		Mime: "image/png", SizeBytes: 3, SHA256: "abc", Kind: "question_image",
	}
	if err := st.CreateFile(context.Background(), f); err != nil {
		t.Fatalf("create file: %v", err)
	}
	return f
}

func createImagePaper(t *testing.T, st *store.Store) int64 {
	t.Helper()
	var subject domain.Subject
	if err := st.DB().First(&subject).Error; err != nil {
		t.Fatalf("subject: %v", err)
	}
	var teacher domain.User
	if err := st.DB().Where("role = ?", "teacher").First(&teacher).Error; err != nil {
		t.Fatalf("teacher: %v", err)
	}
	paper := &domain.Paper{Title: "配图删除测试", SubjectID: subject.ID, Stage: subject.Stage, CreatedBy: teacher.ID}
	if err := st.DB().Create(paper).Error; err != nil {
		t.Fatalf("create paper: %v", err)
	}
	return paper.ID
}

func createImageQuestion(t *testing.T, st *store.Store, paperID int64, imageID string) int64 {
	t.Helper()
	q := &domain.Question{
		PaperID:     paperID,
		Type:        "single",
		Stem:        "看图选答案",
		Options:     domain.OptionList{{Key: "A", Text: "2"}, {Key: "B", Text: "3"}},
		Answer:      domain.StringList{"A"},
		SourceImage: imageID,
	}
	if err := st.CreateQuestion(context.Background(), q); err != nil {
		t.Fatalf("create question: %v", err)
	}
	return q.ID
}

func fileExists(t *testing.T, svc *Service, f *domain.File) bool {
	t.Helper()
	_, err := os.Stat(svc.FilePath(f))
	return err == nil
}

func TestDeleteQuestionRemovesUnusedImage(t *testing.T) {
	ctx := context.Background()
	svc, st, dir := newImageTestService(t)
	img := createImageFile(t, svc, st, dir)
	paperID := createImagePaper(t, st)

	first := createImageQuestion(t, st, paperID, img.ID)
	second := createImageQuestion(t, st, paperID, img.ID)

	if err := svc.DeleteQuestion(ctx, first); err != nil {
		t.Fatalf("delete first question: %v", err)
	}
	if _, err := st.GetFile(ctx, img.ID); err != nil {
		t.Fatalf("image removed while still referenced: %v", err)
	}
	if !fileExists(t, svc, img) {
		t.Fatal("image file removed while still referenced")
	}

	if err := svc.DeleteQuestion(ctx, second); err != nil {
		t.Fatalf("delete second question: %v", err)
	}
	if _, err := st.GetFile(ctx, img.ID); err == nil {
		t.Fatal("image record should be removed with the last question")
	}
	if fileExists(t, svc, img) {
		t.Fatal("image file should be removed with the last question")
	}
}

func TestDeletePaperRemovesQuestionImages(t *testing.T) {
	ctx := context.Background()
	svc, st, dir := newImageTestService(t)
	img := createImageFile(t, svc, st, dir)
	paperID := createImagePaper(t, st)
	createImageQuestion(t, st, paperID, img.ID)

	if err := svc.DeletePaper(ctx, paperID); err != nil {
		t.Fatalf("delete paper: %v", err)
	}
	if _, err := st.GetFile(ctx, img.ID); err == nil {
		t.Fatal("image record should be removed with the paper")
	}
	if fileExists(t, svc, img) {
		t.Fatal("image file should be removed with the paper")
	}
}
