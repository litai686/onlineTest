package ai_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
	"onlineTest/internal/security"
	"onlineTest/internal/store"
)

func TestClientChatAndLogging(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	st := store.New(db)
	secret := []byte("secret")
	cipher, err := security.NewCipher(secret)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt("sk-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p := &domain.AIProvider{
		Name: "mock", BaseURL: "http://example.invalid/v1", APIKeyCipher: enc, APIKeyHint: "****test",
		Model: "mock-1", SupportsJSON: true, Temperature: 0.1, TimeoutSec: 10, Concurrency: 1, Status: 1,
	}
	if err := st.CreateProvider(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, "ai.default", "1"); err != nil {
		t.Fatal(err)
	}

	var gotAuth string
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		lastBody = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&lastBody)
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "pong"}}},
			"usage":   map[string]int{"prompt_tokens": 3, "completion_tokens": 2},
		})
	}))
	defer srv.Close()
	p.BaseURL = srv.URL + "/v1"
	if err := st.UpdateProvider(ctx, p.ID, map[string]any{"base_url": p.BaseURL}); err != nil {
		t.Fatal(err)
	}

	client := ai.NewClient(st, cipher)
	res, err := client.Chat(ctx, p, ai.ChatRequest{
		Messages: []ai.Message{{Role: "user", Content: "ping"}},
	}, "test", "")
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if res.Content != "pong" {
		t.Fatalf("want pong, got %q", res.Content)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	if _, ok := lastBody["max_tokens"]; ok {
		t.Fatal("max_tokens must not be sent")
	}
	if _, ok := lastBody["response_format"]; ok {
		t.Fatal("response_format should only be sent when requested")
	}
	if _, err := client.Chat(ctx, p, ai.ChatRequest{
		Messages:       []ai.Message{{Role: "user", Content: "json please"}},
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	}, "test", ""); err != nil {
		t.Fatalf("chat with json: %v", err)
	}
	if _, ok := lastBody["response_format"]; !ok {
		t.Fatal("response_format missing for json request")
	}
	if _, ok := lastBody["max_tokens"]; ok {
		t.Fatal("max_tokens must not be sent")
	}
	p.ThinkingMode = "high"
	if _, err := client.Chat(ctx, p, ai.ChatRequest{
		Messages: []ai.Message{{Role: "user", Content: "think"}},
	}, "test", ""); err != nil {
		t.Fatalf("chat with thinking: %v", err)
	}
	if got, _ := lastBody["reasoning_effort"].(string); got != "high" {
		t.Fatalf("reasoning_effort = %q, want high", got)
	}
	var calls []domain.AICall
	if err := db.Find(&calls).Error; err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0].PromptTokens != 3 || calls[0].CompletionTokens != 2 {
		t.Fatalf("unexpected ai calls: %+v", calls)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"```json\n{\"a\":1}\n```", "{\"a\":1}"},
		{"前缀说明 {\"a\":\"}{\"} 后缀", "{\"a\":\"}{\"}"},
		{"[{\"a\":1}]", "[{\"a\":1}]"},
	}
	for _, c := range cases {
		got, err := ai.ExtractJSON(c.raw)
		if err != nil {
			t.Fatalf("extract %q: %v", c.raw, err)
		}
		var a, b any
		if json.Unmarshal(got, &a) != nil || json.Unmarshal([]byte(c.want), &b) != nil || !deepEqualJSON(a, b) {
			t.Fatalf("extract %q = %s, want %s", c.raw, got, c.want)
		}
	}
}

func deepEqualJSON(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	os.Exit(m.Run())
}
