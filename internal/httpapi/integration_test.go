package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"onlineTest/internal/ai"
	"onlineTest/internal/config"
	"onlineTest/internal/httpapi"
	"onlineTest/internal/security"
	"onlineTest/internal/service"
	"onlineTest/internal/store"
)

type testApp struct {
	srv *httptest.Server
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.Addr = ":0"

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
	if _, _, err := store.Seed(t.Context(), st, "Admin12345"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	secret := []byte("test-secret-for-integration")
	cipher, err := security.NewCipher(secret)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	jwtManager := security.NewJWTManager(secret, 30)
	aiClient := ai.NewClient(st, cipher)
	svc := service.New(st, aiClient, cipher, dir, 20<<20)
	queue := service.NewQueue(svc)
	svc.SetQueue(queue)
	app := httpapi.New(cfg, st, jwtManager, cipher, svc, queue, "test")
	srv := httptest.NewServer(app.Routes())
	t.Cleanup(srv.Close)
	return &testApp{srv: srv}
}

func (a *testApp) do(t *testing.T, method, path, token string, body any) (int, envelope) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.srv.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env envelope
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode response %s: %v", string(raw), err)
		}
	}
	return res.StatusCode, env
}

func (a *testApp) decode(t *testing.T, env envelope, out any) {
	t.Helper()
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatalf("decode data: %v", err)
	}
}

func login(t *testing.T, app *testApp, username, password string) string {
	t.Helper()
	status, env := app.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	})
	if status != http.StatusOK {
		t.Fatalf("login %s: status=%d message=%s", username, status, env.Message)
	}
	var data struct {
		Token string `json:"token"`
	}
	app.decode(t, env, &data)
	if data.Token == "" {
		t.Fatalf("empty token")
	}
	return data.Token
}

