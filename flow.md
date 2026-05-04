# CodeDone Flow

## Goal

This document defines the execution flow for CodeDone as a serialized orchestration engine.

The core rule is:

- planning can be multi-stage
- implementer identities can be many
- execution is still serialized, one active implementation ticket at a time

CodeDone is not a worker swarm. It is a controlled ticket-dispatch system with Contre-Maitre oversight at every step.

## Artifact Strategy

Do not use Markdown for everything.

Best choice is a hybrid model:

- Markdown for human-facing planning and review artifacts
- JSON for machine-owned state, queues, tickets, and reports

Reason:

- Markdown is easy for Contre-Maitres to read, critique, and enrich
- JSON is better for deterministic parsing, dispatch, filtering, status tracking, and resume-after-crash behavior

Recommended split:

- `directive.md`
  Human-readable master planning document
- `questions.md`
  Human-readable record of CM1 questions and user answers
- `session.json`
  Canonical session state
- `backlog.json`
  Ordered ticket queue and ticket metadata
- `tickets/T-0001.json`
  One structured ticket per task
- `reports/T-0001.json`
  One structured implementer completion report per completed attempt
- `review-log.jsonl`
  Append-only CM review decisions over time
- `final-report.md`
  Human-readable end-of-session summary

## Session Storage

These artifacts should not live in the user repo by default.

Use a private per-session workspace, for example:

```text
%AppData%/CodeDone/sessions/<session_id>/
```

Recommended layout:

```text
<session_id>/
  session.json
  config-snapshot.json
  questions.md
  directive.md
  backlog.json
  review-log.jsonl
  final-report.md
  tickets/
    T-0001.json
    T-0002.json
  reports/
    T-0001.attempt-1.json
    T-0002.attempt-1.json
```

The user repo remains the implementation target.
The session workspace remains the orchestration brain.

## Main Roles

### CM1

Responsibilities:

- decide whether user questions are needed
- ask at most 5 questions, only at the very beginning
- frame the goal correctly
- identify missing assumptions
- research the codebase
- research the outside world if needed
- draft the first directive

### CM2..CMn-1

Responsibilities:

- read the latest directive first
- critique weak assumptions
- add missing expected features
- improve architecture and scope framing
- reduce ambiguity
- sharpen the quality bar

### Final CM

Responsibilities:

- finalize the directive
- convert the directive into a structured backlog
- create atomic tickets
- define implementer roster/profiles
- dispatch one ticket at a time
- review every completion report and code diff
- decide accept, revise, split, defer, or block

### Implementer

Responsibilities:

- read only the assigned ticket and narrow constraints
- inspect the codebase as needed
- implement one ticket
- commit the work
- write a structured completion report
- stop and wait

### Finalizer

Responsibilities:

- run full validation after backlog completion
- run tests and build checks
- verify end-to-end requirements
- emit a final report and signoff state

## Session State Machine

Use an explicit session state machine.

Recommended top-level states:

- `created`
- `question_gate`
- `planning`
- `directive_review`
- `backlog_frozen`
- `dispatch_ready`
- `implementing`
- `cm_review`
- `finalizing`
- `done`
- `blocked`
- `error`
- `cancelled`

`session.json` should always hold the current state.

Example:

```json
{
  "session_id": "cd-20260427-014500",
  "state": "implementing",
  "user_repo": "C:/Users/abdeb/Documents/GitHub/SomeRepo",
  "created_at": "2026-04-27T01:45:00Z",
  "cm_count": 3,
  "implementer_max": 20,
  "active_ticket_id": "T-0012",
  "active_agent_id": "impl-backend-02",
  "backlog_summary": {
    "todo": 488,
    "in_progress": 1,
    "done": 11,
    "blocked": 0
  }
}
```

## Ticket Model

Tickets should be atomic.

A ticket is not:

- "build the app"
- "implement the browser OS"

A ticket is:

- "add wallpaper selection persistence"
- "implement command palette keyboard open/close behavior"
- "persist app launcher search history"

Recommended ticket schema:

```json
{
  "id": "T-0042",
  "title": "Persist wallpaper selection",
  "type": "feature",
  "area": "desktop-shell",
  "priority": 82,
  "status": "todo",
  "depends_on": ["T-0039"],
  "blocked_by": [],
  "acceptance_criteria": [
    "Selected wallpaper persists across restart",
    "Invalid wallpaper config falls back safely",
    "UI reflects current wallpaper on load"
  ],
  "constraints": [
    "Do not redesign unrelated settings UI",
    "Reuse existing config storage path"
  ],
  "suggested_profile": "impl-frontend",
  "risk_level": "medium"
}
```

