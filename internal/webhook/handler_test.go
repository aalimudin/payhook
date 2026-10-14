package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/payhook/payhook/internal/provider"
	"github.com/payhook/payhook/internal/store"
)

// Verification and size limits run before any DB access, so no database is needed here.
func TestRejectsBeforeTouchingDB(t *testing.T) {
	h := New(store.Store{}, provider.Xendit{CallbackToken: "tok"})
	do := func(path, token, body string) int {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("X-Callback-Token", token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := do("/webhooks/xendit", "", `{}`); c != http.StatusUnauthorized {
		t.Errorf("no token: %d", c)
	}
	if c := do("/webhooks/xendit", "bad", `{}`); c != http.StatusUnauthorized {
		t.Errorf("bad token: %d", c)
	}
	if c := do("/webhooks/nope", "tok", `{}`); c != http.StatusNotFound {
		t.Errorf("unknown provider: %d", c)
	}
	if c := do("/webhooks/xendit", "tok", strings.Repeat("a", 2<<20)); c != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize: %d", c)
	}
}
