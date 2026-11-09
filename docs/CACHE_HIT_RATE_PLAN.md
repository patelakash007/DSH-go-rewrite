# DeepSeek Prefix Cache Hit Rate Alignment Plan

> **Objective**: Align the DeepSeek API prefix-cache hit rate in DSH-Go with upstream reference performance (**97–99%**).
>
> **Core Principle**: Upstream reaches 97–99% cache efficiency not through opaque tricks, but by **strictly avoiding unintended disruptions to the prompt prefix**. This engineering plan specifies disciplines, AST checks, and verification procedures to ensure DSH-Go introduces no cache-breaking mutations.

---

## 0. Roadmap: 7-Phase Execution

```text
┌──────────────────────────────────────────────────────────────────────────┐
│ Phase 0 · Cache Probe Instrumentation     │ Baseline telemetry & metrics │
│   └─ pkg/llm/tokenmeter                  │ (Cache probe attributes)     │
├──────────────────────────────────────────────────────────────────────────┤
│ Phase 1 · Append-Only Fold Enforcement   │ Session cluster (M02)        │
│   ├─ Pure fold functions                 │                              │
│   ├─ SurfaceOp replacement semantics     │                              │
│   └─ Invariant verification (8 rules)    │                              │
├──────────────────────────────────────────────────────────────────────────┤
│ Phase 2 · System Prompt Stability        │ Static ordering (M06/M07/M42)│
│   ├─ Strict template ordering            │                              │
│   ├─ Constant section sequence           │                              │
│   └─ Static lint: reject dynamic values  │                              │
├──────────────────────────────────────────────────────────────────────────┤
│ Phase 3 · Skills Catalog Change-Only     │ Skills registry (M27)        │
│   ├─ Change-only diff injection          │                              │
│   ├─ agent.inject() implementation       │                              │
│   └─ Deterministic catalog sorting       │                              │
├──────────────────────────────────────────────────────────────────────────┤
│ Phase 4 · PromptContext Registry         │ Dynamic context (M42)        │
│   ├─ PromptContext store                 │                              │
│   ├─ Change-only persistence             │                              │
│   └─ Retain latest state on compaction   │                              │
├──────────────────────────────────────────────────────────────────────────┤
│ Phase 5 · Defensive Anti-Pattern Guards  │ Cross-cutting concerns       │
│   ├─ Deterministic schema serialization  │                              │
│   ├─ Lexicographical tool registration   │                              │
│   ├─ AST static linter                   │                              │
│   └─ Runtime invariant assertion         │                              │
├──────────────────────────────────────────────────────────────────────────┤
│ Phase 6 · End-to-End Acceptance Tests    │ Final gate validation        │
│   ├─ 50-turn stability rate >= 95%       │                              │
│   ├─ Preset switching validation         │                              │
│   ├─ Compaction recovery                 │                              │
│   └─ Concurrent session stress test      │                              │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 1. The Four Engineering Disciplines

### Discipline 1: Append-Only Event Stream and Pure Derived Projections
- All session mutations must be recorded as append-only events.
- Never mutate prior events in place. Modifications to previous outputs (e.g., surface compaction or status updates) must be appended as typed delta events (`SurfaceReplaceEvent`).
- Derived state calculations (`FoldAll`, `IncrementalFold`) must remain referentially transparent and side-effect free.

### Discipline 2: Static System Prompt Ordering
- System prompt sections must be concatenated in a strictly hardcoded order:
  1. Base Persona / Role Definition
  2. Safety & Guardrail Boundaries
  3. Skill Schemas & Tool Definitions
  4. Context Window & Environment Facts
- Dynamic parameters (such as `time.Now()`, random tokens, or volatile process IDs) must never be embedded in the static system prefix. Dynamic facts must be passed via dedicated user-turn context frames.

### Discipline 3: Change-Only Skill Catalog Injections
- When the skill registry changes, only diffs are emitted.
- If no skills have been added, modified, or removed between turns, the skill definitions in the prompt header must remain byte-for-byte identical to leverage DeepSeek prefix caching.

### Discipline 4: Deterministic Schema & Tool Ordering
- Tool schemas and JSON attributes must be sorted deterministically by key prior to serialization.
- Map iterations in Go are non-deterministic by default (`map[string]any`); all tool definitions must be sorted lexicographically before building the LLM payload.

---

## 2. Anti-Pattern Defense

The static AST scanner (`internal/lint/cache_safety.go`) automatically scans code for cache-breaking anti-patterns:

1. **Anti-Pattern A: Dynamic Timestamps in Prompt Prefix**
   - Prohibition: Invoking `time.Now()` or formatting current wall-clock timestamps inside system prompt generators.
   - Solution: Use `pkg/timecontext` to supply temporal reference frames in separate user-side context blocks.

2. **Anti-Pattern B: Non-Deterministic Map Marshalling**
   - Prohibition: Directly calling `json.Marshal` on native Go maps without sorting keys.
   - Solution: Utilize ordered JSON encoders (`pkg/tools/schema.go`).

3. **Anti-Pattern C: Random UUID / Salt Generation in Context Headers**
   - Prohibition: Injecting nonces, random IDs, or session salts into the prompt prefix.
   - Solution: Keep system instructions static; assign trace IDs via metadata headers outside the prompt payload.

4. **Anti-Pattern D: Volatile Filesystem Path Injections**
   - Prohibition: Injecting host-specific absolute paths (e.g. `/tmp/...` or user homedirs) into tool descriptions.
   - Solution: Use standardized workspace URI prefixes (`workspace://`).

---

## 3. Metrics and Acceptance Criteria

| Metric | Target | Detection Mechanism |
|---|---|---|
| DeepSeek Prefix-Cache Hit Rate | >= 97% across 50 turns | `pkg/telemetry` & OpenTelemetry counters |
| First-Chunk Latency (TTFT) | <= 450ms on warm cache | Streaming SSE timer (`pkg/llm`) |
| Linter Violations | 0 occurrences | `go test ./tests/ -run TestCacheSafetyLint` |
| Regression Threshold | Alert when hit rate < 90% | Automated Prometheus / Grafana alarm rules |
