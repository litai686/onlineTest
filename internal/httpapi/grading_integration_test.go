package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"onlineTest/internal/ai"
	"onlineTest/internal/config"
	"onlineTest/internal/domain"
	"onlineTest/internal/httpapi"
	"onlineTest/internal/security"
	"onlineTest/internal/service"
	"onlineTest/internal/store"
)

func mockAIServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var system, user string
		for _, m := range req.Messages {
			var text string
			if err := json.Unmarshal(m.Content, &text); err != nil {
				text = string(m.Content)
			}
			if m.Role == "system" {
				system += text
			} else {
				user += text
			}
		}
		var content any
		switch {
		case strings.Contains(system, "阅卷老师"):
			var payload struct {
				Items []struct {
					QuestionID int64   `json:"question_id"`
					MaxScore   float64 `json:"max_score"`
				} `json:"items"`
			}
			_ = json.Unmarshal([]byte(user), &payload)
			items := make([]map[string]any, 0, len(payload.Items))
			for _, item := range payload.Items {
				items = append(items, map[string]any{
					"question_id": item.QuestionID,
					"score":       item.MaxScore,
					"is_correct":  true,
					"feedback":    "步骤完整",
					"missing":     []string{},
					"advice":      "保持",
				})
			}
			content = items
		case strings.Contains(system, "学习规划老师"):
			content = map[string]any{
				"summary":     "掌握良好",
				"suggestions": []string{"继续练习"},
				"weak_points": []map[string]any{},
			}
		default:
			content = map[string]any{"ok": true}
		}
		raw, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": string(mustJSON(content))}}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestAIGradingAndRegradeKeepObjectiveScore(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	db, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	st := store.New(db)
	if _, _, err := store.Seed(t.Context(), st, "Admin12345"); err != nil {
		t.Fatal(err)
	}
	secret := []byte("integration-secret")
	cipher, _ := security.NewCipher(secret)
	enc, _ := cipher.Encrypt("sk-test")
	mock := mockAIServer(t)
	provider := &domain.AIProvider{
		Name: "mock", BaseURL: mock.URL + "/v1", APIKeyCipher: enc, APIKeyHint: "****test",
		Model: "mock", SupportsJSON: true, Temperature: 0.1, TimeoutSec: 20, Concurrency: 2, Status: 1,
	}
	if err := st.CreateProvider(t.Context(), provider); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(t.Context(), "ai.default", fmt.Sprint(provider.ID)); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(t.Context(), "ai.role.grading", fmt.Sprint(provider.ID)); err != nil {
		t.Fatal(err)
	}

	jwtManager := security.NewJWTManager(secret, 30)
	aiClient := ai.NewClient(st, cipher)
	svc := service.New(st, aiClient, cipher, dir, 20<<20)
	queue := service.NewQueue(svc)
	svc.SetQueue(queue)
	queue.Start(2)
	t.Cleanup(queue.Stop)

	app := httpapi.New(cfg, st, jwtManager, cipher, svc, queue, "test")
	srv := httptest.NewServer(app.Routes())
	t.Cleanup(srv.Close)
	ta := &testApp{srv: srv}

	teacher := login(t, ta, "admin", "Admin12345")
	subjectsEnv := envelope{}
	_, subjectsEnv = ta.do(t, http.MethodGet, "/api/v1/subjects", teacher, nil)
	var subjects struct {
		Items []struct {
			ID    int64  `json:"id"`
			Stage string `json:"stage"`
			Name  string `json:"name"`
		} `json:"items"`
	}
	ta.decode(t, subjectsEnv, &subjects)
	var subjectID int64
	for _, s := range subjects.Items {
		if s.Stage == "junior" && s.Name == "数学" {
			subjectID = s.ID
		}
	}
	if subjectID == 0 {
		t.Fatal("subject not found")
	}

	username := "ai_student"
	status, env := ta.do(t, http.MethodPost, "/api/v1/students", teacher, map[string]any{
		"username": username, "display_name": "AI 学生", "password": "Student123",
	})
	if status != http.StatusCreated {
		t.Fatalf("create student: %d %s", status, env.Message)
	}
	status, env = ta.do(t, http.MethodPost, "/api/v1/papers", teacher, map[string]any{
		"title": "AI 阅卷测试", "subject_id": subjectID,
	})
	if status != http.StatusCreated {
		t.Fatalf("create paper: %d %s", status, env.Message)
	}
	var paper struct {
		ID int64 `json:"id"`
	}
	ta.decode(t, env, &paper)
	questions := []map[string]any{
		{"type": "single", "stem": "1+1=?", "options": []map[string]string{{"key": "A", "text": "2"}, {"key": "B", "text": "3"}}, "answer": []string{"A"}, "score": 5},
		{"type": "short", "stem": "解方程 x+1=2", "answer": []string{"x=1"}, "score": 10, "knowledge": []string{"方程"}},
	}
	for _, q := range questions {
		status, env = ta.do(t, http.MethodPost, fmt.Sprintf("/api/v1/papers/%d/questions", paper.ID), teacher, q)
		if status != http.StatusCreated {
			t.Fatalf("add question: %d %s", status, env.Message)
		}
	}
	status, env = ta.do(t, http.MethodPost, fmt.Sprintf("/api/v1/papers/%d/publish", paper.ID), teacher, nil)
	if status != http.StatusOK {
		t.Fatalf("publish: %d %s", status, env.Message)
	}

	student := login(t, ta, username, "Student123")
	status, env = ta.do(t, http.MethodPost, fmt.Sprintf("/api/v1/student/papers/%d/attempts", paper.ID), student, nil)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("start: %d %s", status, env.Message)
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
	ta.decode(t, env, &started)
	var answers []map[string]any
	for _, q := range started.Questions {
		switch q.Type {
		case "single":
			answers = append(answers, map[string]any{"question_id": q.ID, "answer": []string{"A"}})
		case "short":
			answers = append(answers, map[string]any{"question_id": q.ID, "answer": []string{"x=1"}})
		}
	}
	status, env = ta.do(t, http.MethodPut, fmt.Sprintf("/api/v1/student/attempts/%d/answers", started.Attempt.ID), student, map[string]any{"answers": answers})
	if status != http.StatusOK {
		t.Fatalf("save: %d %s", status, env.Message)
	}
	status, env = ta.do(t, http.MethodPost, fmt.Sprintf("/api/v1/student/attempts/%d/submit", started.Attempt.ID), student, map[string]bool{"confirm": true})
	if status != http.StatusOK {
		t.Fatalf("submit: %d %s", status, env.Message)
	}
	var submitted struct {
		Status string `json:"status"`
	}
	ta.decode(t, env, &submitted)
	if submitted.Status != "grading" {
		t.Fatalf("want grading, got %s", submitted.Status)
	}

	waitGraded := func() map[string]any {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			status, env := ta.do(t, http.MethodGet, fmt.Sprintf("/api/v1/teacher/attempts/%d/result", started.Attempt.ID), teacher, nil)
			if status != http.StatusOK {
				t.Fatalf("result: %d %s", status, env.Message)
			}
			var data struct {
				Attempt struct {
					Status          string  `json:"status"`
					Score           float64 `json:"score"`
					ObjectiveScore  float64 `json:"objective_score"`
					SubjectiveScore float64 `json:"subjective_score"`
				} `json:"attempt"`
			}
			ta.decode(t, env, &data)
			if data.Attempt.Status == "graded" {
				return map[string]any{
					"score":      data.Attempt.Score,
					"objective":  data.Attempt.ObjectiveScore,
					"subjective": data.Attempt.SubjectiveScore,
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("grading timeout")
		return nil
	}

	first := waitGraded()
	if first["score"].(float64) != 15 || first["objective"].(float64) != 5 || first["subjective"].(float64) != 10 {
		t.Fatalf("unexpected first grading: %+v", first)
	}

	status, env = ta.do(t, http.MethodPost, fmt.Sprintf("/api/v1/teacher/attempts/%d/regrade", started.Attempt.ID), teacher, nil)
	if status != http.StatusAccepted {
		t.Fatalf("regrade: %d %s", status, env.Message)
	}
	second := waitGraded()
	if second["score"].(float64) != 15 || second["objective"].(float64) != 5 || second["subjective"].(float64) != 10 {
		t.Fatalf("regrade lost scores: %+v", second)
	}
}
