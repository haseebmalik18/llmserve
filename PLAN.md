# llmserve — Build Plan

A small but real **LLM inference server in Go** with llmserve KV cache, continuous batching, and priority-aware chunked prefill. Built incrementally; each phase ends in something that runs and is benchmarkable.

The point isn't to compete with vLLM. The point is to have **built every layer ourselves** — server, queue, scheduler, KV cache, batching, chunked prefill, priority lanes — so the design decisions can be defended from first principles.

> **Reboot note:** Prior LSM and Raft work is being wiped. Phase 0 starts the directory fresh (keeping only `go.mod`, `.gitignore`, `Makefile` skeleton). Repo on GitHub (`lsmkv`) will be force-reset and renamed.

---

## What makes this stand out in 2026

These four angles are folded into the phases below.

1. **Priority-aware chunked prefill** *(Phase 6 — the original twist)*
   Chunked prefill (Sarathi-Serve, 2024) breaks long prompts into chunks to avoid head-of-line blocking. We add **priority tiers** — interactive requests get a decode-bandwidth reservation even when prefill demand is high. Measure P99 latency for short-prompt users under mixed workloads against the standard scheduler; chart the win.

2. **Paged KV cache** *(Phase 2 — foundational)*
   Fixed-size blocks, refcounted, dynamically allocated. The same pattern vLLM uses; the implementation here is the systems-engineering signal.

3. **Honest comparison to vLLM** *(Phase 10)*
   Run identical workloads against local vLLM and our impl. Publish the chart. We'll come in slower; the credibility move is the *writeup* — naming specifically which optimizations vLLM has that we don't (Flash Attention, kernel fusion, multi-GPU sharding, Triton).

4. **Adversarial workload bench** *(Phase 7)*
   Find workloads where the standard scheduler degrades (long prefill + short decode mix, bursty arrivals, mixed priorities). Show the standard scheduler's P99 latency spike. Show ours doesn't. Real numbers, not synthetic-uniform.

The first is novelty. The second is depth. The third is humility. The fourth is the "I noticed this problem" story interviewers want.

---

## Tech stack

| Concern         | Choice                                              |
| --------------- | --------------------------------------------------- |
| Language        | Go 1.23+                                            |
| Module path     | `github.com/haseebmalik18/lsmkv` (will rename)      |
| Model backend   | `llama.cpp` via CGO bindings (same architecture as Ollama) |
| Server          | `net/http` with SSE for token streaming             |
| Dependencies    | Stdlib for everything except llama.cpp bindings. `testify` for tests. **No external schedulers, queues, or DBs.** |
| Testing         | stdlib `testing` + `testify/require`; race detector mandatory |

---

## Target folder layout

```
llmserve/
├── cmd/
│   ├── llmserve/main.go       inference daemon (HTTP)
│   └── llmctl/main.go         client CLI: generate, status, bench
├── internal/
│   ├── model/                    CGO bindings to llama.cpp
│   │   ├── llama.go              forward-pass interface
│   │   ├── llama_cgo.go          CGO glue (build tag: cgo)
│   │   └── tokenizer.go          tokenize/detokenize via llama.cpp
│   ├── kvcache/
│   │   ├── block.go              fixed-size block, refcount
│   │   ├── manager.go            allocate/free, eviction
│   │   └── seqmap.go             seqID → block list
│   ├── queue/
│   │   ├── request.go            Request type, priority tiers
│   │   └── lanes.go              interactive + batch lanes
│   ├── scheduler/
│   │   ├── scheduler.go          main scheduling loop
│   │   ├── continuous_batch.go   continuous batching
│   │   ├── chunked_prefill.go    chunked prefill (Sarathi-Serve baseline)
│   │   └── priority.go           priority-aware reservation (the twist)
│   ├── server/
│   │   ├── server.go             net/http server, route registration
│   │   └── generate.go           /v1/generate, SSE streaming
│   ├── metrics/
│   │   └── metrics.go            TTFT, TPOT, P50/P99/P999 via expvar
│   └── workload/
│       ├── generator.go          synthetic workload generator
│       └── scenarios.go          named workload mixes (mixed-len, bursty, etc.)
├── bench/
│   ├── run.go                    runs scenarios, collects metrics
│   └── compare/                  same scenarios against local vLLM
├── models/                       downloaded GGUF files (gitignored)
├── Makefile
├── go.mod / go.sum
├── CLAUDE.md
├── PLAN.md
└── DESIGN.md                     written as decisions are made
```

