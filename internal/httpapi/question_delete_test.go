package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func seedJuniorMath(t *testing.T, app *testApp, teacher string) int64 {
	t.Helper()
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
	for _, s := range subjects.Items {
		if s.Stage == "junior" && s.Name == "数学" {
			return s.ID
		}
	}
	t.Fatal("junior math subject not seeded")
	return 0
}

func TestDeleteQuestionWithAnswers(t *testing.T) {
	app := newTestApp(t)
	teacher := login(t, app, "admin", "Admin12345")
	subjectID := seedJuniorMath(t, app, teacher)

	username := "stu_del_q"
	status, env := app.do(t, http.MethodPost, "/api/v1/students", teacher, map[string]any{
		"username": username, "display_name": "删除测试学生", "password": "Student123",
	})
	if status != http.StatusCreated {
		t.Fatalf("create student: %d %s", status, env.Message)
	}

	status, env = app.do(t, http.MethodPost, "/api/v1/papers", teacher, map[string]any{
		"title": "删除题目测试卷", "subject_id": subjectID, "grade": "初二", "duration_min": 30,
	})
	if status != http.StatusCreated {
		t.Fatalf("create paper: %d %s", status, env.Message)
	}
	var paper struct {
		ID int64 `json:"id"`
	}
	app.decode(t, env, &paper)

	var questionIDs []int64
	for _, stem := range []string{"1+1=?", "2+2=?"} {
		status, env = app.do(t, http.MethodPost, fmt.Sprintf("/api/v1/papers/%d/questions", paper.ID), teacher, map[string]any{
			"type": "single", "stem": stem,
			"options": []map[string]string{{"key": "A", "text": "2"}, {"key": "B", "text": "4"}},
			"answer":  []string{"A"}, "score": 5,
		})
		if status != http.StatusCreated {
			t.Fatalf("add question: %d %s", status, env.Message)
		}
		var created struct {
			ID int64 `json:"id"`
		}
		app.decode(t, env, &created)
		questionIDs = append(questionIDs, created.ID)
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
	}
	app.decode(t, env, &started)

	status, env = app.do(t, http.MethodPut, fmt.Sprintf("/api/v1/student/attempts/%d/answers", started.Attempt.ID), student, map[string]any{
		"answers": []map[string]any{{"question_id": questionIDs[0], "answer": []string{"A"}}},
	})
	if status != http.StatusOK {
		t.Fatalf("save answers: %d %s", status, env.Message)
	}

	// 未提交答卷中的作答不应阻止删除。
	status, env = app.do(t, http.MethodDelete, fmt.Sprintf("/api/v1/papers/%d/questions/%d", paper.ID, questionIDs[0]), teacher, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete question in progress attempt: want 204, got %d %s", status, env.Message)
	}

	status, env = app.do(t, http.MethodGet, fmt.Sprintf("/api/v1/papers/%d", paper.ID), teacher, nil)
	if status != http.StatusOK {
		t.Fatalf("get paper: %d %s", status, env.Message)
	}
	var detail struct {
		Questions []struct {
			ID int64 `json:"id"`
		} `json:"questions"`
	}
	app.decode(t, env, &detail)
	if len(detail.Questions) != 1 || detail.Questions[0].ID != questionIDs[1] {
		t.Fatalf("unexpected questions after delete: %+v", detail.Questions)
	}

	if _, env = app.do(t, http.MethodPut, fmt.Sprintf("/api/v1/student/attempts/%d/answers", started.Attempt.ID), student, map[string]any{
		"answers": []map[string]any{{"question_id": questionIDs[1], "answer": []string{"A"}}},
	}); env.Code != 0 {
		t.Fatalf("save answers: %s", env.Message)
	}
	if status, env = app.do(t, http.MethodPost, fmt.Sprintf("/api/v1/student/attempts/%d/submit", started.Attempt.ID), student, map[string]bool{"confirm": true}); status != http.StatusOK {
		t.Fatalf("submit: %d %s", status, env.Message)
	}

	// 已提交答卷引用的题目应返回友好提示而不是 500。
	status, env = app.do(t, http.MethodDelete, fmt.Sprintf("/api/v1/papers/%d/questions/%d", paper.ID, questionIDs[1]), teacher, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("delete submitted question: want 422, got %d %s", status, env.Message)
	}
	if !strings.Contains(env.Message, "答卷") || strings.Contains(env.Message, "constraint") {
		t.Fatalf("unexpected message: %s", env.Message)
	}
}
