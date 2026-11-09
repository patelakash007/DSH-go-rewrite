# DSH-Go Comprehensive Test Matrix (TEST_CASES.md)

> This document is generated based on [tasks.json](./tasks.json)   **98 itemsuccess capability point**step-by-step 1 splitparse generate，is  Dsh-Go itemtarget  **full volume testtest case design matrix**。
>
> - - Coverage: M01-M48（MUST）/ S01-S16（SHOULD）/ C01-C17（COULD）/ N01-N09（Cache Affinity）/ H01-H08（Concurrency Hardening），total 98 items.
> - - Case ID convention: `TC-<any service ID>-<order number >`，e.g. `TC-M01-01` denotes case 1 of task M01.
> - - Status metric: Based on per-item status in tasks.json (`completed` / `pending` / `deferred` / `skipped`).
> - - Related files: Completed tasks list existing test files (`tests/*_test.go`); pending tasks list target test files for regression.

---

## 0. Overview

### 0.1 Task Distribution Statistics (per-item status from tasks.json)

| Level | Cluster | Tasks | Complete | Pending | Deferred | Skipped |
|---|---|---|---|---|---|---|
| MUST | cluster-1~7 (M01-M48) | 48 | 48 | 0 | 0 | 0 |
| MUST | cluster-8 Cache Affinity (N01-N09) | 9 | 9 | 0 | 0 | 0 |
| SHOULD | cluster-4~7 (S01-S16) | 16 | 9 | 7 | 0 | 0 |
| COULD | extensions / ui-skip (C01-C17) | 17 | 0 | 0 | 11 | 6 |
| MUST | cluster-9 Concurrency Hardening (H01-H08) | 8 | 0 | 8 | 0 | 0 |
| **Total** | | **98** | **66** | **15** | **11** | **6** |

> Note: tasks.json per-item status reflects completed=66 / pending=15.

### 0.2 Testing Strategy

1. **Layered Coverage**: Every functional point covers at least 3 categories: Positive primary path (acceptance criteria), Boundary/Exception path (error codes, limits, concurrency), Integration path (cross-module dependencies).
2. **Completed Items**: Directly correspond to test suites in `tests/` for regression. Uncovered boundaries are listed as Supplemental.
3. **Pending Items**: Architectural drafts marked `[Pending]`, required for implementation regression testing.
4. **Deferred/Skipped Items**: Marked `[Deferred]`/`[Skipped]`, capturing criteria without requiring current implementation.

### 0.3 Field Definitions

| Field | Meaning |
|---|---|
| Case ID | `TC-<task-id>-<index>`, globally unique |
| Type | `Positive` / `Boundary` / `Exception` / `Concurrency` / `Integration` |
| Associated Task | Feature verified by this case |
| Preconditions | State required prior to execution |
| Steps | Executable sequence of actions |
| Expected Result | Expected output aligned with acceptance criteria |
| Status | `Implemented` (code + tests) / `Pending` / `Deferred` / `Skipped` |
| Related Files | Existing or planned test file |

---

## 1. Cluster 1: Core Driver (cluster-1-core-driver)

### M01 Branded ID Type Encapsulation (Completed)

> Goal: Branded/Bytes Two branded struct kinds (SessionID, ToolCallID, ApprovalRequestId, JobId, SkillId, AttachmentId, CredentialRef, WorkspaceId, ProjectionId, etc.) preventing raw string ID confusion.
> Related Files: `tests/brand_ids_test.go`（6 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M01-01 | Positive | 9 kindsitem card  ID construct construct function count  | None | Create 9 branded ID types using NewXxxID | Generates correct branded values, String() returns raw string | Implemented | brand_ids_test.go |
| TC-M01-02 | Positive | ID serialize round-trip | Pre-created various ID instances | Execute MarshalJSON -> UnmarshalJSON on IDs | Deserialized values strictly match original IDs | Implemented | brand_ids_test.go |
| TC-M01-03 | Boundary | Zero/IsZero determine define  | None | Create Zero value and invoke IsZero | Zero evaluation correct; non-zero IDs evaluate to false | Implemented | brand_ids_test.go |
| TC-M01-04 | Exception | errorTypepass input（encode translate phase） | Go call site | Pass SessionID to function expecting JobID | Compile-time error (type mismatch) preventing invalid ID passing | Implemented | brand_ids_test.go |
| TC-M01-05 | Boundary | empty wordtoken serial /malformed form  ID Parse | None | Execute Parse on empty or malformed strings | Returns error or zero value without panic | Supplemental | — |

### M02 Waterfall Middleware Chain Primitives (Completed)

> Goal: Generic WaterfallFunc[T] + Chain composition + typed next(); 4-stage agent/pre-step/request/tools chain + approval + plan/goal injection reusing a single implementation.
> Related Files: `tests/waterfall_chain_test.go`（8 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M02-01 | Positive | multistageintercept truncate strict format order order  | Assemble 3-stage Chain | Execute sequentially and record invocation order | Invocation matches registration order (onion model) | Implemented | waterfall_chain_test.go |
| TC-M02-02 | Exception | next() un-invokeshort route  | A middleware omits calling next() | Execute Chain | Short-circuits and returns middleware error; subsequent stages skipped | Implemented | waterfall_chain_test.go |
| TC-M02-03 | Positive | payload cross-stagemodify write | Middleware mutates payload | Execute full Chain | Downstream receives mutated payload; final output reflects all mutations | Implemented | waterfall_chain_test.go |
| TC-M02-04 | Exception | re-reuse invoke next() | Middleware calls next() twice | Execute Chain | Panic guard active (RunSafe catch) or error; avoids duplicate side effects | Implemented | waterfall_chain_test.go |
| TC-M02-05 | Integration | 4 stagechainreuse use  | Construct 4-stage agent/pre-step/request/tools chain | Inject shared Chain implementation and execute | All 4 tiers follow identical implementation and behavior | Supplemental | — |

### M03 Scope Hierarchical Registry Primitives (Completed)

> Goal: ScopeKey + Layer + ScopedLayers merge (nearest-scope-wins + rank) reused across Tools/Skills/Commands/Credentials registries.
> Related Files: `tests/scope_layers_merge_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M03-01 | Positive | host + scope stack add priority | Register same-name entry in host and scope layers | Read target entry | nearest-scope-wins: scope value overrides host value | Implemented | scope_layers_merge_test.go |
| TC-M03-02 | Boundary | anonymous name  entry and named entry isolate | Register anonymous and named entries | Read each entry | No conflict; each retrieves its own entry | Implemented | scope_layers_merge_test.go |
| TC-M03-03 | Boundary | rank mutually same time afterregisteroverride | Register 2 entries with identical rank | Read | Later registration overrides earlier (registration tie-breaker) | Implemented | scope_layers_merge_test.go |
| TC-M03-04 | Exception | multilayer scope deep degree merge | Construct 3 scope levels | Merge afterRead | Innermost wins; Merge result is deterministic and reproducible | Implemented | scope_layers_merge_test.go |

### M04 Session Event Sourcing & 53-Event Vocabulary (Completed)

> Goal: 53 event types (Map->Derived Union pattern) + SessionEvent{seq,time,type,data,surfaceOp?,sourceEventSeqs?} + append strict invariants (turn open/close, step pairing, tool call<->result pairing).
> Related Files: `tests/session_event_vocab_test.go` + `tests/session_event_ordering_test.go`（7 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M04-01 | Positive | 53 kindsevent round-trip | Construct each event type | Serialize -> deserialize | Field-by-field identical | Implemented | session_event_vocab_test.go |
| TC-M04-02 | Exception | turn/end Nonebefore turn/start | Directly append turn/end | Trigger append | Invariant error reported | Implemented | session_event_ordering_test.go |
| TC-M04-03 | Exception | tool/call lack lose  tool/result | Append unmatched tool/call | Trigger append | Invariant error (missing pairing) | Implemented | session_event_ordering_test.go |
| TC-M04-04 | Boundary | seq/time continuousmonotonic | Append 100 events | Validate seq and time | seq strictly continuous, time monotonically increasing | Implemented | session_event_ordering_test.go |
| TC-M04-05 | Positive | surfaceOp/sourceEventSeqs carry carry  | Append event with surfaceOp | serialize round-trip | surfaceOp and sourceEventSeqs preserved intact | Supplemental | — |

### M05 Session Derived Projection Functions (Completed)

> Goal: `deriveMessages()` + foldRequestHeader + foldEffectiveSandboxMode + foldEffectiveApprovalPolicy + foldGoalChange + foldTodoWrite + foldPlanMode + foldPermissionPreset + foldSessionTitle。
> Related Files: `tests/session_fold_consistency_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M05-01 | Positive | re-release  vs hot  append consistentproperty  | 1k+ event log | Full replay folds vs active append folds | each  fold resultField-by-field identical | Implemented | session_fold_consistency_test.go |
| TC-M05-02 | Integration | compaction replace after deriveMessages | Execute compaction replace | Re-run deriveMessages | Output condensed and subsequent messages derive correctly | Implemented | session_fold_consistency_test.go |
| TC-M05-03 | Positive | FoldAll aggregate combine | Construct mixed event stream | Call FoldAll | Aggregated projections match individual fold results | Implemented | session_fold_consistency_test.go |

### M06 SessionHeader Metadata (Completed)

