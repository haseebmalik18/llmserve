package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/haseebmalik18/llmserve/internal/model"
)

const help = `llmctl — client CLI for llmserve

Usage:
  llmctl <command> [args]

Commands:
  generate --model PATH --prompt TEXT [--max-tokens N]
      Generate text using a local GGUF model (requires llama.cpp built via 'make setup'
      and rebuilt with -tags llamacpp).
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
