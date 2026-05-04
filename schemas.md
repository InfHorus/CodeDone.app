# CodeDone Schemas

## Purpose

This document defines the canonical schemas for CodeDone session artifacts.

Rule:

- human-facing planning artifacts use Markdown with fixed sections
- engine-facing state artifacts use JSON or JSONL with fixed fields

These schemas are for the private per-session workspace, not for the user repo.

## Session Workspace

Recommended layout:

```text
<session_id>/
  session.json
  config-snapshot.json
  repo-snapshot.json
  roster.json
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
    T-0001.attempt-2.json
```

## Global Conventions

### IDs

- session id: `cd-YYYYMMDD-HHMMSS-<rand>`
- ticket id: `T-0001`
- agent id examples:
  - `cm-01`
  - `cm-02`
  - `cm-03`
  - `impl-frontend-01`
  - `impl-backend-02`
  - `finalizer-01`

### Timestamps

Use ISO 8601 UTC strings.

Example:

```json
"2026-04-27T02:10:34Z"
```

### Status Enums

Session states:

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

Ticket states:

- `todo`
- `in_progress`
- `done`
- `blocked`
- `deferred`
- `cancelled`

Review decisions:

- `accept`
- `revise`
- `split`
- `defer`
- `block`

Implementer report status:

- `done`
- `partial`
- `blocked`
- `failed`

Priority bands:

- `critical`
- `high`
- `medium`
- `low`

Scope classes:

- `required`
- `best_in_class`
- `stretch`

## 1. `session.json`

This is the canonical runtime state for the session.

### Required Fields

```json
{
  "session_id": "cd-20260427-021034-a7k2",
  "state": "implementing",
  "autonomy_mode": "autonomous",
  "created_at": "2026-04-27T02:10:34Z",
  "updated_at": "2026-04-27T02:26:12Z",
  "user_request": "Build a premium desktop coding assistant.",
  "user_repo_path": "C:/Users/abdeb/Documents/GitHub/CodeDone",
  "workspace_path": "C:/Users/abdeb/AppData/Roaming/CodeDone/sessions/cd-20260427-021034-a7k2",
  "config_snapshot_file": "config-snapshot.json",
  "cm_count": 3,
  "implementer_max": 20,
  "active_cm_id": "cm-03",
  "active_agent_id": "impl-backend-01",
  "active_ticket_id": "T-0012",
  "question_gate": {
    "needed": true,
    "asked": true,
    "max_questions": 5,
    "question_count": 4,
    "answered": true,
    "blocked_on_user": false
  },
  "artifacts": {
    "questions_md": "questions.md",
    "directive_md": "directive.md",
    "roster_json": "roster.json",
    "backlog_json": "backlog.json",
    "review_log_jsonl": "review-log.jsonl",
    "final_report_md": "final-report.md"
  },
  "backlog_summary": {
    "total": 500,
    "todo": 472,
    "in_progress": 1,
    "done": 22,
    "blocked": 3,
    "deferred": 2
  },
  "last_error": null
}
```

### Field Notes

- `autonomy_mode`
  Allowed values:
  - `question_gate`
  - `autonomous`
  - `waiting_on_user`
- `active_cm_id`
  The CM currently responsible for the active orchestration step.
- `active_agent_id`
  The currently active implementer or finalizer.
- `active_ticket_id`
  Null outside implementation or review states.
- `last_error`
  Null unless the session is in `error` or `blocked`.

## 2. `config-snapshot.json`

This stores the resolved session config at start time so the session remains reproducible even if the user changes app settings later.

### Example

```json
{
  "provider": "deepseek",
  "model": "deepseek-chat",
  "cm_count": 3,
  "max_agents": 20,
  "agent_timeout_seconds": 1200,
  "max_output_tokens": 8192,
  "context_strategy": "relevant",
  "enable_finalizer": true,
  "auto_create_branch": true,
  "require_clean_tree": false,
  "auto_commit": true,
  "branch_prefix": "codedone/work-",
  "font_size": 14,
  "font_family": "fira",
  "show_timestamps": true,
  "auto_scroll": true,
  "clear_on_session": false,
  "verbose_mode": false
}
```