---

## Phases

Each phase: **scope → deliverables → acceptance criteria.** Don't start phase N+1 until N is green under `-race` and acceptance passes.

### Phase 0 — Reboot bootstrap

**Scope:** clean slate repo.

- Wipe `internal/db`, `internal/wal`, `internal/raft`, `internal/sim`, `internal/testcluster`, `cmd/lsmctl`
- New `cmd/llmserve/main.go` and `cmd/llmctl/main.go` print help text
- Updated `Makefile`: `build`, `test`, `race`, `bench`, `lint`, `clean`, `setup` (the last fetches/builds llama.cpp)
- `.gitignore` updated for `models/` (GGUF files are large)
- First clean commit on a clean `main`

**Acceptance:** `make build` produces both binaries; both run and print help.

---

### Phase 1 — Model integration via CGO

**Scope:** wire llama.cpp so we can actually generate tokens, synchronously, one request at a time.

- `internal/model/llama_cgo.go`: CGO bindings to llama.cpp. Functions: `LoadModel(path)`, `NewContext(modelHandle, opts)`, `Tokenize(text)`, `Decode(tokenIDs)`, `Forward(ctx, tokens) → logits`, `SampleGreedy(logits) → tokenID`.
- `internal/model/llama.go`: Go-friendly wrapper around the CGO surface; the rest of the codebase only sees this.
- `make setup` script that pulls a pinned llama.cpp version and builds the static library locally.
- Bench: download Phi-3-mini or Llama 3.2 1B GGUF, run a single end-to-end "Hello, world" generation from `cmd/llmctl`.

**Decision to record in DESIGN.md:** pinned llama.cpp commit, model choice and why (we want something small enough to run on a laptop without GPU but real enough to validate end-to-end).

**Acceptance:** `llmctl generate "Hello"` produces real tokens via the model. Single-request only; no scheduling yet.

---

### Phase 2 — Paged KV cache

**Scope:** the foundational data structure of the project.

- `internal/kvcache/block.go`: fixed-size `Block` (e.g., 16 tokens of K and V tensors per block). Refcounted.
- `internal/kvcache/manager.go`: `Allocate(n int) ([]BlockID, error)`, `Free(blockIDs)`, `Ref(id)`, `Unref(id)`. LRU eviction when full.
- `internal/kvcache/seqmap.go`: `seqID → []BlockID`. Tracks which blocks each in-flight sequence owns.
- Configurable: block size, total blocks (capacity).
- Unit tests with property test: random allocate/free sequences against a `map[BlockID]int` refcount ground truth.

**Why block-based:** identical to virtual memory pages. Avoids fragmentation, makes prefix-sharing possible later. The same idea as vLLM's PagedAttention.

**Acceptance:** race-clean under `-count=10`; property test green over 1000 random sequences; cache pressure scenarios (allocate beyond capacity) trigger correct eviction.

---

### Phase 3 — Request lifecycle + FIFO scheduler

**Scope:** end-to-end single-request inference through the server, with the scheduler stub.

- `internal/queue/request.go`: `Request{ID, Prompt, MaxTokens, Priority, ResultCh, Done}`.
- `internal/queue/lanes.go`: two priority lanes — `Interactive` (default) and `Batch`. Phase 6 will use both; for now everything goes interactive.
- `internal/scheduler/scheduler.go`: main loop. Picks one request from the head of the interactive lane, runs full prefill, then decode until EOS or `MaxTokens`. One request at a time (no batching yet).
- `internal/server/server.go`: `POST /v1/generate` accepts a prompt, creates a request, registers it on the queue, returns SSE-streamed tokens as they're produced.
- `internal/server/generate.go`: SSE handler, cancels generation on client disconnect.

**Acceptance:** end-to-end test: spin up server, hit `/v1/generate` from `llmctl`, get streamed tokens, server returns when done.

---

### Phase 4 — Continuous batching

**Scope:** the throughput multiplier.

