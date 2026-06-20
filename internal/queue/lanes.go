package queue

import (
	"context"
	"errors"
	"sync"
)

var ErrClosed = errors.New("queue: closed")

type Lanes struct {
	mu          sync.Mutex
	interactive []*Request
	batch       []*Request
	notify      chan struct{}
	closeCh     chan struct{}
	closed      bool
}

func NewLanes() *Lanes {
	return &Lanes{
		notify:  make(chan struct{}, 1),
		closeCh: make(chan struct{}),
	}
}

func (l *Lanes) Enqueue(r *Request) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return ErrClosed
	}
	switch r.Priority {
	case Interactive:
		l.interactive = append(l.interactive, r)
	case Batch:
		l.batch = append(l.batch, r)
	default:
		l.interactive = append(l.interactive, r)
	}
	l.mu.Unlock()
	select {
	case l.notify <- struct{}{}:
	default:
	}
	return nil
}

func (l *Lanes) Dequeue(ctx context.Context) (*Request, error) {
	for {
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			return nil, ErrClosed
		}
		if len(l.interactive) > 0 {
			r := l.interactive[0]
			l.interactive = l.interactive[1:]
			l.mu.Unlock()
			return r, nil
		}
		if len(l.batch) > 0 {
			r := l.batch[0]
			l.batch = l.batch[1:]
			l.mu.Unlock()
			return r, nil
		}
		l.mu.Unlock()
		select {
		case <-l.notify:
		case <-l.closeCh:
			return nil, ErrClosed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (l *Lanes) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	close(l.closeCh)
}

func (l *Lanes) Depth() (interactive, batch int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.interactive), len(l.batch)
}
