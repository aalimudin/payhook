// Package webhook is the HTTP entry point for provider callbacks.
package webhook

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/payhook/payhook/internal/provider"
	"github.com/payhook/payhook/internal/store"
)

const maxBody = 1 << 20 // 1 MB

func New(st store.Store, providers ...provider.Provider) http.Handler {
	byName := map[string]provider.Provider{}
	for _, p := range providers {
		byName[p.Name()] = p
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.DB.PingContext(r.Context()); err != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("POST /webhooks/{provider}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := byName[r.PathValue("provider")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
			return
		}
		if err := p.Verify(r, body); err != nil {
			slog.Warn("verification failed", "provider", p.Name(), "err", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ev, err := p.Parse(body)
		if errors.Is(err, provider.ErrIgnored) {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		outcome, err := st.Record(r.Context(), p.Name(), ev)
		if err != nil {
			slog.Error("record failed", "provider", p.Name(), "event", ev.ID, "err", err)
			http.Error(w, "try again", http.StatusInternalServerError) // provider will retry
			return
		}
		slog.Info("webhook", "provider", p.Name(), "event", ev.ID, "outcome", outcome)
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