- Scheduler runs in a continuous loop: each "step" advances all batched sequences by one decode token (or some chunk of prefill).
- New requests join the batch as room opens up; finished requests leave the batch mid-step.
- Configurable: `maxBatchSize` (max concurrent sequences), `tokenBudgetPerStep`.
- Benchmark: throughput at 1, 4, 8, 16 concurrent requests on a synthetic workload of identical short prompts. Must scale roughly linearly until saturation.

**Acceptance:** `bench/run.go` shows continuous batching is ≥ 3× throughput of one-at-a-time on a synthetic identical-prompt workload at 8 concurrent requests.

---

### Phase 5 — Chunked prefill (Sarathi-Serve baseline)

**Scope:** baseline mechanism for the priority twist.

- Long prefills (> `prefillChunkSize`, e.g., 256 tokens) get processed in chunks rather than all at once.
- Each scheduling step has a `tokenBudgetPerStep` that's split between prefill chunks and decode tokens.
- Without priority logic yet — this phase just implements chunked prefill as a fair-share mechanism.
- Reference: Sarathi-Serve OSDI 2024 paper.

**Acceptance:**
- A request with a 4096-token prompt does not block a request with a 16-token prompt from starting decode.
- P99 short-prompt latency drops vs Phase 4 baseline when a long-prefill request is in flight.
- DESIGN.md entry on chunk size choice.

---

### Phase 6 — Priority-aware reservation *(the twist)*

**Scope:** the original contribution.

- Two priority lanes already exist (`Interactive`, `Batch`).
- New scheduling rule: **the scheduler reserves a configurable fraction of `tokenBudgetPerStep` for decode tokens of `Interactive` requests** that have already begun generation. Prefill chunks (regardless of priority) can use the remainder.
- Result: interactive users' decode latency stays bounded even when a 50k-token batch request is mid-prefill.
- Configurable: `interactiveDecodeReservation` (0.0 to 1.0). At 0.0, behaves like Phase 5. At 1.0, prefill is fully starved while any interactive decode is active.

**Acceptance:**
- `bench/scenarios/mixed_priority.yaml` defines an adversarial workload: 1 batch request with a 32k-token prompt + 20 interactive requests with 32-token prompts.
- P99 latency for interactive users under that workload:
  - Phase 4 baseline (continuous batching only): expected ~seconds
  - Phase 5 (chunked prefill, no priority): improved, still bad
  - Phase 6 (priority reservation): bounded at hundreds of milliseconds
- Chart in `bench/results/`.

---

### Phase 7 — Adversarial workload bench

**Scope:** the numbers that make the twist defensible.

- `internal/workload/generator.go`: parameterized synthetic workload (prompt length distribution, arrival rate, priority mix, request duration).
- `internal/workload/scenarios.go`: named scenarios:
  - `uniform-short`: all 32-token prompts, control case
  - `mixed-len`: 90% short / 10% long-prefill
  - `bursty`: requests arrive in bursts of 20 every 5s
  - `priority-mix`: mixed-len + 50/50 interactive/batch priority
  - `pathological`: the worst-case mix for the standard scheduler
- `bench/run.go`: runs a scenario against the server, collects per-request TTFT, TPOT, total latency; emits CSV.

**Acceptance:** all five scenarios run cleanly; CSV output has the columns to feed Phase 10's comparison chart.

---

### Phase 8 — Speculative decoding *(stretch / Phase 8 of core)*

**Scope:** the second optional optimization. Skip if running short on time.

- `internal/model/draft.go`: load a smaller draft model (e.g., TinyLlama 1.1B if main is Phi-3 3.8B).
- Per request: draft generates `specDepth` tokens, main model verifies in parallel. Accepted tokens become the actual output; rejected tokens fall back to greedy decode.
- Adaptive `specDepth`: tracks per-session acceptance rate, scales depth up when high, down when low.
- Bench: friendly workload (continuation of common prefixes) vs adversarial workload (out-of-distribution prompts). Show the adaptive depth adjusts.

**Acceptance:** ≥ 1.5× speedup on friendly workload, ≤ 5% slowdown on adversarial workload.

---

### Phase 9 — vLLM comparison

**Scope:** honest A/B against the reference implementation.