func TestStudentExamFlow(t *testing.T) {
	app := newTestApp(t)
	teacher := login(t, app, "admin", "Admin12345")

	status, env := app.do(t, http.MethodGet, "/api/v1/subjects", teacher, nil)
	if status != http.StatusOK {
		t.Fatalf("subjects: %d %s", status, env.Message)
	}
	var subjects struct {
		Items []struct {
			ID    int64  `json:"id"`
			Stage string `json:"stage"`
			Name  string `json:"name"`
		} `json:"items"`
	}
	app.decode(t, env, &subjects)
	var subjectID int64
	for _, s := range subjects.Items {
		if s.Stage == "junior" && s.Name == "数学" {
			subjectID = s.ID
			break
		}
	}
	if subjectID == 0 {
		t.Fatal("junior math subject not seeded")
	}

	username := "stu_test_01"
	status, env = app.do(t, http.MethodPost, "/api/v1/students", teacher, map[string]any{
		"username": username, "display_name": "测试学生", "password": "Student123",
	})
	if status != http.StatusCreated {
		t.Fatalf("create student: %d %s", status, env.Message)
	}

	status, env = app.do(t, http.MethodPost, "/api/v1/papers", teacher, map[string]any{
		"title": "集成测试试卷", "subject_id": subjectID, "grade": "初二", "duration_min": 30,
	})
	if status != http.StatusCreated {
		t.Fatalf("create paper: %d %s", status, env.Message)
	}
	var paper struct {
		ID int64 `json:"id"`
	}
	app.decode(t, env, &paper)

	questions := []map[string]any{
		{"type": "single", "stem": "1+1=?", "options": []map[string]string{{"key": "A", "text": "2"}, {"key": "B", "text": "3"}}, "answer": []string{"A"}, "score": 5},
		{"type": "multiple", "stem": "质数有", "options": []map[string]string{{"key": "A", "text": "2"}, {"key": "B", "text": "3"}, {"key": "C", "text": "4"}}, "answer": []string{"A", "B"}, "score": 5},
		{"type": "judge", "stem": "0 是自然数", "answer": []string{"true"}, "score": 5},
		{"type": "fill", "stem": "1/2 + 1/3 = ____", "answer": []string{"5/6"}, "score": 5},
	}
	for _, q := range questions {
		status, env = app.do(t, http.MethodPost, fmt.Sprintf("/api/v1/papers/%d/questions", paper.ID), teacher, q)
		if status != http.StatusCreated {
			t.Fatalf("add question: %d %s", status, env.Message)
		}
	}
	status, env = app.do(t, http.MethodPost, fmt.Sprintf("/api/v1/papers/%d/publish", paper.ID), teacher, nil)
	if status != http.StatusOK {
		t.Fatalf("publish: %d %s", status, env.Message)
	}

	student := login(t, app, username, "Student123")
	status, env = app.do(t, http.MethodPost, fmt.Sprintf("/api/v1/student/papers/%d/attempts", paper.ID), student, nil)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("start attempt: %d %s", status, env.Message)
	}
	var started struct {
		Attempt struct {
			ID int64 `json:"id"`
		} `json:"attempt"`
		Questions []struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"questions"`
	}
	app.decode(t, env, &started)
	if len(started.Questions) != 4 {
		t.Fatalf("want 4 questions, got %d", len(started.Questions))
	}
	answerMap := map[string][]string{"single": {"A"}, "multiple": {"B", "A"}, "judge": {"true"}, "fill": {"5/6"}}
	var answers []map[string]any
	for _, q := range started.Questions {
		answers = append(answers, map[string]any{"question_id": q.ID, "answer": answerMap[q.Type]})
	}
	status, env = app.do(t, http.MethodPut, fmt.Sprintf("/api/v1/student/attempts/%d/answers", started.Attempt.ID), student, map[string]any{"answers": answers})
	if status != http.StatusOK {
		t.Fatalf("save answers: %d %s", status, env.Message)
	}
	status, env = app.do(t, http.MethodPost, fmt.Sprintf("/api/v1/student/attempts/%d/submit", started.Attempt.ID), student, map[string]bool{"confirm": true})
	if status != http.StatusOK {
		t.Fatalf("submit: %d %s", status, env.Message)
	}
	var submitted struct {
		Status string   `json:"status"`
		Score  *float64 `json:"score"`
	}
	app.decode(t, env, &submitted)
	if submitted.Status != "graded" {
		t.Fatalf("want graded, got %s", submitted.Status)
	}
	if submitted.Score == nil || *submitted.Score != 20 {
		t.Fatalf("want score 20, got %v", submitted.Score)
	}

	status, env = app.do(t, http.MethodGet, fmt.Sprintf("/api/v1/student/attempts/%d/result", started.Attempt.ID), student, nil)
	if status != http.StatusOK {
		t.Fatalf("result: %d %s", status, env.Message)
	}
	var result struct {
		Attempt struct {
			Status       string  `json:"status"`
			Score        float64 `json:"score"`
			FullScore    float64 `json:"full_score"`
			CorrectCount int     `json:"correct_count"`
		} `json:"attempt"`
	}
	app.decode(t, env, &result)
	if result.Attempt.Score != 20 || result.Attempt.FullScore != 20 || result.Attempt.CorrectCount != 4 {
		t.Fatalf("unexpected result: %+v", result.Attempt)
	}

	// Student must not access teacher APIs.
	status, _ = app.do(t, http.MethodGet, "/api/v1/papers", student, nil)
	if status != http.StatusForbidden {
		t.Fatalf("student access teacher api: want 403, got %d", status)
	}
	// No token.
	status, _ = app.do(t, http.MethodGet, "/api/v1/papers", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", status)
	}
	// Wrong answers should be wrong.
	status, env = app.do(t, http.MethodGet, fmt.Sprintf("/api/v1/teacher/attempts/%d/result", started.Attempt.ID), teacher, nil)
	if status != http.StatusOK {
		t.Fatalf("teacher result: %d %s", status, env.Message)
	}
}

func TestChangePasswordInvalidatesOldToken(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "Admin12345")
	status, env := app.do(t, http.MethodPost, "/api/v1/auth/change-password", token, map[string]string{
		"old_password": "Admin12345", "new_password": "NewPass123",
	})
	if status != http.StatusOK {
		t.Fatalf("change password: %d %s", status, env.Message)
	}
	status, _ = app.do(t, http.MethodGet, "/api/v1/auth/me", token, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("old token should be invalid, got %d", status)
	}
	login(t, app, "admin", "NewPass123")
}