> Goal: Version/ID/CreatedAt/Cwd/ParentSession/SeedLength/Origin/DelegationDepth/AgentPreset; used by persistence keys, fork lineage, and subagent recursion depth.
> Related Files: `tests/session_header_versioning_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M06-01 | Positive | Header serialize round-trip | Construct full Header | Serialize -> deserialize | Fields match and SESSION_FORMAT_VERSION aligns | Implemented | session_header_versioning_test.go |
| TC-M06-02 | Exception | unknown Header version fail-closed | Header contains unknown version number | Read | rejectRead and report formaterror | Implemented | session_header_versioning_test.go |
| TC-M06-03 | Integration | fork/cold-resume spectrum systemwriteinput | Execute fork or cold-resume | Inspect new Header | ParentSession/SeedLength/DelegationDepth written correctly | Implemented | session_header_versioning_test.go |
| TC-M06-04 | Boundary | subagent pass attribute deep degree  | Multi-tier subagent fork | Inspect Header at each tier | DelegationDepth increments by 1 per tier | Supplemental | — |

### M07 LLM Provider Seam + Streaming Protocol (Completed)

> Goal: LLMAdapter interface + Message/ContentBlock(text/tool_use/tool_result/image) + StreamChunk(text/reasoning/tool-call) + ToolSchema + LlmFailure taxonomy + DeepSeek REST+SSE implementation.
> Related Files: `tests/llm_stream_roundtrip_test.go`（4 cases PASS，mock SSE）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M07-01 | Positive | SSE streaming chunk parse | Mock SSE response | Readcompletestream | text/reasoning/tool-call chunk order and content correct | Implemented | llm_stream_roundtrip_test.go |
| TC-M07-02 | Positive | 4 classes ContentBlock round-trip | Construct each block type | Serialize -> deserialize | Type and content consistent | Implemented | llm_stream_roundtrip_test.go |
| TC-M07-03 | Exception | LlmFailure splitclassesreflect map  | Simulate overload/rate-limit/refusal/context-overflow | Trigger each failure mode | Mapped to correct taxonomy classification | Implemented | llm_stream_roundtrip_test.go |
|TC-M07-04|Integration| DeepSeek Real Client (requires API key) |Configure valid API key|Invoke /chat/completions(stream=true)|Receive full reasoning and tool_call chunks|Pending (requires API key)| llm_provider_deepseek_integration_test.go |

### M08 Agent Registry + Turn/Step Dual Loop (Completed)

> Goal: Agent interface + Inbox dual queue + Turn loop + Step loop + single Turn serial guarantee.
> Related Files: `tests/agent_loop_turn_step_dual_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M08-01 | Concurrency | dual  Followup serialqueue | Agent running | Submit 2 Followups | Queued serially; pending turns rejected appropriately | Implemented | agent_loop_turn_step_dual_test.go |
| TC-M08-02 | Exception | step/tool errorcollect tail  | Tool execution returns error | Trigger error | agent/error written to log; turn closes as interrupted | Implemented | agent_loop_turn_step_dual_test.go |
| TC-M08-03 | Positive | Turn→Step full streamprocess eventorder  | Standard conversation | Inspect event stream | turn/start -> pre-step -> step loop -> stopping -> turn/end order correct | Implemented | agent_loop_turn_step_dual_test.go |
| TC-M08-04 | Boundary | single Turn serialization guarantee | Trigger concurrently on same Agent | Submit concurrent requests | At most one Turn active at any instant | Implemented | agent_loop_turn_step_dual_test.go |

### M18 Agent Cancel Reason Taxonomy (Completed)

> Goal: {kind:user}/{parent}/{hook reason}/{disposed}/{legacy} 5 kinds -> TurnEndReason.aborted.
> Related Files: `tests/agent_cancel_cause_classify_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M18-01 | Positive | 5 classescancelraw cause zone split | construct construct  user/parent/hook/disposed/legacy cancel | step-by-step 1 triggercancel | turn/end   aborted.reason splitdistinguish for should correctsplitclasses | Implemented | agent_cancel_cause_classify_test.go |
| TC-M18-02 | Integration | RecordCancel writelog | Agent run in cancel | invoke RecordCancel | canceleventwriteinput session log，turn by  aborted collect tail  | Implemented | agent_cancel_cause_classify_test.go |
| TC-M18-03 | Exception | Nonecancelrecordtime splitclasses | normalresult end   turn | ExtractCancelCause | returnNonecancel/default acknowledge splitclasses，not error report  | Implemented | agent_cancel_cause_classify_test.go |

### M19 Request Header Snapshot + Request/Context Routing (Completed)

> Goal: EpochHeader{config/system/tools} + RequestContext{provider/model/contextWindow} + reason in {initial/resume/change/series}。
> Related Files: `tests/request_header_rebuild_test.go`（2 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M19-01 | Integration | fast photo rebuildconsistentproperty  | writeinput request/header fast photo  | use most newfast photo rebuild Prompt+Schema | andreal edge send send  payload Field-by-field identical | Implemented | request_header_rebuild_test.go |
| TC-M19-02 | Boundary | reason route by  | setting set  initial/resume/change/series | Read RequestContext | reason correctroute by andrecord | Implemented | request_header_rebuild_test.go |
| TC-M19-03 | Integration | compaction afterrebuild | execute compaction | RebuildFromHeader | not depend depend raw start  events immediate can rebuild | Supplemental | — |

### M20 session/end-seed Seed Boundary Marker (Completed)

> Goal: First live marker event written after Resume/Fork, marking boundary between cold stored and live work.
> Related Files: `tests/end_seed_bracket_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M20-01 | Positive | Fork/Resume aftermust write end-seed | execute Fork/Resume | Inspect event stream | end-seed marker writeinputand carry carry parent session | Implemented | end_seed_bracket_test.go |
| TC-M20-02 | Boundary | SeedEndSeq splitboundary determine define  | has  end-seed   log | determine break eventposition at  seed beforeafter | IsAfterEndSeed determine define correct | Implemented | end_seed_bracket_test.go |
| TC-M20-03 | Integration | compaction half open include number define position  | execute compaction | use use  end-seed define position  | half open include number define position correct（not cross- seed truncate live content） | Implemented | end_seed_bracket_test.go |

### M21 SurfaceOp (append/replace) + foldSurface (Completed)

> Goal: {op:'replace',start,end} generations; foldSurface(events)=>nodes+replacements; compaction replacement node operates via surface replace.
> Related Files: `tests/compaction_replace_surface_test.go`（2 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M21-01 | Positive | replace range readtime replace | append 100 eventafter replace [3,80] | deriveMessages | beforeafterconsume rest list tablewordnode consistent，sourceeventun-modify dynamic  | Implemented | compaction_replace_surface_test.go |
| TC-M21-02 | Boundary | append-only pure clean property  | execute replace after | inspectsourceeventcount set | sourceevent append-only，None in-place modify | Implemented | compaction_replace_surface_test.go |
| TC-M21-03 | Boundary | non method  replace zone space  | start>end orexceed boundary  | foldSurface | report error orsafetyreject，not produce produce errorreplace | Supplemental | — |

### M23 Tool Execution 4-Stage Waterfall Chain (Completed)

> Goal: pre-execute → execute(exchangeable signal) → post-execute(accept/block/attach ctx) → result。
> Related Files: `tests/tools_waterfall_test.go`（8 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M23-01 | Positive | 4 stage middleware each communicate channel work operate  | register deny/swap parameter /truncate/add  meta  in space item  | executetoolinvoke | each communicate channel all take effect | Implemented | tools_waterfall_test.go |
| TC-M23-02 | Exception | execute swap  signal cancel | execute in cancel | post-execute → result | result carry  isError，cancellang definition correct | Implemented | tools_waterfall_test.go |
| TC-M23-03 | Positive | WithTool injecttrue real real now  | construct construct tool | executeinvoke | true real real now is execute and returnresult | Implemented | tools_waterfall_test.go |
| TC-M23-04 | Boundary | 4 stageorder order  | registermulti in space item  | recordinvokeorder  | pre→execute→post→result strict format order order  | Implemented | tools_waterfall_test.go |

### M32 Agent Preset Seam (Completed)

> Goal: ctx.agentPresets.{mount/composeFrom/standingKeyFor/recompose/select}。
> Related Files: `tests/agent_preset_compose_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M32-01 | Positive | different preset exclusive establish setcombine | mount load  2   preset | splitdistinguish create build  Agent real example  | each self  tools/prompt setcombineexclusive establish correct | Implemented | agent_preset_compose_test.go |
| TC-M32-02 | Positive | ComposeFrom tool and collection+prompt splice connect  | setcombine 2   preset | invoke ComposeFrom | tool and collection、prompt splice connect correct | Implemented | agent_preset_compose_test.go |
| TC-M32-03 | Boundary | standing key | multitimes Select same  preset | inspect standing key | standing key stableconsistent | Implemented | agent_preset_compose_test.go |

### M33 Agent Initiator Context (Completed)

> Goal: withInitiator/requireInitiator/withoutInitiator；security attribution。
> Related Files: `tests/initiator_causal_trace_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M33-01 | Exception | requireInitiator Nonecontext | None withInitiator packagewrap  | invoke requireInitiator | panic orreturn Unauthorized result construct ize error | Implemented | initiator_causal_trace_test.go |
| TC-M33-02 | Positive | withInitiator→require attribute cause  | correctpackagewrap  | invoke requireInitiator | return AgentID/Op correct | Implemented | initiator_causal_trace_test.go |
| TC-M33-03 | Boundary | withoutInitiator clean eliminate  | has  initiator afterclean eliminate  | invoke withoutInitiator then  require | attribute cause is clean eliminate  | Implemented | initiator_causal_trace_test.go |
| TC-M33-04 | Integration | cause result chainappend trace  | multilayerembedsuite initiator | append trace attribute cause chain | each layercause result chaincorrect | Implemented | initiator_causal_trace_test.go |

### M34 agent/request-error Retry Waterfall (Completed)

> Goal: RequestErrorAction{kind:'retry'}，LLM overload / rate-limit explicit retry channel。
> Related Files: `tests/agent_request_error_retry_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M34-01 | Positive | can retryerror → retry | trigger overload/rate-limit | walk  request-error waterfall | return retry action | Implemented | agent_request_error_retry_test.go |
| TC-M34-02 | Exception | over pass  max timesretry | persist proceed fail | retryto up limit  | by  error close close  turn | Implemented | agent_request_error_retry_test.go |
| TC-M34-03 | Integration | and S15 Retry assist same  | config S15 Retry | triggerpass load  | 2 entity assist same not conflict conflict ，eventcorrect | Implemented | agent_request_error_retry_test.go |
| TC-M34-04 | Positive | RecordRequestError writelog | requestfail | recorderror | agent/error Events written correctly | Implemented | agent_request_error_retry_test.go |

### M47 Tool Presentation Neutral Vocabulary (Completed)

> Goal: ToolCallView/ToolResultView 9 card kinds（generic/terminal/diff/search/read/web etc）。
> Related Files: `tests/tools_presentation_cards_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M47-01 | Positive | 9 kindscardfieldfor align  | construct construct  9 card kinds | inspecteach field | andup cursor raw edition 1 1 for should  | Implemented | tools_presentation_cards_test.go |
| TC-M47-02 | Positive | CardOf determine distinguish  | submit provide self define definition  card | invoke CardOf | self define definition back persist  other，determine distinguish correct | Implemented | tools_presentation_cards_test.go |
| TC-M47-03 | Boundary | empty /most small card | construct construct lack field card | serialize | not  panic，fieldlack lose can contain tolerate  | Implemented | tools_presentation_cards_test.go |

### M48 DefineTool JSON Schema Strict Validation Subset (Completed)