## 3. `roster.json`

This defines which agents exist for the session.

### Example

```json
{
  "session_id": "cd-20260427-021034-a7k2",
  "cm_agents": [
    {
      "id": "cm-01",
      "role": "cm_initial",
      "active": true
    },
    {
      "id": "cm-02",
      "role": "cm_reviewer",
      "active": true
    },
    {
      "id": "cm-03",
      "role": "cm_dispatcher",
      "active": true
    }
  ],
  "implementer_agents": [
    {
      "id": "impl-frontend-01",
      "profile": "frontend",
      "active": true,
      "specialties": ["ui", "interaction", "layout"]
    },
    {
      "id": "impl-backend-01",
      "profile": "backend",
      "active": true,
      "specialties": ["go", "services", "state"]
    }
  ],
  "finalizer_agents": [
    {
      "id": "finalizer-01",
      "active": true
    }
  ]
}
```

### Field Notes

- `active` here means available in the session roster, not currently executing.
- `profile` is how CM selects the best implementer for a ticket.

## 3b. `repo-snapshot.json`

This captures the git state of the target repo at session start.

### Example

```json
{
  "path": "C:/Users/abdeb/Documents/GitHub/CodeDone",
  "root": "C:/Users/abdeb/Documents/GitHub/CodeDone",
  "is_repo": true,
  "head_branch": "main",
  "head_commit": "a1b2c3d4",
  "dirty": false,
  "status_lines": [],
  "session_branch": "codedone/work-1714177261"
}
```

### Field Notes

- `dirty`
  True when `git status --short` is non-empty.
- `status_lines`
  Raw short-status lines captured at startup.
- `session_branch`
  Present when CodeDone creates or switches to a dedicated working branch.

## 4. `questions.md`

This is human-readable and should be concise.

### Required Structure

```md
# Questions

## Need For Questions
- Needed: yes
- Asked by: cm-01
- Question count: 4

## User Questions
1. Which operating system should be primary?
2. Do you want local-first persistence or cloud sync?
3. Is keyboard-first interaction mandatory?
4. Should the first version include extensibility/plugin support?

## User Answers
1. Windows first.
2. Local-first.
3. Yes.
4. Not in v1.

## Locked Decisions
- Primary platform: Windows
- Persistence strategy: local-first
- Interaction model: keyboard-first
- Plugins deferred
```

### Rules

- CM1 may ask 0 to 5 questions.
- If 0 questions are needed, the file should still exist and note that explicitly.
- This file is an audit artifact, not engine source of truth.

## 5. `directive.md`

This is the master planning document for the CM chain.

It should be Markdown, not JSON, because it is meant to be iteratively improved by Contre-Maitres.

### Required Sections

```md
# CM Directive

## Session Identity
- Session ID:
- Current CM Owner:
- Directive Version:

## User Request
- Original request:
- Restated goal:

## User Answers And Locked Decisions
- ...

## Assumptions
- Explicit assumptions made by CM:
- Assumptions that still carry risk:

## Product Framing
- What category of product this is
- What "good" means here
- What "top of the top" means here

## Competitive / Reference Intelligence
- Comparable tools
- Common expectations in this category
- Features expected by strong implementations

## Scope Classification
### Required For Category Credibility
- ...

### Best-In-Class Enhancements
- ...

### Stretch / Later
- ...

## Architecture Direction
- Preferred stack direction
- Constraints from existing codebase
- Major technical decisions

## Full Feature Inventory
- Feature list
- System behaviors
- Operational requirements

## Risks And Unknowns
- ...

## Backlog Strategy
- How work should be split
- What should be done first
- What should be deferred

## Dispatch Notes For Final CM
- Recommended implementer profiles
- Likely ticket clusters
- Cross-ticket dependency notes
```

### Directive Metadata Rules

At the top of `directive.md`, include a short metadata block:

