package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/haseebmalik18/llmserve/internal/kvcache"
	"github.com/haseebmalik18/llmserve/internal/model"
	"github.com/haseebmalik18/llmserve/internal/queue"
	"github.com/haseebmalik18/llmserve/internal/scheduler"
	"github.com/haseebmalik18/llmserve/internal/server"
)

func main() {
	var (
		addr             = flag.String("addr", ":8080", "HTTP listen address")
		modelPath        = flag.String("model", "", "path to GGUF model file (required)")
		nCtx             = flag.Int("ctx", 4096, "model context size")
		nBlocks          = flag.Int("blocks", 1024, "KV cache total blocks (used in Phase 4+)")
		blockSize        = flag.Int("block-size", 16, "tokens per cache block")
		defaultMaxTokens = flag.Int("default-max-tokens", 256, "default max_tokens when client omits")
	)
	flag.Parse()

	if *modelPath == "" {
		log.Fatal("--model is required")
	}

	if err := model.InitBackend(); err != nil {
		log.Fatalf("init backend: %v", err)
	}
	defer model.FreeBackend()

	slog.Info("loading model", "path", *modelPath)
	m, err := model.LoadModel(*modelPath, model.ModelOptions{UseMmap: true})
	if err != nil {
		log.Fatalf("load model: %v", err)
	}
	defer m.Free()
	slog.Info("model loaded", "vocab", m.NVocab())

	if _, err := kvcache.NewManager(kvcache.ManagerOptions{
		BlockSize: *blockSize,
		NumBlocks: *nBlocks,
	}); err != nil {
		log.Fatalf("kvcache: %v", err)
	}

	lanes := queue.NewLanes()
	backend := scheduler.NewModelBackend(m, model.ContextOptions{NCtx: *nCtx, NBatch: 512})
	sched := scheduler.New(lanes, backend)
	srv := server.New(server.Config{
		Addr:             *addr,
		DefaultMaxTokens: *defaultMaxTokens,
	}, lanes)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutdown signal received", "sig", sig.String())
		cancel()
	}()

	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		if err := sched.Run(rootCtx); err != nil {
			slog.Error("scheduler exited with error", "err", err)
		}
	}()

	slog.Info("server starting", "addr", *addr)
	if err := srv.Run(rootCtx); err != nil {
		slog.Error("server exited with error", "err", err)
	}

	lanes.Close()
	select {
	case <-schedDone:
	case <-time.After(5 * time.Second):
		slog.Warn("scheduler did not exit within 5s")
	}
	slog.Info("shutdown complete")
}
