# DSH-Go Implementation Task Matrix (v2.0)

> This document is maintained in synchronization with [tasks.json](./tasks.json).
>
> - **tasks.json**: Structured, machine-readable task catalog with JSON schema validation.
> - **TASKS.md** (this file): Human-readable status matrix covering architecture clusters, priorities, and verification checkpoints.

---

## 0. Upstream Sync Anchor

| Field | Value |
|---|---|
| **Upstream Repository** | `https://github.com/deepseek-ai/deepseek-harness.git` |
| **Branch** | `master` |
| **Target Commit Hash** | `cd5ef8148158c3a752a658978873241fdf8e2bbc` |
| **Short Hash** | `cd5ef81481` |
| **Release Anchor** | **dsh-0.1.2-alpha.1** |
| **Native Rewrite Author** | **patelakash007** (Native Android rewrite) |

---

## 1. Status Legend

| Status | Badge | Meaning |
|---|---|---|
| `completed` | [x] Complete | All acceptance criteria satisfied, unit/integration tests passing |
| `in_progress` | [-] In Progress | Active implementation, testing, or refactoring |
| `pending` | [ ] Pending | Planned milestone |

---

## 2. Architecture Clusters Overview

### Cluster 1: Core Event Sourcing & Session Engine
- [x] **M01**: Branded ID Type Encapsulation (`pkg/brand`) — Strongly-typed SessionID, ToolCallID, etc.
- [x] **M02**: Event Sourcing Append-Only Log (`pkg/session`) — Log store with strict sequence ordering.
- [x] **M03**: Pure Derived Projections (`pkg/session/fold.go`) — State reconstruction via event fold.
- [x] **M04**: Incremental Fold Engine (`pkg/session/incremental.go`) — O(1) step updates, 3400x speedup.
- [x] **M05**: Invariant Verification Suite (`pkg/invariant`) — 8 core session integrity invariants.
- [x] **M06**: Session Compaction & Surface Replacement (`pkg/compaction`) — Dialogue summarization.
- [x] **M07**: Temporal Context Frame (`pkg/timecontext`) — Safe time-handling without cache breaking.

### Cluster 2: Agent Execution Loop & LLM Client
- [x] **M11**: Turn/Step Dual Execution Loop (`pkg/agent`) — Outer turn, inner step loop.
- [x] **M12**: Context & Timeout Propagation (`pkg/agent/cancel.go`) — H01 context budgeting.
- [x] **M13**: DeepSeek Provider Client (`pkg/llm/provider_deepseek`) — SSE parser, connection pool.
- [x] **M14**: LLM Failure Classification Taxonomy (`pkg/llm/retry.go`) — Structured error mapping.
- [x] **M15**: Token Metering & Budget Accounting (`pkg/tokenmeter`) — Prompt and completion quotas.
- [x] **M16**: Session Title Auto-Generation (`pkg/sessiontitle`) — Async title derivation.

### Cluster 3: Tool Pipeline & Security Sandboxing
- [x] **M21**: 4-Stage Tool Pipeline (`pkg/tools/pipeline.go`) — Pre, Exec, Post, Result lifecycle.
- [x] **M22**: JSON Schema Subset Validator (`pkg/tools/schema.go`) — Schema parsing and validation.
- [x] **M23**: Execution Sandboxing & Root Confinement (`pkg/sandbox`) — Restricted paths and commands.
- [x] **M24**: User Approval & Ask-Dangerous Gate (`pkg/approval`) — Interactive permission prompts.
- [x] **M25**: Tool Waterfall & Fallback Fall-through (`pkg/waterfall`) — Graceful degradation.
- [x] **M26**: Model Context Protocol (MCP) Bridge (`pkg/mcp`) — Standardized MCP tool integration.

### Cluster 4: Autonomous Planning & Goals
- [x] **M31**: Interactive Plan Mode (`pkg/plan`) — Non-destructive planning gates.
- [x] **M32**: Goal State Machine (`pkg/goal`) — 4-phase goal lifecycle with CAS transitions.
- [x] **M33**: Replace-All Todo Task Tracking (`pkg/todo`) — Deterministic todo list updates.
- [x] **M34**: Dynamic Skill Discovery & Registry (`pkg/skills`) — Hot-reloading skill sets via fsnotify.

### Cluster 5: Storage & Persistence
- [x] **M41**: Sharded Async JSONL Storage (`pkg/persistence`) — High-throughput append log.
- [x] **M42**: SQLite Storage with FTS5 (`pkg/persistence/sqlite`) — Full-text session search.
- [x] **M43**: Crash Consistency & Recovery (`pkg/persistence/repair.go`) — Automated repair of open turns.

### Cluster 6: Telemetry & Observability
- [x] **M51**: OpenTelemetry Tracing Bridge (`pkg/telemetry`) — Distributed trace context export.
- [x] **M52**: DeepSeek Cache Probe & Metrics (`internal/telemetry`) — Real-time prefix hit tracking.
- [x] **M53**: Cache Safety AST Linter (`internal/lint`) — Automated regression checks.
