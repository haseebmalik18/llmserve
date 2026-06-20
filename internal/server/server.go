package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/haseebmalik18/llmserve/internal/queue"
)

type Config struct {
	Addr             string
	DefaultMaxTokens int
}

type Server struct {
	cfg    Config
	queue  *queue.Lanes
	http   *http.Server
	nextID atomic.Uint64
	log    *slog.Logger
}

func New(cfg Config, q *queue.Lanes) *Server {
	if cfg.DefaultMaxTokens <= 0 {
		cfg.DefaultMaxTokens = 256
	}
	s := &Server{cfg: cfg, queue: q, log: slog.Default()}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/generate", s.handleGenerate)
	s.http = &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok\n"))
}
