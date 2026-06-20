package scheduler

import (
	"context"
	"errors"
	"log/slog"

	"github.com/haseebmalik18/llmserve/internal/queue"
)

var (
	ErrNoLogits = errors.New("scheduler: model returned nil logits")
)

type Scheduler struct {
	queue   *queue.Lanes
	backend Backend
	log     *slog.Logger
}

func New(q *queue.Lanes, b Backend) *Scheduler {
	return &Scheduler{queue: q, backend: b, log: slog.Default()}
}

func (s *Scheduler) Run(ctx context.Context) error {
	for {
		req, err := s.queue.Dequeue(ctx)
		if err != nil {
			if errors.Is(err, queue.ErrClosed) {
				return nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}
		s.handle(ctx, req)
	}
}

func (s *Scheduler) handle(parentCtx context.Context, req *queue.Request) {
	defer close(req.Tokens)

	reqCtx, cancel := mergeCtx(parentCtx, req.Ctx)
	defer cancel()

	gen, err := s.backend.StartGeneration(reqCtx, req.Prompt)
	if err != nil {
		s.emit(reqCtx, req, queue.TokenEvent{Err: err})
		return
	}
	defer gen.Close()

	for i := 0; i < req.MaxTokens; i++ {
		if err := reqCtx.Err(); err != nil {
			return
		}
		text, done, err := gen.Next(reqCtx)
		if err != nil {
			s.emit(reqCtx, req, queue.TokenEvent{Err: err})
			return
		}
		if done {
			s.emit(reqCtx, req, queue.TokenEvent{Done: true})
			return
		}
		if !s.emit(reqCtx, req, queue.TokenEvent{Text: text}) {
			return
		}
	}
	s.emit(reqCtx, req, queue.TokenEvent{Done: true})
}

func (s *Scheduler) emit(ctx context.Context, req *queue.Request, ev queue.TokenEvent) bool {
	select {
	case req.Tokens <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func mergeCtx(a, b context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(a)
	go func() {
		select {
		case <-b.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}