> Goal: JsonSchemaNode + object root assert + validate + INFER；subagent/workflow structured output similarly reused。
> Related Files: `tests/tools_jsonschema_subset_test.go`（8 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M48-01 | Positive | oneOf/items/enum/const/default/examples lang definition  | construct construct reuse mix  schema | Validate each bound end  | andraw edition lang definition  1:1 consistent | Implemented | tools_jsonschema_subset_test.go |
| TC-M48-02 | Exception | not branch persist close keyword fail-closed | schema  contains unknownclose keyword | Compile | report error and non static default ignore policy  | Implemented | tools_jsonschema_subset_test.go |
| TC-M48-03 | Positive | Infer anti push  | providedefine  Go value | Infer schema | anti push out   schema bound end correct | Implemented | tools_jsonschema_subset_test.go |
| TC-M48-04 | Positive | additionalProperties=false | close close quota outer attribute property  | Validate | quota outer attribute property report error  | Implemented | tools_jsonschema_subset_test.go |
| TC-M48-05 | Boundary | confirm define property  Marshal | same  schema multitimes Marshal | compare compare output | wordnode consistent（wordclassic order ） | Implemented | tools_jsonschema_subset_test.go |

## 2. Cluster 2: Planning (cluster-2-planning)

### M09 SystemPrompt Assembly + Section Registry (Completed)

> Goal: PromptSection{name/order/text} + queue order merge + tools schema inject；order order error session lead cause model type execute as drift move 。
> Related Files: `tests/sysprompt_section_ordering_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M09-01 | Positive | setassemble order order andraw edition consistent | registerall section | Assemble | persona→policy→runtime-context-snapshot→plan:policy→tools schema order order step-by-step execute  diff consistent | Implemented | sysprompt_section_ordering_test.go |
| TC-M09-02 | Boundary | order normal volume correct | None | verifyeach  section order | 100/200/300/500/600/700 token combinesetting count  | Implemented | sysprompt_section_ordering_test.go |
| TC-M09-03 | Positive | tools schema wordclassic order inject | multitool | generate ToolsSectionText | toolby wordclassic order inject | Implemented | sysprompt_section_ordering_test.go |
| TC-M09-04 | Boundary | same name  section override | registersame name  section | Assemble | afterregisteroverridefirst register，not out now re-reuse  | Supplemental | — |

### M10 PromptContext Dynamic Registry & Snapshot (Completed)

> Goal: PromptContext{name/order/text(AssembleCtx)} + runtime-context-snapshot user/message。
> Related Files: `tests/prompt_context_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M10-01 | Positive | dynamic state add add /move eliminate take effect | register context | add add /move eliminate after assemble | down 1 turn assemble take effect/invalidate | Implemented | prompt_context_test.go |
| TC-M10-02 | Integration | compaction keep retain fast photo  | execute compaction | inspect runtime-context-snapshot | most newfast photo keep retain not drop  | Implemented | prompt_context_test.go |
| TC-M10-03 | Integration | GoalRoundContext proceed turninject | goal.active | observeinject | proceed turnpromptcorrect，complete afterstop stop  | Implemented | prompt_context_test.go |
| TC-M10-04 | Boundary | Compute stable/change | Statusnot change  vs change | multitimes Compute | Nonechange hash stable，change hash modify change  | Implemented | prompt_context_test.go |

### M11 Plan Mode Soft Guidance + Approval Exit (Completed)

> Goal: plan:policy Prompt Section + plan/mode log + exit_plan_mode tool + UserQuestion approval。
> Related Files: `tests/plan_mode_approval_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M11-01 | Positive | enter input plan mode inject policy | execute Enter | Next round request | plan:policy section succeedinject | Implemented | plan_mode_approval_test.go |
| TC-M11-02 | Positive | exit approvalcommunicate pass move eliminate  | call  exit_plan_mode and approvalcommunicate pass  | Next round request | plan:policy section not then out now  | Implemented | plan_mode_approval_test.go |
| TC-M11-03 | Exception | exit approvalrejectkeep retain  | approvalreject | Next round request | plan:policy section still save at  | Implemented | plan_mode_approval_test.go |
| TC-M11-04 | Positive | plan/mode eventrecord | Enter/Exit | Inspect event stream | plan/mode Events written correctly | Supplemental | — |

### M12 Goal System (State Machine + Turn Driver + 6 Tools) (Completed)

> Goal: GoalPhase/CAS Revision/goal/change event + goal-round-driver turn-stopping monitor listen  + 6   goal_* tool + concludeTurn。
> Related Files: `tests/goal_rounddriver_cas_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M12-01 | Integration | self dynamic proceed turn 5 turn | set_goal(active, maxRounds=5) | Noneuse user inputrun | self dynamic proceed turnrun complete  5 turn | Implemented | goal_rounddriver_cas_test.go |
| TC-M12-02 | Positive | report_blocker → concludeTurn | invoke goal_report_blocker | observe turn | turn/end correctwriteout ，local turnresult end  | Implemented | goal_rounddriver_cas_test.go |
| TC-M12-03 | Concurrency | CAS Revision conflict conflict  | Concurrencymodify goal | conflict conflict write | CAS conflict conflict report error Concurrencykeep protect  | Implemented | goal_rounddriver_cas_test.go |
| TC-M12-04 | Positive | 6   goal_* tool | step-by-step 1 invoke | inspecteffect result  | list/set_phase/set_description/set_max_rounds/add_blocker/report_blocker allcorrect | Implemented | goal_rounddriver_cas_test.go |

### M13 Todo Full Replacement Write (Completed)

> Goal: todo/write event + foldTodoWrite + todo_write tool。
> Related Files: `tests/todo_write_replace_test.go`（1 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M13-01 | Positive | last-write-wins whole entityreplace | multitimes todo/write | fold | per timeswhole entityreplace，andraw edition  last-write-wins consistent | Implemented | todo_write_replace_test.go |
| TC-M13-02 | Positive | todo_write toolwriteevent | Invoke tool | Inspect event stream | todo/write Events written correctly | Supplemental | — |
| TC-M13-03 | Boundary | empty  todo list tablereplace | pass inputempty list table | fold | clean empty take effect | Supplemental | — |

### M14 User Questions Seam (Completed)

> Goal: UQ serve service connect port  + provider_stub(same stepblock block ) + SDK injecttrue real  impl；branch persist  multiSelect + intent standard tag 。
> Related Files: `tests/userq_provider_swap_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M14-01 | Positive | stub returndefault acknowledge options | use use  stub | Ask | returndefault acknowledge options idx | Implemented | userq_provider_swap_test.go |
| TC-M14-02 | Positive | replace provider route by  | SetProvider self define definition  | Ask | route by to self define definition real now  | Implemented | userq_provider_swap_test.go |
| TC-M14-03 | Boundary | multiSelect multiselect return | start use  multiSelect | AskDetailed | returnmultiselect result | Implemented | userq_provider_swap_test.go |
| TC-M14-04 | Boundary | custom inputreturn | use user self define definition input | Ask | return custom string | Implemented | userq_provider_swap_test.go |

### M15 / M41 Commands (Slash Commands) (Completed)

> Goal: CommandDefinition + command/run & command/done event + ctx.commands register/list/execute；/plan /goal commandinputport 。M41 as  M15   cluster-7 index refer （mergereal now ）。
> Related Files: `tests/commands_slash_dispatch_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M15-01 | Positive | /plan off direct writeevent | execute /plan off | Inspect event stream | direct connect write plan/mode(off) and non  user/message | Implemented | commands_slash_dispatch_test.go |
| TC-M15-02 | Positive | command/run & done complete | executeany intent command | Inspect event stream | command/run + command/done completepair for  | Implemented | commands_slash_dispatch_test.go |
| TC-M15-03 | Positive | register/list/execute | registernewcommand | execute | can register、list out 、execute | Implemented | commands_slash_dispatch_test.go |
| TC-M15-04 | Exception | un-registercommand | executeunknowncommand | dispatch | report error not  panic | Implemented | commands_slash_dispatch_test.go |

### M16 Session Projections Registry (Completed)

> Goal: ProjectionDefinition[State any] + register / snapshot / subscribe(changelog)；SDK Readdispatch produce Status unique 1 standard standard connect port 。
> Related Files: `tests/session_projection_test.go`（2 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M16-01 | Integration | 4 deliver shadow  subscribe incremental | register Goal/Todo/Plan/Sandbox deliver shadow  | ApplyEvents + subscribe | changelog incrementalcorrect | Implemented | session_projection_test.go |
| TC-M16-02 | Integration | snapshot and fold consistent | executemultievent | snapshot vs fold* | resultconsistent | Implemented | session_projection_test.go |
| TC-M16-03 | Boundary | rebuild full volume rebuild | delete eliminate Statusafter | rebuild | from eventstreamcompleterebuildStatus | Supplemental | — |

### M17 Session References (Cross-session & File Mention) (Completed)

> Goal: @session/xxx and #path/file lang method  mention parse + PreparedReferencedMessage + stableerrorcode。
> Related Files: `tests/session_reference_test.go`（7 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M17-01 | Positive | multi mention splice inputcontext | consume rest  mention 3   session+file | Prepare | PreparedReferencedMessage splice inputcontextcorrect | Implemented | session_reference_test.go |
| TC-M17-02 | Exception | refer use not save at  session | mention Noneeffect  session | Prepare | SESSION_NOT_FOUND splitclasseserrorcode | Implemented | session_reference_test.go |
| TC-M17-03 | Exception | fileexceed boundary /not save at  | mention exceed boundary /lack lose file | Prepare | FILE_OUT_OF_WORKSPACE / FILE_NOT_FOUND errorcode | Implemented | session_reference_test.go |
| TC-M17-04 | Boundary | self refer use reject | refer use self body  | Prepare | self refer use errorcode | Implemented | session_reference_test.go |
| TC-M17-05 | Boundary | refer use over limit /budgetreject | over count volume up limit orbudgetnot enough  | Prepare | rejectrefer use errorcode | Implemented | session_reference_test.go |
| TC-M17-06 | Integration | writeinput source=reference | parsesucceedafter | inspect user/message | source=reference correctwriteinput | Implemented | session_reference_test.go |
| TC-M17-07 | Boundary | path-only exceed boundary determine define  | workspace outer path | WorkspaceFileResolver | determine define exceed boundary correct | Implemented | session_reference_test.go |

## 3. Cluster 3: Safety (cluster-3-safety)

### M22 PreToolDecision Three-state (allow/deny/ask) (Completed)

