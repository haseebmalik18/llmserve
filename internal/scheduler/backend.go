package scheduler

import (
	"context"

	"github.com/haseebmalik18/llmserve/internal/model"
)

type Backend interface {
	StartGeneration(ctx context.Context, prompt string) (Generation, error)
}

type Generation interface {
	Next(ctx context.Context) (text string, done bool, err error)
	Close()
}

type modelBackend struct {
	m       *model.Model
	ctxOpts model.ContextOptions
}

func NewModelBackend(m *model.Model, opts model.ContextOptions) Backend {
	return &modelBackend{m: m, ctxOpts: opts}
}

func (b *modelBackend) StartGeneration(_ context.Context, prompt string) (Generation, error) {
	mctx, err := b.m.NewContext(b.ctxOpts)
	if err != nil {
		return nil, err
	}
	tokens, err := b.m.Tokenize(prompt, true)
	if err != nil {
		mctx.Free()
		return nil, err
	}
	if err := mctx.Decode(tokens); err != nil {
		mctx.Free()
		return nil, err
	}
	return &modelGen{m: b.m, ctx: mctx, eos: b.m.EOSToken()}, nil
}

type modelGen struct {
	m   *model.Model
	ctx *model.Context
	eos model.TokenID
}

func (g *modelGen) Next(ctx context.Context) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	logits := g.ctx.LogitsLast()
	if logits == nil {
		return "", false, ErrNoLogits
	}
	token := model.SampleGreedy(logits)
	if token == g.eos {
		return "", true, nil
	}
	piece := g.m.DetokenizeOne(token)
	if err := g.ctx.Decode([]model.TokenID{token}); err != nil {
		return "", false, err
	}
	return piece, false, nil
}

func (g *modelGen) Close() {
	g.ctx.Free()
}