- Install vLLM locally (single command via `pip install vllm`); start it serving the same GGUF/HF model.
- `bench/compare/`: run identical scenarios against both servers, collect same metrics.
- Publish the chart: throughput and P99 latency, ours vs vLLM, scenario by scenario.
- Honest gap analysis in DESIGN.md:
  - vLLM has Flash Attention 2/3 — we don't (~3-5× attention throughput)
  - vLLM has Triton kernels — we use llama.cpp's CPU kernels
  - vLLM has multi-GPU tensor parallelism — we're single-machine
  - vLLM has continuous batching with prefix caching — we don't have prefix caching
  - Name each one specifically.

**Acceptance:** comparison chart exists; DESIGN.md gap analysis names at least 4 specific optimizations vLLM has that we don't, with one-line explanations of each.

---

### Phase 10 — DESIGN.md + numbers section in README

**Scope:** the writeup that turns the project from "I built it" into "I built it and can defend every decision."

- DESIGN.md covers, at minimum:
  - Block size: why 16 tokens
  - Eviction policy: why LRU not LFU
  - Chunk budget: how chosen, what happens at boundaries
  - Priority reservation: why a fraction not an absolute count
  - Backend choice: llama.cpp vs alternatives
  - Failure modes: what doesn't work well, 5+ scenarios with honest explanation
- README numbers section: the three charts (Phase 6 priority win, Phase 7 scenarios matrix, Phase 9 vLLM comparison)

**Acceptance:** an interviewer skimming the README can identify the project's contribution in 30 seconds; an interviewer reading DESIGN.md gets enough rationale to ask a deep follow-up question.

---

## Stretch goals (only after Phase 10)

1. **Prefix caching** — content-addressed KV cache blocks for shared prompt prefixes. Huge win on chat workloads with shared system prompts.
2. **Prefill/decode disaggregation** — split into two processes that share cache via RPC, à la Splitwise/DistServe. Multi-node infra signal.
3. **GPU support** — llama.cpp has CUDA/Metal backends; flip the build tag. Doesn't change our code; expands the workloads we can run.
4. **Real model serving demo** — Phi-3-mini behind a public endpoint with rate limiting, monitoring. The "operate it for real users" angle from earlier discussion.

---

## Working agreement

- Every phase leaves `go build ./...` and `go test -race ./...` clean.
- Every non-obvious decision gets a paragraph in `DESIGN.md` *the same day it's made*.
- No half-merged phases. If a phase is incomplete, the branch isn't merged.
- **Author drives algorithmic decisions** (scheduler logic, cache eviction, priority math) — Claude writes boilerplate and pairs through the hard parts. This is the difference between a defensible project and AI slop, per author's commitment to understand-at-end.
- Reference reading happens *before* writing: vLLM source for the cache layout, Sarathi-Serve paper for chunked prefill, Ollama source for the CGO architecture. Cite, don't copy.
- Property tests for cache manager, scheduler, queue. Race tests for anything concurrent.

---

## Out of scope for v1

- Training of any kind
- Distributed inference across nodes (stretch only)
- Fine-tuning / LoRA / adapters
- Custom kernels (we use llama.cpp's)
- Quantization research (we use what llama.cpp ships)
- Web UI / chat frontend
- Authentication / multi-tenancy
- Streaming generation to anything other than SSE

---

## Why this isn't AI slop

A genuine 2026 portfolio project clears all of these bars; we should hit each:

1. **Recent papers, not 2014.** PagedAttention is 2023; Sarathi-Serve is 2024. The reference implementations were public for months at most when this is built.
2. **Real measurement story.** Three charts (Phase 6 twist, Phase 7 scenarios, Phase 9 vLLM comparison) with honest numbers.
3. **An original contribution.** Priority-aware reservation on top of chunked prefill is not in the standard implementations.
4. **Industry-relevance signal.** Directly maps to NVIDIA inference, Cloudflare Workers AI, every model-serving startup hiring in 2026.
5. **Failure-mode honesty.** DESIGN.md "failure modes" section names what doesn't work and why.
6. **Defensibility.** The hard parts (scheduler, KV cache) the author drives; everything else is boilerplate.

If any of these slip, the project becomes slop regardless of code quality. Flag early, fix early.
