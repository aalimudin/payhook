package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/payhook/payhook/internal/outbox"
	"github.com/payhook/payhook/internal/provider"
	"github.com/payhook/payhook/internal/store"
	"github.com/payhook/payhook/internal/webhook"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.Store{DB: db}
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	var provs []provider.Provider
	if k := os.Getenv("MIDTRANS_SERVER_KEY"); k != "" {
		provs = append(provs, provider.Midtrans{ServerKey: k})
	}
	if k := os.Getenv("XENDIT_CALLBACK_TOKEN"); k != "" {
		provs = append(provs, provider.Xendit{CallbackToken: k})
	}
	if k := os.Getenv("STRIPE_WEBHOOK_SECRET"); k != "" {
		provs = append(provs, provider.Stripe{Secret: k})
	}
	if len(provs) == 0 {
		return errors.New("no provider secret configured")
	}

	maxAttempts, _ := strconv.Atoi(env("MAX_ATTEMPTS", "8"))
	w := &outbox.Worker{DB: db, URL: os.Getenv("FORWARD_URL"), MaxAttempts: maxAttempts, Batch: 10,
		Poll: time.Second, BaseDelay: 2 * time.Second, MaxDelay: time.Hour}
	if w.URL == "" {
		return errors.New("FORWARD_URL is required")
	}
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	srv := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: webhook.New(st, provs...), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-done
	return nil
}