> Goal: tools/pre-execute waterfall return3 state  + approval ask→allowed-once lang definition 。
> Related Files: `tests/tools_predecision_ask_once_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M22-01 | Positive | deny short route  | pre-execute return deny | executetool | toolnot execute，return deny result | Implemented | tools_predecision_ask_once_test.go |
| TC-M22-02 | Positive | ask communicate pass  allowed-once | ask is use user communicate pass  | executelocal timesinvoke | only local times tool/call execute | Implemented | tools_predecision_ask_once_test.go |
| TC-M22-03 | Boundary | down timessame toolcontinue proceed  ask | same toolthen timesinvoke | then timesexecute | not is permanent long release execute ，continue proceed  ask | Implemented | tools_predecision_ask_once_test.go |
| TC-M22-04 | Boundary | ask reject deny | ask is reject  | execute | etcsame  deny，toolnot execute | Implemented | tools_predecision_ask_once_test.go |

### M24 Tool Restriction allow/deny (Completed)

> Goal: Scope stage tool mask + intersect + scope tool exempt；Subagent parent limit sub-capability force  / Preset hidden hide tool。
> Related Files: `tests/tools_restriction_intersect_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M24-01 | Positive | host deny + scope exempt recover | host deny a tool | scope exempt afterinvoke | toolrecoveravailable | Implemented | tools_restriction_intersect_test.go |
| TC-M24-02 | Integration | 2 layermutually cross  | host + scope all setting limit control  | Filter | 2 layercross collectioncorrect | Implemented | tools_restriction_intersect_test.go |
| TC-M24-03 | Integration | sub-agentparent limit sub- | parent limit sub-capability force  | Filter sub-agent | sub-agentcapability force is limit control  | Implemented | tools_restriction_intersect_test.go |
| TC-M24-04 | Integration | pre setting hidden hide tool | preset hidden hide tool | Filter | toolnot can see  | Implemented | tools_restriction_intersect_test.go |
| TC-M24-05 | Boundary | nearest-scope-wins step-by-step layer | multilayer scope | parselimit control  | most inner layer scope optimal first  | Implemented | tools_restriction_intersect_test.go |

### M25 ToolRunContext deferContext + concludeTurn (Completed)

> Goal: composite tool embedsuitesplitsend communicate channel  + end stop local  turn  auth risk standard record 。
> Related Files: `tests/tools_conclude_turn_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M25-01 | Integration | concludeTurn end stop local  turn | report_blocker call  concludeTurn | observe turn | turn establish immediate walk  turn-stopping/turn/end，not then down 1   Step | Implemented | tools_conclude_turn_test.go |
| TC-M25-02 | Positive | deferContext pass broadcast  | embedsuite composite tool | splitsend sub-tool | embedsuitesplitsend communicate channel correct | Implemented | tools_conclude_turn_test.go |
| TC-M25-03 | Boundary | None conclude not error trigger | normaltoolinvoke | observe | not end stop  turn | Implemented | tools_conclude_turn_test.go |
| TC-M25-04 | Integration | multilayer context pass broadcast  | WithConclude embedsuite | ConcludeFrom | pass broadcast chaincorrect | Implemented | tools_conclude_turn_test.go |

### M26 Sandbox Seam (3 Modes) (Completed)

> Goal: mode in {read-only / workspace-write / danger-full-access} + {root path, enforced} meta set；Bash/FS consume cost entity system 1 。
> Related Files: `tests/sandbox_mode_apply_test.go`（6 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M26-01 | Integration | Bash & FS consume cost consistentpolicy | setting set  Sandbox Mode | 2 consume cost entity fetchpolicy | all obtain to consistent ExecutionPolicy（enforced + scope） | Implemented | sandbox_mode_apply_test.go |
| TC-M26-02 | Positive | 3 Modes Switching | Configure 3 modes sequentially | Get policy | Each mode maps correctly | Implemented | sandbox_mode_apply_test.go |
| TC-M26-03 | Boundary | Priority: explicit > session > default | Multi-tier configuration | Parse | Explicit wins, fallback persists | Implemented | sandbox_mode_apply_test.go |
| TC-M26-04 | Exception | Confine fail-closed | Provider not available | Confine | SANDBOX_UNAVAILABLE error | Implemented | sandbox_mode_apply_test.go |
| TC-M26-05 | Positive | RunnerFailureRule verify data rules | commandfail | determine define  | verify data rulescorrect | Implemented | sandbox_mode_apply_test.go |

### M27 Approval Policy Seam (Completed)

> Goal: policy 4 enum action  + use user stage/sessionstage override + ask→allowed-once。
> Related Files: `tests/approval_override_order_test.go`（7 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M27-01 | Integration | 3 layer override order order  | preset+user+session each setting  policy | Effective | session>use user >pre setting ，order order correct | Implemented | approval_override_order_test.go |
| TC-M27-02 | Positive | ask invoke UQ succeed | ask policy | triggertool | UQ ask succeedtracecomplete | Implemented | approval_override_order_test.go |
| TC-M27-03 | Exception | UQ fail fail-closed | UQ throw error  | ask | fallback stageas  deny | Implemented | approval_override_order_test.go |
| TC-M27-04 | Boundary | allowed-once | ask communicate pass aftersame toolthen call  | then timestrigger | only release execute when times，down timescontinue proceed  ask | Implemented | approval_override_order_test.go |
| TC-M27-05 | Positive | 4 enum action policy | step-by-step 1 setting set  | Effective | allow-all/deny-all/ask-dangerous/ask-dangerous-tool-edit correct | Implemented | approval_override_order_test.go |
| TC-M27-06 | Boundary | Source review count  | parsetake effectpolicy | inspect Source | review count fieldstandard record from sourcelayer | Implemented | approval_override_order_test.go |

### M28 Permission Presets Composition Knob (Completed)

> Goal: {sandboxMode, approvalPolicy} pre setting table + use user self define definition dispatch produce Status。
> Related Files: `tests/permission_presets_combo_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M28-01 | Positive | 4 pre setting setcombinefor should  | select choose  safe/danger/review/custom | Resolve | setcombinemeta setandraw edition 1 1 for should  | Implemented | permission_presets_combo_test.go |
| TC-M28-02 | Boundary | unknownpre setting back persist  custom | pass unknown preset | Resolve | back persist  custom | Implemented | permission_presets_combo_test.go |
| TC-M28-03 | Positive | DerivedState dispatch produce standard record  | custom setcombine | Readdispatch produce Status | custom dispatch produce standard record correct | Implemented | permission_presets_combo_test.go |
| TC-M28-04 | Integration | Mapper connect line | inject ApprovalMapper/SandboxMapper | take effect | singleto connect linecorrect | Implemented | permission_presets_combo_test.go |

### M29 Attachment Image Reference Pattern (Completed)

> Goal: ImageBlock{url? / reference:AttachmentId} + attachment storage + durable reference parse。
> Related Files: `tests/attachment_ref_resolve_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M29-01 | Positive | keep save graphpiece  content-addressed | keep save graphpiece  | generate AttachmentID | sha256 summarycorrect | Implemented | attachment_ref_resolve_test.go |
| TC-M29-02 | Integration | durable cross-session/compactionparse | sessioncompactionafter | ResolveReference | durable pathparsenot invalidate | Implemented | attachment_ref_resolve_test.go |
| TC-M29-03 | Exception | Readre-compute summarynot match pair  | fileis tamper modify  | Read | report error orverifyfail | Implemented | attachment_ref_resolve_test.go |
| TC-M29-04 | Positive | ImageBlock reference refer use  | construct construct refer use  block | serialize | reference fieldcorrect | Implemented | attachment_ref_resolve_test.go |

### M30 Invariant Registry Invariant Validation (Completed)

> Goal: ctx.invariants.register(pkgName, installer) + packageattribute attribute report error ；open send phasestart use ，produce produce close 。
> Related Files: `tests/invariant_pkg_attribution_test.go`（6 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M30-01 | Exception | violate rule report error carry packagename  | turn alreadyopen then times turn/start | trigger invariant | INVARIANT report error carry packagename  prefix | Implemented | invariant_pkg_attribution_test.go |
| TC-M30-02 | Boundary | produce produce close close not report error  | SetEnabled(false) | triggerviolate rule  | not report error  | Implemented | invariant_pkg_attribution_test.go |
| TC-M30-03 | Exception | panic isolate | single invariant panic | run | isolatenot expand disperse  | Implemented | invariant_pkg_attribution_test.go |
| TC-M30-04 | Positive | violate rule account local register record  | multitimesviolate rule  | inspectaccount local  | per itemviolate rule is register record  | Implemented | invariant_pkg_attribution_test.go |

### M31 Token Meter Accounting & Budget (Completed)

> Goal: per-request prompt/completion token + session-level budget cap + tablesurfacenodedefine value 。
> Related Files: `tests/token_meter_budget_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M31-01 | Positive | budgetreach standard afterreject | budget 10k tokens | reach to up limit afterNext round requestbefore | budget deny rejectand not produce produce true real  LLM invoke | Implemented | token_meter_budget_test.go |
| TC-M31-02 | Positive | per-request record | per request | Record | prompt/completion correctaccumulate count  | Implemented | token_meter_budget_test.go |
| TC-M31-03 | Boundary | un-reach budgetnormalrelease execute  | budgetfill enough  | request | normalcommunicate pass  | Implemented | token_meter_budget_test.go |
| TC-M31-04 | Positive | SurfacePricing define value  | tablesurfacenode | define value  | define value correct | Implemented | token_meter_budget_test.go |

### S09 Message Feedback (Completed)

> Goal: rating/note/version(CAS)/createdAt/updatedAt + fail taxonomy + list/put/delete。
> Related Files: `tests/feedback_cas_failure_taxonomy_test.go`

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S09-01 | Exception | CAS conflict conflict splitclasses | Concurrencyupdate new feedback | Put | VERSION_CONFLICT splitclasseserror | Implemented | feedback_cas_failure_taxonomy_test.go |
| TC-S09-02 | Exception | not save at  session | Noneeffect  session | Put | SESSION_NOT_FOUND | Implemented | feedback_cas_failure_taxonomy_test.go |
| TC-S09-03 | Positive | list/put/delete | complete CRUD | execute | datacorrect，version pass increase  | Implemented | feedback_cas_failure_taxonomy_test.go |
| TC-S09-04 | Exception | not save at record | Noneeffect  id | Get/Delete | NOT_FOUND splitclasses | Supplemental | — |

## 4. Cluster 4: Persistence (cluster-4-persistence)

### M43 Persistence Seam + Flush Checkpoint + Batch Window + Crash Repair (Completed)

> Goal: SessionPersistence{locate/load/inspect/append/list/snapshot} + flush checkpoint + batch window port  + repair orphan child  turn。
> Related Files: `tests/persistence_crash_repair_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M43-01 | Exception | crash crash after repair orphan child  turn | append  in  kill | reload → repair | patch write interrupted turn/end | Implemented | persistence_crash_repair_test.go |
| TC-M43-02 | Positive | alreadywriteeventallkeep retain  | crash crash beforewrite chunk/tool/call | reload | allkeep retain  | Implemented | persistence_crash_repair_test.go |
| TC-M43-03 | Boundary | batch window buffer conflict  | batch volume  append | flush | buffer conflict correctmergepersist disk  | Implemented | persistence_crash_repair_test.go |
| TC-M43-04 | Integration | Flush Checkpoint | strong control  flush | inspect checkpoint | checkpoint recordcorrect | Supplemental | — |

### M44 SessionHeader Format Rejection & Versioning (Completed)

> Goal: SESSION_FORMAT_VERSION + SessionFormatUnsupportedError + CorruptionError + KNOWN_SESSION_EVENT_TYPES rejectunknownevent。
> Related Files: `tests/session_format_reject_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M44-01 | Exception | unknowneventType fail-closed | readinputunknownType | LoadSession | reject | Implemented | session_format_reject_test.go |
| TC-M44-02 | Exception | cross-versionreject | different VERSION file | add load  | mutual mutually reject | Implemented | session_format_reject_test.go |
| TC-M44-03 | Exception | damage bad event | malformed form  JSON | add load  | CorruptionError | Implemented | session_format_reject_test.go |