## Implementer Roster

The user may configure up to 20 implementers, but that does not mean 20 active workers at once.

Best model:

- maintain a roster of implementer profiles or slots
- allow the final CM to choose which profile handles the next ticket
- still run only one active implementer task at a time

Example 20-slot roster:

```text
impl-frontend-01
impl-frontend-02
impl-frontend-03
impl-backend-01
impl-backend-02
impl-backend-03
impl-backend-04
impl-git-01
impl-tests-01
impl-tests-02
impl-state-01
impl-state-02
impl-data-01
impl-data-02
impl-refactor-01
impl-refactor-02
impl-integrations-01
impl-integrations-02
impl-platform-01
impl-platform-02
```

These are dispatch targets, not simultaneously coding agents by default.

## Why Not Use Only Markdown Tickets

Markdown-only tickets are weaker for the engine because:

- status updates become harder to parse safely
- dependency graphs are harder to query
- sorting and reprioritization become brittle
- resume/recovery logic becomes messy

Use JSON as the source of truth for tickets.
If you want a human-readable mirror later, generate it from JSON.

Good rule:

- `directive.md` is authored and improved by CMs
- `backlog.json` and `tickets/*.json` are authored by the engine

## Question Gate Flow

Question gate happens before deep planning.

Rules:

- CM1 may ask 0 to 5 questions
- if no questions are required, move directly to planning
- after question gate ends, execution enters autonomous mode
- no more user questions unless a hard blocker appears that affects an irreversible decision

Question gate output should be written into both:

- `questions.md`
- `session.json`

## Planning Flow

### Step 1: Session Creation

Engine actions:

1. create session workspace
2. snapshot user config
3. snapshot target repo metadata
4. set session state to `question_gate`

### Step 2: CM1 Question Decision

CM1 decides one of two branches:

1. no questions needed
2. ask up to 5 questions

If questions are asked:

1. write them to `questions.md`
2. pause for user answers
3. record answers
4. mark autonomy as enabled after answers are locked

### Step 3: CM1 Initial Directive Draft

CM1 should:

1. scan the user repo
2. inspect relevant local files
3. inspect prior user answers
4. browse externally if the task requires category research
5. infer missing expectations
6. produce the first `directive.md`

### Step 4: CM2 Review Pass

CM2 should:

1. read `directive.md`
2. identify weak assumptions
3. add missing expected features
4. improve quality targets
5. update `directive.md`

### Step 5: CM3 Final Planning Pass

CM3 should:

1. read the latest `directive.md`
2. decide whether scope is coherent
3. turn the directive into a backlog model
4. generate tickets
5. choose implementer roster utilization
6. freeze the backlog

At the end of this step:

- `directive.md` is frozen for implementation
- `backlog.json` is created
- `tickets/*.json` are created
- session state becomes `dispatch_ready`

## Dispatch Loop

This is the heart of CodeDone.

For each ticket:

1. CM selects the next executable ticket
2. CM chooses the best implementer profile
3. CM marks ticket `in_progress`
4. Implementer receives:
   - ticket JSON
   - compact project charter
   - repo path
   - narrow constraints
5. Implementer changes code
6. Implementer commits
7. Implementer writes `reports/<ticket>.attempt-N.json`
8. Session enters `cm_review`
9. Final CM reviews:
   - completion report
   - git diff
   - commit message
   - optional test output
10. Final CM decides:
   - `accept`
   - `revise`
   - `split`
   - `defer`
   - `block`
11. Backlog is updated
12. Loop repeats

## Completion Report Schema

Implementers should report in JSON, not only prose.

Recommended schema:

```json
{
  "ticket_id": "T-0042",
  "attempt": 1,
  "implementer_id": "impl-frontend-02",
  "status": "done",
  "summary": "Persisted wallpaper selection in config and restored it on startup.",
  "files_changed": [
    "frontend/dist/app.js",
    "frontend/dist/style.css",
    "main.go"
  ],
  "commits": [
    "a1b2c3d"
  ],
  "tests_run": [
    "go build ./..."
  ],
  "important_decisions": [
    "Reused existing config storage path"
  ],
  "known_risks": [
    "No migration path for invalid legacy wallpaper keys"
  ],
  "remaining_work": [],
  "suggested_next_step": "Add wallpaper preview thumbnails in settings."
}
```

