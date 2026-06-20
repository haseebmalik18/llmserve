# llmserve

A from-scratch **LLM inference server in Go** with llmserve KV cache, continuous batching, and priority-aware chunked prefill. Inspired by vLLM (SOSP 2023) and Sarathi-Serve (OSDI 2024); the priority-aware scheduling on top is the original contribution.

This is a portfolio / interview-defensibility project for infrastructure engineer intern roles. AI-infra is the target signal — the project exists so the author can speak fluently about LLM serving, llmserve memory management, scheduler tradeoffs, and SLO-aware batching, all of which are 2026-current at NVIDIA, Cloudflare AI, Anthropic, and every model-serving startup.

See `PLAN.md` for the phased roadmap. `DESIGN.md` (added once decisions are real) will hold the rationale.

> **Pivot context:** GitHub repo is still named `lsmkv` — leftover from prior project ideas. Will rename via `gh repo rename` once the project shape stabilizes. Phases 0-2 of the previous direction (in-memory KV, WAL with fuzz tests) shipped to `main`; that code is irrelevant for `llmserve` and will likely be wiped from the repo before a clean Phase 0 commit.

## Tech stack

- **Go 1.23+** for the systems layer — scheduler, KV cache, server, request queues
- **CGO + llama.cpp** for the actual neural-network math (we are not implementing transformers; we are implementing the system around them). [Ollama's architecture](https://github.com/ollama/ollama) is the direct precedent.
- **Stdlib for everything else** — `net/http`, `encoding/json`, `sync`, `context`, `container/heap`, `hash/crc32`
- **`testify/require`** for test asserts
- **No external schedulers, queues, or DBs.** Building it is the point.

## Module path

`github.com/haseebmalik18/lsmkv` for now; will rename alongside the repo.

## Layout

```
cmd/
  llmserve/main.go    the inference daemon (HTTP)
  llmctl/main.go      tiny client CLI (generate, status, bench)
internal/
  model/                 CGO bindings to llama.cpp; forward-pass primitives
  kvcache/               llmserve block manager, allocation, eviction, refcounts
  scheduler/             continuous batching + chunked prefill + priority tiers
  queue/                 request queue with priority lanes (interactive vs batch)
  server/                net/http handlers, streaming SSE for token output
  metrics/               TTFT, TPOT, P50/P99/P999 trackers (expvar)
  workload/              synthetic workload generators for bench/
bench/
  scenarios/             YAML/JSON scenario definitions
  compare/               same workloads against local vLLM for honest A/B
```

Full structure and per-phase deliverables in `PLAN.md`.

## Common commands

```sh
make build         # build llmserve and llmctl
make test          # go test ./...
make race          # go test -race ./...
make bench         # benchmark suite
make lint          # gofmt + go vet
make clean
```

The CGO build requires llama.cpp built locally; `make setup` will pull and build the right version (Phase 1 sets this up).

## Current phase

**Phase 0 — Reboot bootstrap.** Wiping legacy LSM/Raft code, building fresh skeleton for `llmserve`.

## Conventions

- **One concept per file.** `scheduler/chunked_prefill.go` holds chunked prefill and nothing else.
- **Race detector is mandatory** for any code that touches the scheduler, queue, or KV cache.
- **Errors returned, not logged-and-swallowed.** The server / CLI / `main` decides whether to log or exit.
- **No `panic` outside `main`-time setup.** Even "impossible" cases return an error.
- **`context.Context` for any long-running operation** — request handling, generation, model calls. Cancellation must work end-to-end (client disconnects → generation stops → KV cache freed).
- **Decisions in `DESIGN.md` *the day they're made*.** Block size, eviction policy, chunk budget, priority tier definitions, reservation thresholds. The interview ROI is in the *why*.
- **Reference, don't copy.** vLLM source, TGI source, Ollama source, Sarathi-Serve paper, Splitwise/DistServe papers — read to understand, then write our own. Cited prior art is fine; copy-paste makes interview discussion hollow.

## What not to do

- **No training.** Inference serving only.
- **No distributed inference in v1.** Single-node only. Multi-node is a Phase 9 stretch.
- **No custom tokenizer.** Delegate to whatever llama.cpp uses for the loaded model.
- **No transformer math from scratch.** We invoke llama.cpp's forward pass. Implementing flash attention is a different project.
- **No premature optimization.** Build the correct version, measure, then optimize. The scheduling twist (priority-aware chunked prefill) is the optimization that's part of the design — don't add others (llmserve TLB tricks, KV quantization) until benchmarks justify.
- **No web UI / dashboard.** Phase 11 metrics via `expvar` is enough.

## Useful references

These are read-and-understand, not copy-from:

- **PagedAttention / vLLM paper** (Kwon et al., SOSP 2023) — the foundational paper
- **Sarathi-Serve** (Agrawal et al., OSDI 2024) — chunked prefill, our baseline for the twist
- **Splitwise** (Patel et al., ISCA 2024) — prefill/decode disaggregation
- **DistServe** (Zhong et al., OSDI 2024) — same theme, different angle
- **Medusa / EAGLE-2 / Lookahead** (2024) — speculative decoding, the Phase 9 stretch
- **vLLM source** ([github.com/vllm-project/vllm](https://github.com/vllm-project/vllm)) — the canonical Python implementation
- **Ollama source** ([github.com/ollama/ollama](https://github.com/ollama/ollama)) — Go + CGO architecture precedent
- **llama.cpp** ([github.com/ggerganov/llama.cpp](https://github.com/ggerganov/llama.cpp)) — our forward-pass backend

## Author context

Targeting infrastructure engineer intern roles broadly (NVIDIA inference team, Cloudflare AI / Workers AI, Anthropic, OpenAI, Together / Fireworks / Modal / Anyscale, plus general infra like Stripe / Cockroach / Snowflake). Wants honest assessment over validation. Project decisions optimize for *interview-defensibility* — every choice should have a "why" articulable from first principles. Has stated intent to understand all of it at the end, after the build is done.