### M45 Storage Domain KV Abstraction (Completed)

> Goal: hub → backend(JSONL/SQLite) → domain + CAS version mismatch + typed read/write。
> Related Files: `tests/storage_domain_cas_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M45-01 | Positive | filekv / sqlitekv dual afterside | cut swap afterside | read-write | Consistent behavior | Implemented | storage_domain_cas_test.go |
| TC-M45-02 | Exception | CAS version mismatch | pass phaseversionwrite | writeinput | VERSION_MISMATCH errorconsistent | Implemented | storage_domain_cas_test.go |
| TC-M45-03 | Integration | cross-aftersidemigrate move  | datamigrate move  | executemigrate move  | idempotent etccan run ，datanot drop  | Implemented | storage_domain_cas_test.go |
| TC-M45-04 | Positive | Typeize  typed read/write | Domain[T] | read-write | Typeize correct | Implemented | storage_domain_cas_test.go |

### S01 Compaction (LLM Summary + Surface Replace) (Pending)

> Goal: >N tokens long for dialogue trigger → yield  LLM compactionold  surface → generate assistant/message{replace range}。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-S01-01|Positive| Threshold Triggered Compaction |long for dialogue  >N tokens|trigger compact|LLM generatesummary and writeinput replace|Pending| compaction_replace_surface_test.go (reused in M21) |
|TC-S01-02|Integration| Resume Conversation After Compaction |already compact|send newconsume rest  turn|turn normalcontinue proceed |Pending| compaction_engine_basic_test.go |
|TC-S01-03|Integration| Header Rebuild Independent of Raw Events |compact after|RebuildFromHeader|rebuildsucceed|Pending| — |
|TC-S01-04|Boundary| Below Threshold Does Not Trigger |short for dialogue |observe|not triggercompaction|Pending| — |

### S03 SQLite Persistence Backend + FTS5 Index (Completed)

> Goal: sqlite-session single DB + session eventexecute ize  + FTS5 + atomicwriteinput。
> Related Files: `tests/persistence_sqlite_fts5_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S03-01 | Performance | 10k eventwriteinput | empty library | writeinput 10k event | singlecheck query  <100ms | Implemented | persistence_sqlite_fts5_test.go |
| TC-S03-02 | Positive | FTS5 title/contenthit | build index refer  | search index close keyword  | hitcorrect | Implemented | persistence_sqlite_fts5_test.go |
| TC-S03-03 | Boundary | event service atomicwritecorrect doc +FTS | batch volume writeinput | AppendBatch | correct doc and FTS consistent | Implemented | persistence_sqlite_fts5_test.go |
| TC-S03-04 | Boundary | fallback stage LIKE search index  | <3 wordor FTS not available | Search | fallback stage LIKE available | Implemented | persistence_sqlite_fts5_test.go |

### S04 Session Query + Search (SQLite FTS5) (Pending)

> Goal: SessionListRequest/SessionSearchRequest + by title/created + ctrl+f define position 。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-S04-01|Positive| Title Prefix Filter |multisession|by titlebeforesuffix check query |resultstandard confirm |Pending| session_query_title_content_test.go |
|TC-S04-02|Positive| Creation Time Range Query |multisession|by time space scopecheck query |resultstandard confirm |Pending| session_query_title_content_test.go |
|TC-S04-03|Positive| Contains Keywords Query |multisession|close keyword search index |resultstandard confirm |Pending| session_query_title_content_test.go |
|TC-S04-04|Boundary| Three-field Combination Filter |multisession|setcombineitemitem |pass filter resultprecise confirm |Pending| session_query_title_content_test.go |

### S05 Session Telemetry Hooks (Completed)

> Goal: sessionTelemetry{event/chunk/tool latency}。
> Related Files: `tests/session_telemetry_hooks_fired_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S05-01 | Positive | 3 classeshook sub-trigger | register 3 hook sub- | append/chunk/tool | 3 classeshook sub-all trigger | Implemented | session_telemetry_hooks_fired_test.go |
| TC-S05-02 | Positive | latency combinemanage zone space  | injectcan control time clock  | record tool | latency at zone space inner  | Implemented | session_telemetry_hooks_fired_test.go |
| TC-S05-03 | Exception | singlehook sub- panic isolate | a hook sub- panic | trigger | callSafe isolatenot expand disperse  | Implemented | session_telemetry_hooks_fired_test.go |
| TC-S05-04 | Concurrency | Concurrencysafety | Concurrencytrigger | record | RWMutex safety | Implemented | session_telemetry_hooks_fired_test.go |

### S07 OTel Telemetry Export (Pending)

> Goal: OTLP exporter + resource detector + baggage；and S05 hook for connect 。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-S07-01|Integration| Local Collector Receives Spans |start local local  OTel Collector|produce produce  span|span  contains  session/turn/step baggage|Pending| otel_collector_integration_test.go (optional) |
|TC-S07-02|Positive| Three Signals Export |config OTLP|trigger trace/metric/log|3 classesinfo number all can lead out |Pending| — |
|TC-S07-03|Boundary| Missing Collector Does Not Block |Collector not can reach |trigger|fallback stagenot block block main streamprocess |Pending| — |

### S08 Session Title (Completed)

> Goal: latest-wins fold + fallback human msg prefix 30 word + LLM auxiliary assist generate。
> Related Files: `tests/session_title_fallback_llm_test.go`（8 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S08-01 | Positive | fallback 30 wordtruncate | un-start use  LLM | generatetitle | rune beforesuffix  30 wordtruncate，multiwordnode safety | Implemented | session_title_fallback_llm_test.go |
| TC-S08-02 | Positive | LLM generatetitle | start use  LLM | Generate | titlewriteinput session/title | Implemented | session_title_fallback_llm_test.go |
| TC-S08-03 | Exception | LLM failback exit  | LLM throw error  | Generate | back exit  fallback | Implemented | session_title_fallback_llm_test.go |
| TC-S08-04 | Exception | LLM empty outputback exit  | LLM returnempty  | Generate | back exit  fallback | Implemented | session_title_fallback_llm_test.go |
| TC-S08-05 | Integration | latest-wins fold | multitimeswriteinput | FoldSessionTitle | most newtitletake effect | Implemented | session_title_fallback_llm_test.go |

## 5. Cluster 5: Subagents (cluster-5-subagent)

### S02 Subagent Seam (3 Backends) (Pending)

> Goal: in-process fork / ACP child / fork-copy-process 3   Provider。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-S02-01|Positive| Three Backend Unit Tests |construct construct  3 Provider|step-by-step 1 run|each self execute as correct|Pending| subagent_fork_inprocess_test.go |
|TC-S02-02|Integration| Parent Dispose Auto-drains Children |sub-run in  dispose parent |observe|sub-self dynamic  drain|Pending| — |
|TC-S02-03|Integration| Fork Lineage Correctness |fork sub-agent|inspect header|parent/session lineage correct|Pending| — |

### S10 Terminal PTY (Pending)

> Goal: TerminalBackend/Session + spawn/send/read/signal/close + bounded scrollback + waitReason + single agent exclusive occupy active dynamic 。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-S10-01|Positive| spawn -> send -> read |start  PTY|send  echo hello|read return hello|Pending| terminal_pty_interactive_test.go |
|TC-S10-02|Positive| close Exit Code 0 |start  PTY|close|pty processexit out code 0|Pending| terminal_pty_interactive_test.go |
|TC-S10-03|Boundary| Bounded Scrollback Buffer |large volume output|read|scroll dynamic buffer conflict restricted|Pending| — |
|TC-S10-04|Concurrency| Single Agent Exclusive Lock |multi agent preempt |trigger|only 1   agent active dynamic |Pending| — |

### S12 Workflow Engine + tool-workflow (Pending)

> Goal: Worker thread engine + script local full local (pipeline/parallel/agent/phase) + Chat Durable Records fold invariant。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-S12-01|Positive| Parallel 3 Subagents |encode queue  parallel|run 2 step|aggregate total resultcorrect|Pending| workflow_parallel_subagents_test.go |
|TC-S12-02|Integration| Workflow Cancel Cascade |workflow run in |cancel|sub- agent stagelink  cancel|Pending| — |
|TC-S12-03|Boundary| Chat Durable Records Fold Invariant |record chat record|fold verify|invariantsucceed establish |Pending| — |

### S13 MCP Client (Pending)

> Goal: MCPTransport + MCPClient + list tools + call tool → self dynamic reflect map  ToolDefinition。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-S13-01|Integration| Connect to Sample MCP Server |start  filesystem-server|list tools|toolself dynamic out now at  ToolRegistry|Pending| mcp_client_tool_bridge_test.go |
|TC-S13-02|Positive| Call Tool Mapping |connectionafter|Invoke tool|normalinvoke and return|Pending| — |
|TC-S13-03|Exception| Connection Failure Handling |server not can reach |connect|stableerror，not  panic|Pending| — |

### S14 Workspace Registry (Completed)

> Goal: workspace record{id/root/sessionGroup/resume-on-open} + ctx.workspaces。
> Related Files: `tests/workspace_group_session_test.go`（6 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S14-01 | Positive | mutually same  root idempotent etcreuse use  | create build 2 timessame  root | Create | returnsame 1  id | Implemented | workspace_group_session_test.go |
| TC-S14-02 | Boundary | diff  root diff  id | different root | Create | id different | Implemented | workspace_group_session_test.go |
| TC-S14-03 | Positive | resume-on-open record | setting set default acknowledge  session | SetResumeOnOpen | recordcorrect，can clean eliminate  | Implemented | workspace_group_session_test.go |
| TC-S14-04 | Positive | sessionsplitset | splitsetsession | SetSessionGroup | splitsetcorrect | Implemented | workspace_group_session_test.go |
| TC-S14-05 | Exception | empty  root reject | empty  root | Create | reject | Implemented | workspace_group_session_test.go |

### S15 LLM Retry (Completed)

> Goal: exponential backoff + max attempts + jitter + write llm/retry event。
> Related Files: `tests/llm_retry_backoff_event_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S15-01 | Positive | 3 times overload aftersucceed | model mock  3 times overload | retry | write 3 item llm/retry，number  4 timessucceed | Implemented | llm_retry_backoff_event_test.go |
| TC-S15-02 | Boundary | over pass  max forward  request-error | persist proceed fail | retryto  max | forward  M34 request-error waterfall | Implemented | llm_retry_backoff_event_test.go |
| TC-S15-03 | Positive | point count exit avoid  + jitter | observeexit avoid  | record | backoff point count increase long carry  jitter | Implemented | llm_retry_backoff_event_test.go |