## CM Review Decision Model

CM review should be structured, not improvised chat.

Recommended decision payload:

```json
{
  "ticket_id": "T-0042",
  "decision": "accept",
  "reviewer_id": "cm-03",
  "reason": "Acceptance criteria met and no blocking regressions found.",
  "next_ticket_id": "T-0043",
  "followup_notes": []
}
```

Allowed decisions:

- `accept`
- `revise`
- `split`
- `defer`
- `block`

## Backlog Policy

Backlog should not be a flat random list.

Each ticket should be categorized into:

- required for category credibility
- best-in-class enhancement
- stretch / later

This prevents infinite scope explosion while preserving the "top of the top" philosophy.

## Mock Startup Example

This is a mock session startup using:

- 3 Contre-Maitres
- max 20 implementers

### Inputs

- user request arrives
- target repo is selected
- config says:
  - `cm_count = 3`
  - `implementer_max = 20`

### Startup Trace

#### Phase A: Session Boot

1. Engine creates `session_id = cd-20260427-020000`
2. Engine creates session workspace in AppData
3. Engine writes `session.json`
4. Engine sets state to `question_gate`

#### Phase B: CM1

1. CM1 reads user prompt
2. CM1 decides whether clarifying questions are needed
3. Assume CM1 asks 4 questions
4. User answers
5. Engine records answers in `questions.md`
6. CM1 scans repo and external references
7. CM1 drafts `directive.md`

Example CM1 contribution:

- project framing
- explicit assumptions
- initial full feature inventory
- likely stack direction
- first risk register

#### Phase C: CM2

1. CM2 reads `directive.md`
2. CM2 notices missing feature expectations
3. CM2 expands backlog expectations
4. CM2 tightens acceptance quality
5. CM2 updates `directive.md`

Example CM2 contribution:

- identifies missing persistence and recovery behaviors
- adds observability and error handling expectations
- challenges weak architecture assumptions

#### Phase D: CM3

1. CM3 reads final directive draft
2. CM3 freezes planning scope
3. CM3 generates `backlog.json`
4. CM3 generates `tickets/*.json`
5. CM3 evaluates whether all 20 implementer slots are needed
6. CM3 decides only 8 profiles are necessary for this session
7. Engine records the active implementer roster

Example result:

- `impl-frontend-01`
- `impl-frontend-02`
- `impl-backend-01`
- `impl-backend-02`
- `impl-state-01`
- `impl-tests-01`
- `impl-refactor-01`
- `impl-platform-01`

The configured maximum was 20.
The chosen active roster for this session is 8.

#### Phase E: Dispatch Begins

1. CM3 selects `T-0001`
2. CM3 assigns it to `impl-state-01`
3. Implementer completes ticket
4. Implementer commits code
5. Implementer writes `reports/T-0001.attempt-1.json`
6. CM3 wakes and reviews
7. CM3 accepts
8. CM3 selects `T-0002`
9. CM3 assigns it to `impl-frontend-01`
10. Repeat

#### Phase F: Mid-Backlog Behavior

Assume 500 total tickets exist.

CodeDone does not spawn 500 implementers.
CodeDone does not run 20 tickets at once.

Instead:

- one ticket is active
- one implementer handles it
- CM reviews the result
- next ticket is chosen
- implementer selection can change per ticket

This preserves serialized correctness while still allowing specialization.

#### Phase G: Endgame

1. backlog reaches zero executable tickets
2. session state becomes `finalizing`
3. Finalizer runs full validation
4. Finalizer writes `final-report.md`
5. session state becomes `done`

## Engineering Recommendation

Implement in this order:

1. `session.json` state machine
2. `directive.md` generation and CM chain
3. `backlog.json` + ticket schema
4. serialized dispatch loop
5. implementer completion reports
6. CM review decisions
7. finalizer pass

Do not start with real providers first.
Build the orchestration engine with mocks until the flow is stable.

## Final Rule

Contre-Maitres own:

- questioning
- planning
- backlog design
- dispatch
- review

Implementers own:

- one ticket
- one commit
- one completion report

Finalizer owns:

- full-session validation

That separation is what makes the system coherent.
