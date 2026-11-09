# DSH-Go

<div align="center">

**In-process Go Engine and Reference Implementation of DeepSeek Harness Agent**

[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![DSH Compat](https://img.shields.io/badge/DeepSeek_Harness-v1.0.0--native-blue)](#compatibility)
[![Architecture](https://img.shields.io/badge/architecture-event--sourcing-7c3aed)](#features)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

</div>

---

**DSH-Go** is a high-performance, pure Go, in-process engine and reference implementation for the [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) Agent architecture. It provides an idiomatic, modular, and fully debuggable Go implementation of Turn/Step dual execution loops, append-only event sourcing, derived fold projections, Goal state machines, and fine-grained tool governance.

This repository serves as the native backend execution engine for **patelakash007's Native Android rewrite**, enabling on-device execution without virtualized containers or chroot overhead.

---

## Table of Contents

- [Core Principles](#core-principles)
- [Key Features](#key-features)
- [Architecture](#architecture)
- [Performance Benchmarks](#performance-benchmarks)
- [Quick Start](#quick-start)
- [Runnable Examples](#runnable-examples)
- [Package Layout](#package-layout)
- [Compatibility](#compatibility)
- [License & Attribution](#license--attribution)

---

## Core Principles

1. **Native In-Process Execution**: Zero external runtime overhead. Embeds directly into Go services or Android native binaries via CGO/JNI.
2. **Event Sourcing as Single Source of Truth**: Append-only event stream (`pkg/session`) where session states, goals, todos, and plans are deterministic projections derived via `fold`.
3. **Prefix-Cache Affinity**: Engineered specifically to maximize DeepSeek prefix-cache hit rates (97–99%), minimizing token latency and inference costs.
4. **Resilient Tool Pipeline**: 4-stage tool pipeline (`pre -> execute -> post -> result`) with sandbox containment, permission overrides, and token budgets.

---

## Key Features

- **Event-Sourced Sessions (`pkg/session`)**: Append-only log with deterministic `fold` projections, 45+ typed events, and strict temporal invariants.
- **Turn/Step Dual Loop (`pkg/agent`)**: Comprehensive cancellation, timeouts, and distributed context propagation across LLM and tool boundaries.
- **Autonomous Planning Primitives**:
  - `pkg/plan`: Interactive plan-mode safety gates.
  - `pkg/goal`: 4-phase goal state machine (`active`, `paused`, `blocked`, `complete`) with multi-round continuation.
  - `pkg/todo`: Deterministic replace-all task tracking.
  - `pkg/skills`: Dynamic skill discovery, grammar enforcement, and live filesystem watching (`fsnotify`).
- **4-Stage Tool Pipeline (`pkg/tools`)**: Pre-execution validation, sandboxed execution, post-processing, and structured output formatting backed by `sync.Pool`.
- **DeepSeek LLM Provider (`pkg/llm/provider_deepseek`)**: Streaming SSE parser, production connection pooling, and standardized failure classification.
- **Multi-Level Governance (`pkg/sandbox`, `pkg/approval`)**: Policy-based execution, confirmation prompts for sensitive operations, and granular filesystem confinement.
- **Full Observability (`pkg/telemetry`, `pkg/tokenmeter`)**: OpenTelemetry integration, structured session hooks, token budgeting, and live cache metrics.

---

## Architecture

```
                    ┌───────────────────────────────┐
                    │       User / Native App       │
                    └───────────────┬───────────────┘
                                    │ (Turn / Input)
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                          DSH-Go Agent Loop                             │
│                                                                        │
│   ┌────────────────────┐    Step     ┌─────────────────────────────┐   │
│   │ Turn / Step Driver ├────────────►│  Tool Pipeline (4-Stage)    │   │
│   └─────────┬──────────┘             │  pre -> exec -> post -> res │   │
│             │                        └──────────────┬──────────────┘   │
│             │ LLM Chat Stream                       │                  │
│             ▼                                       ▼                  │
│   ┌────────────────────┐             ┌─────────────────────────────┐   │
│   │ DeepSeek Provider  │             │ Sandbox / Approval Boundary │   │
│   └────────────────────┘             └─────────────────────────────┘   │
│                                                                        │
│   ┌────────────────────────────────────────────────────────────────┐   │
│   │                 Append-Only Event Store                        │   │
│   │             (Session Log -> Derived Projections)               │   │
│   └────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────┘
```

---

## Performance Benchmarks

Key paths have been hardened for concurrent execution and minimal allocations:

| Benchmark Scenario | Optimization Technique | Result |
|---|---|---|
| Session Derived Projections (10k events) | Incremental `fold` | **16.9s → 4.9ms (≈3437× speedup)** |
| Shared Registry Read (100 keys) | Lock-free snapshot after Freeze | 65.6ns → 49.4ns (**25% faster**, 0 allocs) |
| Persistence I/O Hot Path | Dual `sync.Pool` (`bytes.Buffer` + `bufio.Writer`) | Zero redundant allocations |
| Tool Pipeline Invocation | `ExecContext` object pooling | Heap allocations reduced from 9 to 8 |

---

## Quick Start

### Installation

```bash
go get github.com/JopenChen/dsh-go@latest
```

### Minimal Example

Create an event-sourced session log, append messages, and derive the state projection:

```go
package main

import (
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func main() {
	sl := session.NewSessionLog(brand.NewSessionID("session-001"))
	
	if _, err := sl.Append(session.UserMessageData{Content: "Hello, DSH!"}); err != nil {
		panic(err)
	}

	proj := session.FoldAll(sl.Events())
	fmt.Printf("Session active with %d message(s)\n", len(proj.Messages))
}
```

---

## Runnable Examples

The repository includes ready-to-run examples under `examples/`:

```bash
# Core tutorial: Event sourcing -> fold projection -> Goal state machine
go run ./examples/tutorial

# Full agent Turn/Step loop with tool chaining
go run ./examples/agent_loop

# End-to-end usage with session, persistence, and commands
go run ./examples/usage

# Interactive streaming chat with DeepSeek (requires DEEPSEEK_API_KEY)
go run ./examples/chat

# In-process subagent spawning and lifecycle cascading
go run ./examples/subagent

# Multi-step pipeline and parallel workflow orchestration
go run ./examples/workflow

# Sandboxed execution and approval modes
go run ./examples/sandbox_approval

# Model Context Protocol (MCP) tool integration
go run ./examples/mcp

# Atomic replacement todo list
go run ./examples/todo
```

---

## Package Layout

| Package | Purpose |
|---|---|
| `pkg/agent` | Turn and step execution loops, context cancellation, model selection |
| `pkg/session` | Append-only event log, derived fold projections, event vocabulary |
| `pkg/goal` | Planning state machine, multi-round continuations, CAS concurrency |
| `pkg/plan` | Interactive plan mode gates and approval tools |
| `pkg/todo` | Replace-all task tracking primitives |
| `pkg/tools` | 4-stage tool pipeline middleware, schemas, and registration |
| `pkg/sandbox` | Execution boundary isolation, workspace confinement, escalation policies |
| `pkg/approval` | User confirmation workflows, ask-dangerous policies, decision triage |
| `pkg/llm` | LLM abstraction, token metering, streaming protocols |
| `pkg/llm/provider_deepseek` | DeepSeek production client, connection pooling, SSE streaming |
| `pkg/persistence` | Sharded JSONL and SQLite event storage engines |
| `pkg/skills` | Dynamic skill registry, grammar validation, fsnotify triggers |
| `pkg/subagent` | In-process subagent hierarchies and lifecycle ownership cascades |
| `pkg/workflow` | Composable sequential pipelines and parallel branch execution |
| `pkg/telemetry` | OpenTelemetry tracing bridges and session metric collectors |
| `cmd/dsh-server` | Standalone server exposing REST / WebSocket control endpoints |

---

## Compatibility

- **Go Version**: Go 1.25 or later.
- **SQLite Engine**: [modernc.org/sqlite](https://modernc.org/sqlite) (pure Go, CGO-free).
- **DeepSeek Harness Alignment**: Aligned with upstream specification and core protocol behaviors.

---

## License & Attribution

This project is licensed under the **GNU Affero General Public License v3.0** (AGPL-3.0). See [LICENSE](LICENSE) for full details.

```
Portions of the architecture and implementation are derived from the original upstream project by ccch1mneyyy (licensed under AGPL-3.0).
This repository is patelakash007's Native Android rewrite, maintained at https://github.com/patelakash007/DSH-go-rewrite.
```
