package fieldservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateUserAvatarIncludesUserID(t *testing.T) {
	const userID = "tech-42"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/user/update/"+userID {
			t.Errorf("path = %q, want user update path", r.URL.Path)
		}
		var body struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.UserID != userID {
			t.Errorf("user_id = %q, want %q", body.UserID, userID)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{}}`))
	}))
	defer server.Close()

	client := NewInfraiClient(server.URL, "test-key", server.Client())
	if err := client.UpdateUserAvatar(context.Background(), userID, "https://example.test/avatar.webp", "request-1"); err != nil {
		t.Fatalf("UpdateUserAvatar() error = %v", err)
	}
}
