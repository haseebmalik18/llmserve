package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/haseebmalik18/llmserve/internal/queue"
)

type generateRequest struct {
	Prompt    string `json:"prompt"`
	MaxTokens int    `json:"max_tokens"`
	Priority  string `json:"priority"`
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body generateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	if body.MaxTokens <= 0 {
		body.MaxTokens = s.cfg.DefaultMaxTokens
	}
	priority := queue.Interactive
	if body.Priority == "batch" {
		priority = queue.Batch
	}

	req := &queue.Request{
		ID:        s.nextID.Add(1),
		Prompt:    body.Prompt,
		MaxTokens: body.MaxTokens,
		Priority:  priority,
		Ctx:       r.Context(),
		Tokens:    make(chan queue.TokenEvent, 16),
	}
	if err := s.queue.Enqueue(req); err != nil {
		http.Error(w, "queue closed", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	for ev := range req.Tokens {
		if ev.Err != nil {
			data, _ := json.Marshal(map[string]string{"error": ev.Err.Error()})
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
			flusher.Flush()
			return
		}
		if ev.Done {
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
			return
		}
		data, _ := json.Marshal(map[string]string{"text": ev.Text})
		fmt.Fprintf(w, "event: token\ndata: %s\n\n", data)
		flusher.Flush()
	}
}
