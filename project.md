# CodeDone

Open source agentic coding tool — serialized, not parallelized.

## Concept

Most agentic tools throw tasks at agents in parallel. CodeDone does the opposite: agents work in strict serialized sequence, one after another, each building on verified prior work. This makes execution predictable, reviewable, and correct by construction.

## Agent Architecture

### Contre-Maître (1–2 agents)
The orchestrating agents. They never implement — they direct and verify.

- **Phase 1 — Search:** Intensive upfront research on what needs to be done. Full codebase understanding, requirements decomposition, dependency mapping.
- **Phase 2 — Oversight:** Constantly monitor an internal git diff. After each feature lands, they review the diff, assess correctness, and issue the next instruction.
- **Phase 3 — Loop:** Continue until all features are implemented and verified.

### Implementer Agent
Receives one feature at a time from Contre-Maître. Implements it, commits to the internal git. Does not move on — waits for the next instruction.

### Finalizer Agent
Activated at the end of the implementation loop. Runs tests, validates the full build, confirms everything works end-to-end, and signs off on the deliverable.

## Internal Git
The internal git is the shared ground truth. It is the communication layer between Contre-Maître and the Implementer — diffs replace status updates, commits replace reports.

## Tech Stack

| Layer | Technology |
|---|---|
| Backend / CLI core | Go (Golang) |
| Desktop shell | Wails |
| UI paradigm | CLI-like interface (think Claude Code / Qwen Code / Codex CLI — but significantly better) |

## Design & Frontend

UI inspiration comes from [x22-raycast](C:\Users\abdeb\Documents\GitHub\x22-raycast) — a custom Raycast-like launcher for Windows. Borrow its:
- Visual language and motion design
- Animation system
- Layout and interaction patterns

The aesthetic target: a premium CLI-native tool that feels as designed as a consumer app.

## Positioning

CodeDone sits in the same category as Claude Code, Qwen Code, OpenAI Codex CLI — but differentiates on:
- Serialized agent execution (predictable, auditable)
- Contre-Maître oversight loop (self-correcting without human babysitting)
- Desktop-first (Wails, not browser or raw terminal)
- Design quality that matches or exceeds the best consumer software

---

## 2026-04-26

### API Layer

**Dev model:** Deepseek — cheap, agentic-tuned, fits the use case.

**What the API layer must cover:**
- OpenAI-compatible adapter as default (covers Deepseek, Qwen, OpenRouter, most providers)
- Anthropic adapter (separate SDK, not OAI-compatible)
- Provider interface in Go so any new provider slots in without touching agent logic
- Model selection per-session (user picks provider + model)
- Local model support deferred — designed in from the start, implemented later

### Build Plan

1. Wails scaffold + frontend UI (x22-raycast inspired, mocked agent output)
2. Backend agent loop (Contre-Maître cycle, git branch management, Implementer, Finalizer)
3. Wire frontend ↔ backend via Wails bindings
4. Replace mocks with real Deepseek API calls

---

## 2026-04-27

### Contre-MaÃ®tre Directive Pipeline

The serialized model needs a stronger planning and directive layer before implementation begins.

#### CM1 â€” Initial Research + Goal Framing

The first Contre-MaÃ®tre should:

- Perform the initial research pass on the user request
- Frame the goal properly instead of taking the first prompt at face value
- Ask the user clarifying questions when needed, especially when preferences, scope, tradeoffs, or product direction are unclear
- Push beyond the user's initial wording and actively raise the quality bar
- If the user did not specify stack, implementation direction, or product standards, research what other strong products in the space do
- Search online and gather external reference points when needed
- Produce a serious top-tier feature expectation, not a minimal interpretation

The standard is not "good enough to work." The standard is "what would a top-of-the-top implementation require if a very strong engineer or research-heavy product team were defining it properly."

Example:
If a user asks for something as large as a web-browser OS, the first Contre-MaÃ®tre must not reduce that to a toy prototype. It should expand the expected surface area and identify the full product reality: CLI, wallpaper, search, applications, games, saves, restore, internet, and all other major expected capabilities and system-level concerns.

#### CM Directive

After the first research and framing pass, the Contre-MaÃ®tre should commit its planning output into an internal directive artifact such as:

- `CM-DIRECTIVE.md`

This directive is the planning backbone for the rest of the orchestration layer.

It should contain:

- Refined goal definition
- User answers and locked decisions
- Assumptions made when the user did not specify enough
- Competitive / reference findings
- Proposed stack direction when not user-specified
- Full feature inventory
- Missing expectations the user did not mention explicitly
- Quality bar and acceptance standard
- Risks, unknowns, and follow-up research areas