```md
- Session ID: cd-20260427-021034-a7k2
- Current CM Owner: cm-02
- Previous CM Owner: cm-01
- Directive Version: 2
- Last Updated: 2026-04-27T02:18:55Z
```

### Ownership Rules

- CM1 creates the first version.
- Each subsequent CM revises the same file.
- Final CM freezes the file before backlog generation.

## 6. `backlog.json`

This is the ordered queue plus summary model for all tickets.

### Example

```json
{
  "session_id": "cd-20260427-021034-a7k2",
  "generated_by": "cm-03",
  "generated_at": "2026-04-27T02:22:11Z",
  "frozen": true,
  "scope_summary": {
    "required": 320,
    "best_in_class": 140,
    "stretch": 40
  },
  "ticket_order": [
    "T-0001",
    "T-0002",
    "T-0003"
  ],
  "ready_queue": [
    "T-0001",
    "T-0002"
  ],
  "blocked_queue": [],
  "deferred_queue": [],
  "done_queue": [],
  "ticket_index": {
    "T-0001": "tickets/T-0001.json",
    "T-0002": "tickets/T-0002.json"
  }
}
```

### Field Notes

- `ticket_order`
  Original strategic order.
- `ready_queue`
  Tickets executable right now.
- `ticket_index`
  Path lookup to detailed ticket files.

## 7. `tickets/T-XXXX.json`

This is the canonical unit of implementation work.

### Required Schema

```json
{
  "id": "T-0042",
  "title": "Persist wallpaper selection",
  "type": "feature",
  "area": "desktop-shell",
  "scope_class": "required",
  "priority": "high",
  "status": "todo",
  "created_by": "cm-03",
  "created_at": "2026-04-27T02:24:08Z",
  "updated_at": "2026-04-27T02:24:08Z",
  "parent_ticket_id": null,
  "depends_on": ["T-0039"],
  "blocked_by": [],
  "source_refs": {
    "directive_sections": [
      "Full Feature Inventory",
      "Backlog Strategy"
    ],
    "user_answers": [
      "local-first persistence"
    ]
  },
  "summary": "Persist the selected wallpaper and restore it on app startup.",
  "acceptance_criteria": [
    "Selected wallpaper persists across restart",
    "Invalid stored wallpaper falls back safely",
    "Current wallpaper is reflected in settings on load"
  ],
  "constraints": [
    "Do not redesign unrelated settings sections",
    "Reuse current config storage"
  ],
  "suggested_profile": "impl-frontend-01",
  "estimated_complexity": 4,
  "risk_level": "medium",
  "handoff_notes": "Touches config read/write path and settings UI."
}
```

### Required Fields Explained

- `type`
  Examples:
  - `feature`
  - `bugfix`
  - `refactor`
  - `infra`
  - `test`
- `area`
  Major product or codebase area.
- `scope_class`
  One of:
  - `required`
  - `best_in_class`
  - `stretch`
- `estimated_complexity`
  Integer 1-5.
- `source_refs`
  Traceability back to planning and user answers.

### Optional Fields

```json
{
  "implementation_hints": [
    "Reuse existing settings save path"
  ],
  "files_likely_touched": [
    "frontend/dist/app.js",
    "main.go"
  ],
  "test_expectations": [
    "go build ./...",
    "manual settings persistence check"
  ]
}
```

## 8. `reports/T-XXXX.attempt-N.json`

This is the implementer completion report.

### Required Schema

```json
{
  "session_id": "cd-20260427-021034-a7k2",
  "ticket_id": "T-0042",
  "attempt": 1,
  "implementer_id": "impl-frontend-01",
  "started_at": "2026-04-27T02:31:18Z",
  "completed_at": "2026-04-27T02:38:02Z",
  "status": "done",
  "summary": "Persisted wallpaper selection in config and restored it at startup.",
  "files_changed": [
    "frontend/dist/app.js",
    "main.go"
  ],
  "commits": [
    "a1b2c3d4"
  ],
  "git_diff_ref": "working-tree",
  "git_diff_stat": " main.go | 12 +++++++++---",
  "tests_run": [
    {
      "name": "go build ./...",
      "result": "pass"
    }
  ],
  "important_decisions": [
    "Reused existing config save mechanism instead of adding a new storage file"
  ],
  "known_risks": [
    "Legacy invalid wallpaper config does not migrate, only falls back"
  ],
  "remaining_work": [],
  "suggested_next_step": "Add wallpaper preview thumbnails in settings."
}
```

