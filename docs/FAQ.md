# Frequently Asked Questions (FAQ)

This document addresses architectural, design, and integration questions regarding **DSH-Go**.

---

## 1. What is DSH-Go?

**DSH-Go** is an in-process, pure Go implementation of the [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) Agent architecture. It translates the core mechanics of DeepSeek Harness—including Turn/Step loops, append-only event sourcing, derived `fold` state projections, Goal state machines, and fine-grained tool pipelines—into idiomatic, highly performant Go.

It serves as the native backend engine for **patelakash007's Native Android rewrite**, executing directly on-device without PRoot, Ubuntu chroots, or virtualization layers.

---

## 2. How does DSH-Go compare to general frameworks like LangChain or Eino?

They serve fundamentally different architectural tiers:

| Dimension | Generic Frameworks (LangChain, Eino) | DSH-Go |
|---|---|---|
| **Primary Scope** | Broad application and chain orchestration across arbitrary LLMs | DeepSeek-specific autonomous agent engine |
| **State Paradigm** | Snapshot-based state management | Append-only event sourcing (`fold` projections) |
| **Cache Engineering** | General prompt assembly | Prefix-cache affinity (achieving 97–99% cache hits) |
| **Execution Model** | External application process | In-process native library or standalone microservice |

---

## 3. What are the key architectural advantages of DSH-Go?

1. **Protocol Alignment**: Complete alignment with DeepSeek Harness specifications (53 typed event definitions, standardized `LlmFailure` taxonomy, 4-phase goal lifecycle).
2. **Deterministic Event Sourcing**: All state (messages, active plans, pending approvals, todos) is deterministically derived from an append-only event log. Replaying the log reproduces the exact state at any point in time.
3. **Optimized Cache Affinity**: Designed specifically to preserve DeepSeek API prefix caching across multi-turn interactions.
4. **Resilient Tool Pipeline**: 4-stage pipeline (`pre -> execute -> post -> result`) with sandboxing, token enforcement, and approval gates.

---

## 4. How does event sourcing improve auditability and recovery?

- **Immutable History**: Every user message, model turn, tool call, and system decision is recorded as an immutable event.
- **Time Travel & Projection**: Any historical checkpoint can be computed by folding over the slice `events[:N]`.
- **Crash Consistency**: In the event of an abnormal termination, the persistence layer automatically repairs unclosed step or turn boundaries upon recovery.

---

## 5. What platforms and environments are supported?

- **Native Android**: Arm64 / x86_64 native execution (no PRoot, no Termux, no root required).
- **Linux / macOS / Windows**: Standard Go 1.25+ binary execution.
- **Embedded / Server**: Can be embedded directly as a Go library or deployed as a lightweight standalone service (`cmd/dsh-server`).
