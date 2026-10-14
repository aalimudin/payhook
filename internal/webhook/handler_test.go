package webhook

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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

func TestSNAPRouteErrorsAreSNAPShaped(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	sp, err := provider.NewSNAP("midtrans-snap", string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})))
	if err != nil {
		t.Fatal(err)
	}
	h := New(store.Store{}, sp)
	for name, c := range map[string]struct {
		ts, sig string
		status  int
		code    string
	}{
		"no signature":  {"2026-10-14T10:00:00+07:00", "", 401, "4012500"},
		"bad timestamp": {"nope", "AAAA", 400, "4002501"},
	} {
		r := httptest.NewRequest("POST", "/webhooks/midtrans-snap/v1.0/transfer-va/payment", strings.NewReader(`{}`))
		r.Header.Set("X-TIMESTAMP", c.ts)
		r.Header.Set("X-SIGNATURE", c.sig)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.code) {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
}