### S16 Output Retention (Completed)

> Goal: post-execute afterstill keep retain  canonical value；concurrent reader keep protect 。
> Related Files: `tests/output_retention_concurrent_read_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S16-01 | Concurrency | 10MB Concurrency reader complete | keep retain  10MB result | 2 reader Concurrencyread | all returncompletewordnode cut piece  | Implemented | output_retention_concurrent_read_test.go |
| TC-S16-02 | Positive | Canonicalize branch persist  | []byte/string/JSON | Canonicalize | each Typeattribute 1 correct | Implemented | output_retention_concurrent_read_test.go |
| TC-S16-03 | Boundary | Remove logic logic delete eliminate  | keep retain afterdelete eliminate  | Read | lack lose correct | Implemented | output_retention_concurrent_read_test.go |

## 6. Cluster 6: File & Process Execution (cluster-6-file-process)

### M35 Filesystem Seam + obs-policy + tool-fs (Completed)

> Goal: FsTarget/Version/Info/EditRequest/WriteIntent + write/edit/resolve/listDir/stat + observepolicy policy （first readafterwrite）。
> Related Files: `tests/filesystem_obs_policy_test.go`（6 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M35-01 | Exception | obs-policy rejectraw write | un-observe version | fs_write | reject | Implemented | filesystem_obs_policy_test.go |
| TC-M35-02 | Concurrency | Concurrencyencode logic same 1 execute  | 2   fs_edit same 1 execute  | execute | number 2   FS_STALE_VERSION | Implemented | filesystem_obs_policy_test.go |
| TC-M35-03 | Positive | resolve/stat/read/listDir | normalpath | execute | returncorrect | Implemented | filesystem_obs_policy_test.go |
| TC-M35-04 | Positive | atomicwrite + versioncommand card  | writefile | inspect | atomicwritetake effect，versionupdate new | Implemented | filesystem_obs_policy_test.go |
| TC-M35-05 | Exception | stableerrorcode | pathexceed boundary /not save at  | operate operate  | splitclasseserrorcode | Implemented | filesystem_obs_policy_test.go |

### M36 Subprocess Seam (Completed)

> Goal: SubprocessSpawnSpec + tree terminate + CollectedOutput{truncated/spillPath} + scrubbedParentEnv。
> Related Files: `tests/subprocess_tree_terminate_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M36-01 | Integration | Win taskkill /T full treeclean | start subprocesstree | Terminate | Noneresidual retain process | Implemented | subprocess_tree_terminate_test.go |
| TC-M36-02 | Integration | *nix SIGTERM→grace→SIGKILL | start subprocesstree | Terminate | treecleancorrect | Implemented | subprocess_tree_terminate_test.go |
| TC-M36-03 | Positive | outputover threshold value spill | large output | CollectedOutput | truncate+spillPath correct | Implemented | subprocess_tree_terminate_test.go |
| TC-M36-04 | Boundary | DSH_* env scrub | loopenv  contains  DSH_* | spawn | parent loopenv is clean | Implemented | subprocess_tree_terminate_test.go |

### M37 Shell/Bash Seam + tool-bash (Completed)

> Goal: ShellExecRequest→resolve()→ExecSpec + RunResult 5 fieldcorrect cross  + SandboxExecutionPolicy + beforeplatform  run / afterplatform  Job。
> Related Files: `tests/bash_subprocess_spill_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M37-01 | Positive | long outputtruncate+spill complete | bash('seq 1 100000') | run | 64kb truncate，read spillPath take complete 100000 execute  | Implemented | bash_subprocess_spill_test.go |
| TC-M37-02 | Boundary | timeout timedOut | timeout 30s | runlong command | RunResult.timedOut=true | Implemented | bash_subprocess_spill_test.go |
| TC-M37-03 | Positive | 5 correct cross field | each field scene run | inspect RunResult | exitCode/signal/timedOut/aborted/timeoutMs correct cross correct | Implemented | bash_subprocess_spill_test.go |

### M42 Spill Storage Overflow Seam (Completed)

> Goal: ctx.spill{previewBytes/maxTotalBytes/writeFile ref} + spill-policy operate as  post-tool/Bash listener。
> Related Files: `tests/spill_recovery_roundtrip_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M42-01 | Positive | tool/Bash resultsplitdistinguish  spill | resultover threshold value | Apply | all correct spill | Implemented | spill_recovery_roundtrip_test.go |
| TC-M42-02 | Integration | readrefer use return raw consistent | already spill | readrefer use  | contentwordnode consistent | Implemented | spill_recovery_roundtrip_test.go |
| TC-M42-03 | Boundary | private has  0700 + exclusive occupy write | create build  spill file | inspect | permission 0700，wx exclusive occupy  | Implemented | spill_recovery_roundtrip_test.go |
| TC-M42-04 | Exception | keep save fail best-effort | writefail | Apply | not block block main streamprocess  | Implemented | spill_recovery_roundtrip_test.go |

### M46 Job Lifecycle Owner Binding (Completed)

> Goal: owner Agent bind define  dispose cancel + JobSnapshot{ownerSession}。
> Related Files: `tests/jobs_incremental_output_test.go`（override）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M46-01 | Integration | Dispose → cancelname down all Jobs | multi Job run | DisposeOwner | allwalk  cancel hook | Implemented | jobs_incremental_output_test.go |
| TC-M46-02 | Integration | orphan child processclean | Dispose after | inspect | orphan child processthorough bottom clean | Implemented | jobs_incremental_output_test.go |
| TC-M46-03 | Positive | JobSnapshot ownerSession | create build  Job | Readfast photo  | ownerSession correct | Implemented | jobs_incremental_output_test.go |

### S11 Jobs Runtime (Completed)

> Goal: JobStart/Hooks/Snapshot + read/waitForDone/cancel/listByOwner。
> Related Files: `tests/jobs_incremental_output_test.go`

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S11-01 | Positive | incremental read | afterplatform cycle loopoutput | per  100ms read | readto down 1  count word | Implemented | jobs_incremental_output_test.go |
| TC-S11-02 | Positive | Job cancel | run in  Job | cancel | canceltake effect | Implemented | jobs_incremental_output_test.go |
| TC-S11-03 | Positive | listByOwner | multi owner | list out  | by  owner pass filter correct | Implemented | jobs_incremental_output_test.go |

## 7. Cluster 7: Config & Credentials & Skills (cluster-7-config)

### M38 Settings Seam + pathop + CAS + secrets (Completed)

> Goal: SettingsNamespace + SettingsScope(get/watch/update/replace) + SettingsPathOp{set/unset} + describe(redactSecrets:true) + expectedRevision CAS + secrets role redaction。
> Related Files: `tests/settings_pathop_secrets_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M38-01 | Positive | secret detach sensitive  describe | secret field | describe(redactSecrets:true) | only provide path + set/unset operate operate position ，value full detach sensitive  | Implemented | settings_pathop_secrets_test.go |
| TC-M38-02 | Concurrency | CAS expectedRevision | Concurrency2   replace | write | afterwrite expectedRevision not token trigger CAS error | Implemented | settings_pathop_secrets_test.go |
| TC-M38-03 | Positive | host/session splitlayer | 2 layersetting set  | Get | nearest-wins correct | Implemented | settings_pathop_secrets_test.go |
| TC-M38-04 | Positive | PathOp set/unset | operate operate config | inspect | set/unset take effect | Supplemental | — |

### M39 Credentials & Authorization Seam (Completed)

> Goal: CredentialRef(POSIX change volume name item card ) + per request resolve + describe/writable + set/unset + AuthorizationFlow + record modify CAS。
> Related Files: `tests/credentials_per_request_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M39-01 | Positive | per request resolve | LLM request in  | per turn resolve | per turnparse1 times CredentialRef | Implemented | credentials_per_request_test.go |
| TC-M39-02 | Positive | modify env afternewvaluecan see  | modify env | Next round request | inspect to newvalue | Implemented | credentials_per_request_test.go |
| TC-M39-03 | Positive | authorize auth streamregister and  begin | register flow | begin | succeed | Implemented | credentials_per_request_test.go |
| TC-M39-04 | Positive | Storage Domain CAS persistence | modify credential | inspect | CAS persistencecorrect | Implemented | credentials_per_request_test.go |

### M40 Skill System 6-layer Rank + fsnotify Watcher + tool-skill (Completed)

> Goal: SkillProvider.list/get + SkillCandidate{rank/locator} + 6 layer rank + fsnotify + skills/change event + invocable policy。
> Related Files: `tests/skills_registry_fsnotify_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-M40-01 | Integration | newbuild  skill take effect | newbuild  my-skill.md | skills.list | down 1 times list out now  | Implemented | skills_registry_fsnotify_test.go |
| TC-M40-02 | Integration | delete eliminate  skill consume lose  | delete eliminate  skill | skills.list | down 1 timesnot out now  | Implemented | skills_registry_fsnotify_test.go |
| TC-M40-03 | Positive | 6 layer rank same name resolve win  | multilayersame  skill | parse | rank high entity win  | Implemented | skills_registry_fsnotify_test.go |
| TC-M40-04 | Integration | skill(name) inject | Invoke tool | inspect | injected-context user/message correct | Implemented | skills_registry_fsnotify_test.go |

### S06 Authorization Service (OAuth Flow Stub) (Completed)

> Goal: credentials   flow part split；stub capability list out /open start /cancelimmediate can 。
> Related Files: `tests/authorization_flow_stub_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-S06-01 | Integration | list→begin→back call →resolve | register flow | completetrace | resolved credential can is  M39 resolve | Implemented | authorization_flow_stub_test.go |
| TC-S06-02 | Boundary | cancel | begin after | cancel | Status cancelled | Implemented | authorization_flow_stub_test.go |
| TC-S06-03 | Exception | dual completedreject | alreadycompleted flow | then times Complete | reject | Implemented | authorization_flow_stub_test.go |
| TC-S06-04 | Exception | unknown token | Noneeffect  token | check query  | error | Implemented | authorization_flow_stub_test.go |

## 8. Cluster 8: Cache Affinity (cluster-8-cache-affinity)

### N01 DeepSeek Prefix Cache Instrumentation Probes (Completed)