### Status Semantics

- `done`
  Acceptance criteria appear complete.
- `partial`
  Some work landed but ticket is incomplete.
- `blocked`
  Implementer hit an external or architectural blocker.
- `failed`
  Attempt was not valid and should likely be retried.

## 9. `review-log.jsonl`

This is append-only.

One JSON object per line.

### Line Schema

```json
{
  "timestamp": "2026-04-27T02:39:14Z",
  "reviewer_id": "cm-03",
  "ticket_id": "T-0042",
  "attempt": 1,
  "decision": "accept",
  "reason": "Acceptance criteria met and no blocking regressions found.",
  "next_action": {
    "type": "dispatch_next_ticket",
    "ticket_id": "T-0043",
    "implementer_id": "impl-backend-01"
  },
  "followup_notes": []
}
```

### Decision Notes

- `accept`
  Ticket becomes `done`.
- `revise`
  Same ticket gets another attempt.
- `split`
  Original ticket is closed and replaced with child tickets.
- `defer`
  Ticket moves to deferred queue.
- `block`
  Ticket moves to blocked queue and may block the session.

## 10. `final-report.md`

This is the human-readable closeout artifact.

### Required Structure

```md
# Final Report

## Session Summary
- Session ID:
- Finalizer:
- Final State:

## Original Goal
- ...

## What Was Completed
- ...

## Tickets Completed
- T-0001
- T-0002

## Deferred Or Blocked Items
- ...

## Validation
- Tests run
- Build checks
- Manual verification notes

## Risks / Residual Gaps
- ...

## Final Signoff
- Approved: yes/no
- Reason:
```

### Ownership Rules

- Only finalizer should author the final report.
- CM may append a short post-finalization note if needed, but should not replace the finalizer report.

## 11. Internal Prompt Inputs

Not every schema needs a file.

Some artifacts should exist only as runtime prompt inputs assembled by the engine.

### Implementer Prompt Input Contract

When dispatching a ticket, the engine should assemble:

```json
{
  "session_id": "cd-20260427-021034-a7k2",
  "agent_id": "impl-frontend-01",
  "role": "implementer",
  "project_charter": {
    "restated_goal": "Build a premium desktop coding assistant.",
    "current_constraints": [
      "Serialized execution",
      "Do not redesign unrelated settings UI"
    ]
  },
  "ticket_file": "tickets/T-0042.json",
  "user_repo_path": "C:/Users/abdeb/Documents/GitHub/CodeDone"
}
```

### CM Review Prompt Input Contract

When reviewing a completed ticket, the engine should assemble:

```json
{
  "session_id": "cd-20260427-021034-a7k2",
  "reviewer_id": "cm-03",
  "directive_file": "directive.md",
  "ticket_file": "tickets/T-0042.json",
  "report_file": "reports/T-0042.attempt-1.json",
  "git_diff_ref": "HEAD~1..HEAD"
}
```

## 12. Minimal First Implementation Set

To keep engineering sane, v1 does not need every optional field.

Implement these first:

- `session.json`
- `directive.md`
- `backlog.json`
- `tickets/*.json`
- `reports/*.json`
- `review-log.jsonl`

Add these immediately after:

- `questions.md`
- `roster.json`
- `final-report.md`

## Final Rule

If two artifacts disagree:

1. `session.json` wins for runtime session state
2. `ticket.json` wins for ticket truth
3. `review-log.jsonl` wins for historical decisions
4. `directive.md` wins for planning narrative and intent

That separation keeps the engine deterministic without losing the CM planning layer.
