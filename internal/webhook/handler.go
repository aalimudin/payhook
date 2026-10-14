// Package webhook is the HTTP entry point for provider callbacks.
package webhook

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

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
		serve(st, byName, w, r, plainReply)
	})
	// BI-SNAP senders append the standard path to the notification URL, and it is part of the signed string.
	mux.HandleFunc("POST /webhooks/{provider}/v1.0/transfer-va/payment", func(w http.ResponseWriter, r *http.Request) {
		serve(st, byName, w, r, snapReply)
	})
	return mux
}

// reply writes an outcome: HTTP status, a 7-digit SNAP code, a message, and the provider's ack body for success.
type reply func(w http.ResponseWriter, status int, snapCode, msg string, ack map[string]any)

func plainReply(w http.ResponseWriter, status int, _, msg string, _ map[string]any) {
	if status == http.StatusOK {
		w.WriteHeader(status)
		return
	}
	http.Error(w, msg, status)
}

func snapReply(w http.ResponseWriter, status int, code, msg string, ack map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-TIMESTAMP", time.Now().Format(time.RFC3339))
	w.WriteHeader(status)
	if ack == nil {
		ack = map[string]any{"responseCode": code, "responseMessage": msg}
	}
	json.NewEncoder(w).Encode(ack)
}

func serve(st store.Store, byName map[string]provider.Provider, w http.ResponseWriter, r *http.Request, out reply) {
	p, ok := byName[r.PathValue("provider")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		out(w, http.StatusRequestEntityTooLarge, "4132500", "body too large or unreadable", nil)
		return
	}
	if err := p.Verify(r, body); err != nil {
		slog.Warn("verification failed", "provider", p.Name(), "err", err)
		if errors.Is(err, provider.ErrBadTimestamp) {
			out(w, http.StatusBadRequest, "4002501", "invalid timestamp", nil)
			return
		}
		out(w, http.StatusUnauthorized, "4012500", "unauthorized", nil)
		return
	}
	ev, err := p.Parse(body)
	if errors.Is(err, provider.ErrIgnored) {
		out(w, http.StatusOK, "2002500", "Successful", ackFor(p, body))
		return
	}
	if err != nil {
		out(w, http.StatusBadRequest, "4002502", "bad payload", nil)
		return
	}
	outcome, err := st.Record(r.Context(), p.Name(), ev)
	if err != nil {
		slog.Error("record failed", "provider", p.Name(), "event", ev.ID, "err", err)
		out(w, http.StatusInternalServerError, "5002500", "try again", nil) // provider will retry
		return
	}
	slog.Info("webhook", "provider", p.Name(), "event", ev.ID, "outcome", outcome)
	out(w, http.StatusOK, "2002500", "Successful", ackFor(p, body))
}

func ackFor(p provider.Provider, body []byte) map[string]any {
	if a, ok := p.(interface{ Ack([]byte) map[string]any }); ok {
		return a.Ack(body)
	}
	return nil
}