> Goal: TokenUsage patch full  + CacheStats + ComputeHitRatio() guard  0/0 NaN + DeepSeek client SSE parse + Token Meter increase strong 。
> Related Files: `tests/cache_probe_unit_test.go`（5）+ `tests/cache_probe_deepseek_client_test.go`（1）PASS

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N01-01 | Positive | cachefieldpatch full  | TokenUsage | serialize | PromptCacheHitTokens + PromptCacheMissTokens save at  | Implemented | cache_probe_unit_test.go |
| TC-N01-02 | Boundary | ComputeHitRatio guard  NaN | 0/normal/full hit/full un-hit | count compute  | each field scene confirm define value | Implemented | cache_probe_unit_test.go |
| TC-N01-03 | Positive | parseUsage parse SSE | Mock SSE response | parseUsage | cachefieldparsecorrect | Implemented | cache_probe_deepseek_client_test.go |
| TC-N01-04 | Positive | log contains  hit_ratio | per times LLM request | inspectlog | cache.hit_ratio=... fieldsave at  | Implemented | cache_probe_unit_test.go |

### N02 D1 Strict Append-only Fold + SurfaceOp Hardening (Completed)

> Goal: Session only break expose  Append() + SurfaceOp tablesurfacereplace + 8 iteminvariant day-1 verify。
> Related Files: `tests/session_invariant_8_test.go`（7 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N02-01 | Positive | only  Append writepath | encode translate inspect | scan describe  Session method method  | Noneitsother  public/private writemethod method  | Implemented | session_append_only_test.go |
| TC-N02-02 | Boundary | 8 iteminvariant | construct construct anti example  | VerifyInvariants | alloverride | Implemented | session_invariant_8_test.go |
| TC-N02-03 | Exception | anti example catch get  | inject seq jump number /time back exit /turn not pair for  | verify | is  invariant catch get  | Implemented | session_invariant_8_test.go |
| TC-N02-04 | Exception | failreject append + account local  | invariant fail | append | throw  InvariantError + reject + register record  ledger | Implemented | session_invariant_8_test.go |
| TC-N02-05 | Positive | 50 turn seq/time strict format  | 50 turnfor dialogue  | verifyfile | seq strict format  1..N + time monotonic | Implemented | session_invariant_8_test.go |

### N03 D2 System Prompt Template Copy + Static Order + Static Check (Completed)

> Goal: sections strict format copy budget raw edition  + Order() read const + forbid insert dynamic state value + runtime  Recorder + CI static state check test 。
> Related Files: `tests/sysprompt_static_check_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N03-01 | Positive | render taint  1000 timesstep-by-step wordnode mutually same  | each  Section | Render 1000 times | step-by-step wordnode mutually same （pure function count ） | Implemented | sysprompt_static_check_test.go |
| TC-N03-02 | Exception | Dynamic Values Anti-pattern Check | Contains time.Now/rand etc | StaticCheck | Detects violation | Implemented | sysprompt_static_check_test.go |
| TC-N03-03 | Positive | systemsystem  hash cross-turnstable | 50 turnfor dialogue  | compare compare  hash | system_prompt_hash cross-turnmutually same  | Implemented | sysprompt_static_check_test.go |
| TC-N03-04 | Boundary | Order() read const | runtime try test override | Read | not capability is override | Implemented | sysprompt_static_check_test.go |

### N04 D3 Skills Catalog Stable Serialization + Change-only Injection (Completed)

> Goal: CatalogText wordclassic order stable + MaybeInjectCatalog hash diff change-only inject + fsnotify refresh new。
> Related Files: `tests/skill_catalog_change_only_test.go`（3 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N04-01 | Positive | CatalogText 1000 timesstep-by-step wordnode mutually same  | multi skill | invoke 1000 times | Nonewith machine /Nonetime space stamp  | Implemented | skill_catalog_change_only_test.go |
| TC-N04-02 | Integration | increase delete  skill trigger inject | increase delete  skill | next call | hash change → inject trigger | Implemented | skill_catalog_change_only_test.go |
| TC-N04-03 | Boundary | 50 turnnot change only inject 1 times | 50 turnNonechange | observe | <available_skills> only inject 1 times | Implemented | skill_catalog_change_only_test.go |

### N05 D4 PromptContext Change-only Injection + Compaction Retains Snapshot (Completed)

> Goal: PromptContext registertable + change-only hash persistence + GoalRoundContext proceed turn + Compaction keep retain most after context snapshot。
> Related Files: `tests/prompt_context_change_only_test.go`（5 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N05-01 | Boundary | Compute hash stable/change | Statuschange /not change  | Compute | Nonechangestable，changemodify change  | Implemented | prompt_context_change_only_test.go |
| TC-N05-02 | Integration | goal.complete stop stop inject | goal complete | observe | self dynamic stop stop  | Implemented | prompt_context_change_only_test.go |
| TC-N05-03 | Integration | compaction keep retain most new snapshot | execute compaction | check  derived messages | most after snapshot still can find to  | Implemented | prompt_context_change_only_test.go |
| TC-N05-04 | Integration | change update walk  user-msg not modify system | plan/goal/approval change update  | inspect | all user-msg append add  | Implemented | prompt_context_change_only_test.go |
| TC-N05-05 | Performance | 50 turn + cut swap afterhitrate  | 50 turn+plan cut swap +goal | E2E | hitrate  ≥ 95% | Implemented | prompt_context_change_only_test.go |

### N06 4 Anti-pattern Defenses + Custom Lint Tool (Completed)

> Goal: static_check + encode translate phasereject + JSON Schema define order  + catalog wordclassic order ；internal/lint + check-cache-safety.sh。
> Related Files: `tests/cache_safety_lint_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N06-01 | Exception | 4 Classes Anti-pattern AST Scan | Inject anti-pattern code | ScanDir/ScanFile | Detects Violation | Implemented | cache_safety_lint_test.go |
| TC-N06-02 | Positive | schema define order output | reuse mix  schema | compileToJSONSchema | properties/required/enum wordclassic order  | Implemented | cache_safety_lint_test.go |
| TC-N06-03 | Boundary | ToolRegistry.List stable | multitimes List | compare compare  | wordclassic order stable | Implemented | cache_safety_lint_test.go |
| TC-N06-04 | Integration | check-cache-safety.sh 0 violations | runscript local  | scan describe  | 0 violations | Implemented | cache_safety_lint_test.go |

### N07 Cache Hit Rate E2E Acceptance Suite (5 Tests) (Completed)

> Goal: tests/cache_hit_rate_e2e_test.go submit provide  5   E2E test + deepseek_mock.go submit provide  Mock beforesuffix cachemodel mock 。
> Related Files: `tests/cache_hit_rate_e2e_test.go`（T1-T5 full  PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N07-01 | Performance | T1 50 turnstablerate  | stablefield scene  | run  50 turn | level all hitrate  ≥ 95% | Implemented | cache_hit_rate_e2e_test.go |
| TC-N07-02 | Boundary | T2 cut  preset invalidate | cut  preset | observe | before 5 turn < 50%，afterstable，end  5 turn ≥ 80% | Implemented | cache_hit_rate_e2e_test.go |
| TC-N07-03 | Integration | T3 compaction recover | compaction after | observe | 30 turninner recover ≥ 95% | Implemented | cache_hit_rate_e2e_test.go |
| TC-N07-04 | Concurrency | T4 multi session Concurrency | 10 Concurrency | run | each  ≥ 85% | Implemented | cache_hit_rate_e2e_test.go |
| TC-N07-05 | Performance | T5 toolcount volume not fallback  | 5/10/20 tool | run | hitrate all  ≥ 95% | Implemented | cache_hit_rate_e2e_test.go |

### N08 Cache Breaking Alert (Completed)