This directive is intended to be visible to Contre-MaÃ®tre agents, not to Implementer agents directly as raw planning context.

#### Multiple Contre-MaÃ®tres in Series

If the user configured more than one Contre-MaÃ®tre, they should work strictly in series.

The pipeline should be:

1. CM1 writes the initial directive
2. CM2 reads the directive first, critiques it, improves it, adds what is missing, sharpens the scope, and updates it
3. CM3 does the same
4. Continue until the final Contre-MaÃ®tre

Each later Contre-MaÃ®tre is not starting from scratch. Its first responsibility is to read the prior directive, detect weaknesses, add missing depth, improve quality, and leave behind a stronger version for the next one.

This means the directive becomes a progressively improved internal planning document across the full serialized Contre-MaÃ®tre chain.

#### Final Contre-MaÃ®tre Responsibilities

The last Contre-MaÃ®tre in the series should:

- Perform the same critique / improvement pass as the others
- Finalize the directive
- Decide how many Implementer agents are needed, bounded by the user's configured maximum
- Decide how the implementation work should be partitioned
- Dispatch the implementation phase from the finalized directive

The Implementer count should not be arbitrary. It should be chosen based on actual scope, complexity, and the number of distinct work packets required.

#### Internal Git As Planning Ground Truth

Internal git is not only for implementation diffs. It should also act as the planning memory for the orchestration layer.

The Contre-MaÃ®tre chain should be able to use git-tracked planning artifacts as internal ground truth, with the directive evolving over time as each Contre-MaÃ®tre improves it.

This creates a durable serialized planning trail:

- Initial user intent
- Refined directive
- Iterative Contre-MaÃ®tre improvements
- Final implementation dispatch basis

#### Open Design Question: Implementer Scope Control

A remaining design question is how tightly to scope each Implementer.

Possible approaches under consideration:

- One directive file per Implementer agent, where each agent can read only its own implementation brief
- System-instruction scoping instead of file scoping, so the agent is behaviorally constrained by its assigned role and feature packet
- A combination of both, where git-stored directive files are durable planning state and system instructions enforce strict focus

The goal is for Implementers to stay fully focused on their own assigned work packet instead of drifting into broader planning or re-architecting behavior that belongs to the Contre-MaÃ®tre layer.
### CM1 User Question Gate

Before CM1 performs deep research or planning, it should have the option to ask the user clarifying questions first.

Rules:

- Questions, if any, must happen as the first step
- CM1 decides whether questions are needed
- Maximum of 5 questions
- If the user request is already clear enough, CM1 should ask nothing and proceed immediately
- The goal is to lock important product or implementation preferences while the user is still actively present

This preserves a strong early framing step without forcing unnecessary user interaction on every request.

### User-Facing Execution State

The system should expose a visible execution state in the UI so the user understands whether the orchestration layer is still allowed to ask questions or whether it is operating fully autonomously.

Example concepts:

- Question / framing mode
- Planning mode
- Implementation mode
- Review mode
- Finalization mode
- Fully autonomous mode

The exact naming can change later, but the important idea is that the user should understand the current orchestration mode instead of being surprised by late clarifying questions or hidden agent behavior changes.

### Implementer Dispatch And Task Queue

The major open problem is not just what an Implementer does, but how work is dispatched across a large backlog.

Example:

- User max agents: 20
- Actual implementation backlog: 500 tasks

The final Contre-MaÃƒÂ®tre must therefore do more than choose an Implementer count. It must also manage task dispatch across a much larger work queue.

The intended behavior is:

1. The final Contre-MaÃƒÂ®tre creates or finalizes the full task list
2. It decides how many Implementer agents should exist, bounded by the configured max
3. It dispatches implementation work internally
4. When an Implementer finishes, it leaves behind a completion note / checknote describing what it did
5. The Contre-MaÃƒÂ®tre reads that note plus the git diff / commit result
6. The Contre-MaÃƒÂ®tre then immediately decides the next step and dispatches the next task
7. The loop continues until the full task list is exhausted

This means task completion must not be a silent event. Every Implementer should emit a structured hand-off artifact that the Contre-MaÃƒÂ®tre can inspect before deciding what happens next.

### Implementer Completion Note

Each Implementer should report a concise completion note after finishing a task.

That note should include:

- What was implemented
- Files changed
- Important decisions made
- Risks or follow-up concerns
- Anything left incomplete
- Suggested next step if relevant

The completion note acts as a compact review hand-off so the Contre-MaÃƒÂ®tre can rapidly assess whether to accept the work, request revision, or issue the next task.
