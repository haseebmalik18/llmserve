package queue

import "context"

type Priority int

const (
	Interactive Priority = iota
	Batch
)

func (p Priority) String() string {
	switch p {
	case Interactive:
		return "interactive"
	case Batch:
		return "batch"
	}
	return "unknown"
}

type TokenEvent struct {
	Text string
	Done bool
	Err  error
}

type Request struct {
	ID        uint64
	Prompt    string
	MaxTokens int
	Priority  Priority
	Ctx       context.Context
	Tokens    chan TokenEvent
}
