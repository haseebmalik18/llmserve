package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/haseebmalik18/llmserve/internal/model"
)

const help = `llmctl — client CLI for llmserve

Usage:
  llmctl <command> [args]

Commands:
  generate --model PATH --prompt TEXT [--max-tokens N]
      Generate text directly via local llama.cpp (no server needed).
  chat --server URL --prompt TEXT [--max-tokens N]
      Send a prompt to a running llmserve instance and stream the response.
  help
      Show this message.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(help)
		return
	}
	switch os.Args[1] {
	case "help", "-h", "--help":
		fmt.Print(help)
	case "generate":
		if err := runGenerate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "llmctl: %v\n", err)
			os.Exit(1)
		}
	case "chat":
		if err := runChat(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "llmctl: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "llmctl: unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	modelPath := fs.String("model", "", "path to GGUF model file")
	prompt := fs.String("prompt", "", "prompt text")
	maxTokens := fs.Int("max-tokens", 64, "max tokens to generate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *modelPath == "" || *prompt == "" {
		return fmt.Errorf("--model and --prompt are required")
	}

	if err := model.InitBackend(); err != nil {
		return err
	}
	defer model.FreeBackend()

	m, err := model.LoadModel(*modelPath, model.ModelOptions{UseMmap: true})
	if err != nil {
		return err
	}
	defer m.Free()

	ctx, err := m.NewContext(model.DefaultContextOptions())
	if err != nil {
		return err
	}
	defer ctx.Free()

	tokens, err := m.Tokenize(*prompt, true)
	if err != nil {
		return err
	}

	fmt.Print(*prompt)
	if err := ctx.Decode(tokens); err != nil {
		return err
	}

	eos := m.EOSToken()
	for i := 0; i < *maxTokens; i++ {
		logits := ctx.LogitsLast()
		next := model.SampleGreedy(logits)
		if next == eos {
			break
		}
		fmt.Print(m.DetokenizeOne(next))
		if err := ctx.Decode([]model.TokenID{next}); err != nil {
			return err
		}
	}
	fmt.Println()
	return nil
}

func runChat(args []string) error {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "server URL")
	prompt := fs.String("prompt", "", "prompt text")
	maxTokens := fs.Int("max-tokens", 128, "max tokens to generate")
	priority := fs.String("priority", "interactive", "interactive or batch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *prompt == "" {
		return fmt.Errorf("--prompt required")
	}

	body, _ := json.Marshal(map[string]any{
		"prompt":     *prompt,
		"max_tokens": *maxTokens,
		"priority":   *priority,
	})

	req, err := http.NewRequest(http.MethodPost, *serverURL+"/v1/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	fmt.Print(*prompt)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var event string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			event = ""
			continue
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			switch event {
			case "token":
				var ev struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal([]byte(data), &ev); err == nil {
					fmt.Print(ev.Text)
				}
			case "done":
				fmt.Println()
				return nil
			case "error":
				return fmt.Errorf("server: %s", data)
			}
		}
	}
	fmt.Println()
	return scanner.Err()
}