> Goal: CacheAlert：singletimesconflict fallback  > 30% warn；continuous 5 times < 50% error + can select  webhook。
> Related Files: `tests/cache_alert_threshold_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N08-01 | Exception | singletimesconflict fallback  >30% warn | hitrate step fallback  | Observe | warn log contains  session/current/previous | Implemented | cache_alert_threshold_test.go |
| TC-N08-02 | Exception | continuous 5 times <50% error | continuouslow hit | Observe | error log + can select  webhook | Implemented | cache_alert_threshold_test.go |
| TC-N08-03 | Positive | recoverafterre-set  | hitrate recover | Observe | consecutiveFails attribute zero  | Implemented | cache_alert_threshold_test.go |
| TC-N08-04 | Boundary | threshold valuecan config | modify Config | Observe | newthreshold valuetake effect | Implemented | cache_alert_threshold_test.go |

### N09 Grafana Cache Hit Rate Dashboard + OTel Probes (Completed)

> Goal: 4   OTel metric + 3 surfaceboard inspect board  + alertrules。
> Related Files: `tests/cache_metrics_otel_integration_test.go`（4 cases PASS）

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
| TC-N09-01 | Integration | 4 metriccorrectlead out  | Mock OTel collector | trigger |  contains  session/preset/turn standard tag  | Implemented | cache_metrics_otel_integration_test.go |
| TC-N09-02 | Integration | Grafana inspect board  schema verify | inspect board  JSON | verify | 3 surfaceboard complete | Implemented | dsh-cache-dashboard.json |
| TC-N09-03 | Boundary | alertrules | hit_ratio_p50<0.8 persist proceed  10min | trigger | fire | Implemented | cache_metrics_otel_integration_test.go |
| TC-N09-04 | Positive | broken_count Metrics Count | Detect cache breaking event | MarkBroken | Metric increments by 1 | Implemented | cache_metrics_otel_integration_test.go |

## 9. Cluster 9: Concurrency Hardening (cluster-9-concurrency-hardening)

### H01 Agent Request ctx Propagation (Pending)

> Goal: cancel/timeout/append trace pass broadcast enter toolreal now and LLM request；stress testNoneinvokeleak leak 。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H01-01|Integration| Client Cancel Propagation |tool/LLM execute in |cancel|all collect to  Done，Agent by  aborted/interrupted collect tail |Pending| ctx_cancel_propagation_test.go |
|TC-H01-02|Boundary| Timeout Budget Enforcement |budgetto phase|continue proceed request|not then send start newinvoke，alreadyhas invokeby cancellang definition close close |Pending| agent_timeout_budget_test.go |
|TC-H01-03|Integration| Trace/Span Propagation |has inputport  span|Invoke tool|span 1 route carry to tool ctx|Pending| — |
|TC-H01-04|Concurrency| Stress Test No Goroutine Leaks |high Concurrency|monitor control |goroutine/in-flight collect converge |Pending| — |

### H02 Persistence Global Lock Sharding (Pending)

> Goal: by  SessionID hash wish shardlockorper sessiondiff stepbatch volume  writer。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H02-01|Concurrency| 32 Concurrent Sessions No Lock Contention |32 sessionConcurrency|Append|shard lockall even splitdistribute ，Noneetcwait hot point|Pending| persistence_concurrent_shard_test.go |
|TC-H02-02|Integration| Crash Repair Semantic Consistency |shard/diff step writer|triggercrash crash |orphan child  turn still correctpatch write|Pending| persistence_async_writer_flush_test.go |
|TC-H02-03|Integration| Exit Flushes All to Disk |batch volume persist disk |processexit out |Flush Checkpoint allpersist disk not drop |Pending| — |

### H03 LLM HTTP Client Timeout & Connection Pool Tuning (Pending)

> Goal: can config Transport + Client.Timeout + 429/pass load and S15 exit avoid link dynamic 。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H03-01|Boundary| MaxConnsPerHost Enforced |over up limit Concurrency|request|queueand non Nonelimit build link |Pending| llm_http_timeout_transport_test.go |
|TC-H03-02|Exception| Timeout Mapping to LlmFailure |requesttimeout|LLM invoke|return LlmFailure{timeout} and non mount dead |Pending| llm_http_timeout_transport_test.go |
|TC-H03-03|Integration| 429 Linkage with S15 Retry |trigger 429|observe|S15 exit avoid  + llm/retry event，recoverafternot then trigger|Pending| llm_ratelimit_backoff_link_test.go |

### H04 Session Derived Incremental Fold (Pending)

> Goal: remove drop  O(N²) full volume re-release ；tail part incremental fold + cursor standard invalidaterebuild。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H04-01|Positive| Incremental Fold Matches Full Replay |long session|incremental fold vs full volume |Field-by-field identical|Pending| derive_incremental_fold_test.go |
|TC-H04-02|Performance| 1k+ Events Near O(1) |1k+ event|singlestepincremental fold|consume time andeventcount resolve couple |Pending| derive_incremental_fold_test.go |
|TC-H04-03|Boundary| Surface Replace Invalidates Rebuild |execute replace|readdispatch produce |cursor standard invalidaterebuild，not returnstale olddispatch produce |Pending| derive_cursor_surface_invalidate_test.go |

### H05 Persistence IO Memory & Read Reuse (Pending)

> Goal: sync.Pool bytes.Buffer + pre splitpair  + Snapshot cursor standard avoid free full volume read。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H05-01|Performance| 100 Append+Flush Memory Reduction |benchmarkfor compare |run bench|splitpair count display manifest down fallback |Pending| persistence_io_reuse_bench_test.go |
|TC-H05-02|Performance| 10k Events Snapshot Avoids Deserialization |10k event|Snapshot()|not then full volume  JSON deserialize|Pending| persistence_snapshot_cursor_test.go |
|TC-H05-03|Integration| Behavioral Consistency |existing has field scene |back attribute |and crash_repair etctestconsistent|Pending| — |

### H06 Tool Pipeline Object Pool & Lazy Allocation (Pending)

> Goal: Meta lazy property initial start ize  + sync.Pool reuse use core core object。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H06-01|Performance| Meta Unwritten Allocates Zero |not write meta|Run|not splitpair  map|Pending| tools_pipeline_pool_bench_test.go |
|TC-H06-02|Concurrency| Pool Reuse Concurrency Safety |high Concurrencytool|go test -race|Nonerace state 、Nonedataserial interfere |Pending| tools_pipeline_pool_safety_test.go |
|TC-H06-03|Integration| Existing Tests PASS |back attribute |run|tools_waterfall/conclude_turn PASS|Pending| — |

### H07 Shared Registry Read-only & Schema Precompiled Cache (Pending)

> Goal: start dynamic phasefreezeread-only + copy-on-write swap table + tools schema pre encode translate cache。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H07-01|Concurrency| Lock-free Concurrent Reads After Freeze |freezeregistertable|Concurrencyread|atomic.Pointer readsafety|Pending| registry_readonly_schema_cache_test.go |
|TC-H07-02|Integration| Copy-on-write Table Swap |runphasechange update |readregistertable|readentity not sense know tear split |Pending| registry_copy_on_write_swap_test.go |
|TC-H07-03|Performance| Schema Compiled Exactly Once |same config N turn|prompt setassemble |only encode translate  1 times|Pending| — |
|TC-H07-04|Boundary| Cache Preserves Lexicographical Order |cachehit|output|still keep persist  N06 define order |Pending| — |

### H08 Goroutine Resource Governance & Single Watcher (Pending)

> Goal: Agent empty idle back collect  + skills single watcher system 1 splitsend 。

| Case ID | Type | Case Name | Preconditions | Steps | Expected Result | Status | Related Files |
|---|---|---|---|---|---|---|---|
|TC-H08-01|Boundary| Idle Worker Reclaim |over empty idle threshold value|observe|worker goroutine exit out ，newrequestcan rebuild|Pending| agent_idle_reclaim_test.go |
|TC-H08-02|Integration| Resume Cold-start Recovery |resume-on-open|re-open |correctcold start dynamic |Pending| — |
|TC-H08-03|Integration| Single Watcher Avoids Duplicate Events |skill change update |observe|still trigger N04 injectand not re-reuse |Pending| skills_single_watcher_test.go |

## 10. COULD Extensions (extensions / ui-skip)

> by down any service all as  **deferred（Deferred）** or **skipped（Skipped）** Status，Caseonly recordacceptancestandard standard ，**when beforenot need seek real now andexecute**。

### C01 web_fetch Tool Custom Integration (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C01-01 | Integration | by  ToolDefinition connect input HTTP tool | service service side register web_fetch tool | can normalinvoke | Deferred |

### C02 Web Capability Seam (ctx.web) (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C02-01 | Integration | browse view device /HTTP extract object reuse use  | provide  C01 reuse use  | extract object layeravailable | Deferred |

### C03 Webhook Runtime (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C03-01 | Integration | eventcall degree andsessioncreate build  | trigger webhook | eventcorrectcall degree 、sessioncreate build  | Deferred |

### C04 LSP Language Server Integration (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C04-01 | Positive | lang definition jump forward /re-command name /refer use  | connection LSP | success capability available（or grep+AST fallback bottom ） | Deferred |

### C05 Typert CLI Framework (Skipped)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C05-01 | — | Nonehead aftersideuse  Gin/gRPC replace era | not implemented | Noneacceptance | Skipped |

### C06 Schedule Session-level Cron Reminder (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C06-01 | Integration | service service define time any service systemsystem for connect  | for connect  M08 Agent | submit wake take effect | Deferred |

### C07 Agent Teams Experimental Features (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C07-01 | Integration | roster/mailbox/task DAG | base at  S02+M45 construct build  | group queue assist operate correct | Deferred |

### C08 Slots UI Composition System (Skipped)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C08-01 | — | UI mutually close ，Nonehead aftersideSkipped | not implemented | Noneacceptance | Skipped |

### C09 Extensions / Dynamic Cordis Plugins (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C09-01 | Integration | Go Plugin connect port  + setcombineregister | real now insert item connect port  | insert item can add load setcombine | Deferred |

### C10 Code Runtime (PTC) Worker Threads (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C10-01 | Integration | PTC worker execute | model type side branch persist after | worker normalexecute | Deferred |

### C11 Conversation Node Engine (Skipped)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C11-01 | — | UI fold stack ，servernot implemented | not implemented | Noneacceptance | Skipped |

### C12 Client Modules / Web Client / Remote API (Skipped)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C12-01 | — | UI and far process  API | not implemented | Noneacceptance | Skipped |

### C13 Web Server + HTTP API (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C13-01 | Integration | Gin/gRPC break expose capability force  | service service side real now  | HTTP API available | Deferred |

### C14 Approval UI / Settings Panel / Command Palette (Skipped)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C14-01 | — | UI | not implemented | Noneacceptance | Skipped |

### C15 Attachment Consumer (image resize / OCR) (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C15-01 | Integration | multimodel state expand expand  | base at  M29 | resize/OCR available | Deferred |

### C16 Sandbox Landlock/Bwrap Platform Enforcement (Deferred)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C16-01 | Integration | Linux/macOS level platform  Provider | base at  M26 | level platform strong control take effect | Deferred |

### C17 Session Control Frame (Skipped)

| Case ID | Type | Case Name | Steps | Expected Result | Status |
|---|---|---|---|---|---|
| TC-C17-01 | — | server stream/channel replace era | not implemented | Noneacceptance | Skipped |

---

## 11. Summary & Regression Recommendations

### 11.1 Test Case Statistics

| splitclasses | any service count  | Casecount  | Implemented(has test) | patch fill recommendations | Pending/Deferred |
|---|---|---|---|---|---|
| MUST (M01-M48) | 48 | 188 | 175 | 13 | 0 |
| MUST (N01-N09) | 9 | 38 | 38 | 0 | 0 |
| SHOULD (S01-S16) | 16 | 59 | 34 | 1 | 24 |
| COULD (C01-C17) | 17 | 17 | 0 | 0 | 17(Deferred/Skipped) |
| MUST (H01-H08) | 8 | 26 | 0 | 0 | 26 |
| **Total** | **98** | **328** | **247** | **14** | **67** |

> description：
> - 「Implemented」= for should success capability alreadyhas eracode + now has  `tests/` testoverride Case；
> - 「patch fill recommendations」= now has testun-explicit confirm override、recommendationsafterproceed patch fill  Boundary/IntegrationCase；
> - 「Pending」= S Cluster pending 7 item + H Cluster 8 item（real now aftermust must by tablepatch test ）；
> - 「Deferred/Skipped」= C Cluster 17 item（deferred/skipped），when beforenot need seek execute。
> - real edge  `tests/` directory contains bound  75  testfile，single testfileinner communicate normal package contains multi sub-Case，andup tableCasenumber large cause for should 。

### 11.2 Regression Priority Recommendations

1. **P0 Core Regression**: M01-M08 + M23 + M30 + M48 (foundation primitives & kernel loop), required on any core change.
2. **P1 safetyback attribute **：M22/M24/M27/M31 + N Clusterall（safetyandCache Affinitycouple combinemost tight secret ）。
3. **P2 success capability back attribute **：M09-M21、M32-M47 + S ClusterImplementeditem。
4. **P3 expand expand back attribute **：H Clusterreal now afteradd input；C ClusterDeferreditemreal now time by tablepatch test 。

### 11.3 Test Execution Commands

```bash
# Run all unit tests
go test ./... -count=1

# runCache Affinity E2E acceptancesuiteitem 
go test ./tests/ -run TestE2E -v -count=1

# runrace state check test （H ClusterConcurrencyCasedepend depend ）
go test -race ./...

# Run static cache safety linter
bash scripts/check-cache-safety.sh
```

---

> This document is maintained in synchronization with [tasks.json](./tasks.json) and [TASKS.md](./TASKS.md).

