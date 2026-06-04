package model

import "errors"

type TokenID int32

const InvalidToken TokenID = -1

type ModelOptions struct {
	UseMmap bool
}

type ContextOptions struct {
	NCtx   int
	NBatch int
	Seed   uint32
}

func DefaultContextOptions() ContextOptions {
	return ContextOptions{NCtx: 4096, NBatch: 512, Seed: 0xDEADBEEF}
}

var (
	ErrModelLoad        = errors.New("model: load failed")
	ErrContextInit      = errors.New("model: context init failed")
	ErrTokenize         = errors.New("model: tokenize failed")
	ErrDecode           = errors.New("model: decode failed")
	ErrNotInitialized   = errors.New("model: backend not initialized")
	ErrCGONotAvailable  = errors.New("model: CGO bindings not built — run `make setup` and rebuild")
)
