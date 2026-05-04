# CodeDone.app

Local, privacy-first agentic coding workspace in Go. CodeDone breaks large development tasks into atomic serialized tickets, dispatches AI implementers one step at a time, and uses a Contre-Maître system to plan, supervise, review, and finalize work with lower token waste and higher reliability. Built to excel at very long tasks spanning multiple days.

<p align="center">
  <img src="https://codedone.app/cdn/codedone-preview-1.png?v=2" alt="CodeDone overview" width="900">
</p>

## What is CodeDone?

CodeDone is an agentic coding workspace designed for large, long-running software tasks. Instead of launching multiple chaotic agents in parallel, CodeDone uses a serialized agent pipeline: the **Contre-Maître** plans the work, generates a structured backlog, dispatches one ticket at a time to an Implementer, reviews each result, and finally hands the completed session to a Finalizer.

The goal is simple: let users build, refactor, or modify large projects from a single prompt while keeping the process auditable, controlled, and efficient.

## Key Features

### Serialized agent pipeline

CodeDone uses a structured multi-agent workflow:

- **Contre-Maître** plans, supervises, reviews, and controls the execution.
- **Implementers** build one ticket at a time.
- **Finalizer** validates the full session before marking the work complete.

No parallel chaos. No uncontrolled overlap. Each step is serialized, reviewable, and traceable.

### Native Git workflow

CodeDone is built directly around Git. Agents communicate through actual repository state, diffs, and file changes instead of vague status messages.

This makes every step auditable:

- inspect what changed
- review ticket-by-ticket progress
- keep long sessions grounded in real code
- work directly with GitHub-backed repositories

### Multi-provider LLM support

Use the models you want, where you want them.

Supported providers include:

- **OpenAI**
- **Anthropic**
- **DeepSeek**
- **OpenRouter**
- local LLMs, depending on your provider setup

You can configure different models for different agent roles, for example one model for the Contre-Maître and another for Implementers.

### Configurable agent setup

Tune the execution strategy for your project:

- choose how many Contre-Maîtres to spawn
- choose how many Implementers to use
- assign different models per role
- control long-running sessions from a saved workspace state

### Plan Mode

Before building, CodeDone can run in a planning/refinement mode where you discuss the task directly with the Contre-Maître.

Use it to clarify scope, refine requirements, validate assumptions, and shape the backlog before any implementation starts.

<p align="center">
  <img src="https://codedone.app/cdn/codedone-preview-2.png?v=2" alt="CodeDone planning mode and backlog generation" width="900">
</p>

### Guidance system

CodeDone includes built-in LLM guidance for different task types and project domains.

Guidance can help agents behave correctly for work such as:

- web design
- frontend development
- Go
- C / C++ / C#
- databases
- shell and DevOps
- cybersecurity review
- game development
- project-specific coding standards

Guidance acts as persistent task-aware instructions that agents can query during a session.

### Repo inspection and build tools

Agents can inspect and operate on the target repository using tools such as:

- file read
- glob
- grep
- git introspection
- shell execution
- test runner integration

This lets agents understand the codebase before acting, apply targeted changes, and validate the result.

### Long-running session support

CodeDone is designed for tasks that are too large for a single prompt-response cycle.

Sessions can be:

- paused
- resumed
- continued from a saved workspace
- run across long workflows
- used for multi-day implementation tasks

<p align="center">
  <img src="https://codedone.app/cdn/codedone-duringtask.png?v=2" alt="CodeDone during task execution with backlog tickets and live agent editing" width="900">
</p>

### Desktop-first experience

CodeDone is built as a desktop-first application using **Go + Wails**, with a focused CLI-like workflow and support for both **dark** and **light** themes.

<p align="center">
  <img src="https://codedone.app/cdn/codedone-preview-3.png?v=2" alt="CodeDone interface with theme and product UI" width="900">
</p>

## Why CodeDone?

Most agentic coding tools are optimized for short tasks. CodeDone is designed for long, structured, high-context development work.

It focuses on:

- privacy-first local execution
- memory safety through Go
- atomic ticket serialization
- lower token waste
- higher review quality
- direct Git-based auditability
- configurable model/provider strategy
- long-running autonomous project work

CodeDone is for developers who want agents that can work on serious projects without turning the repository into an uncontrolled multi-agent mess.

<p align="center">
  <img src="https://codedone.app/cdn/codedone-taskcomplete.png?v=2" alt="CodeDone completed task with finalizer validation" width="900">
</p>

## Build

CodeDone is a Wails desktop app built with Go and a web frontend.

Install the required tooling first:

- Go
- Node.js / npm
- Wails CLI

Check your local setup:

```bash
wails doctor
```

Install frontend dependencies if needed:

```bash
npm install
```

Build the app for your current operating system:

```bash
wails build
```

The compiled app will be generated in:

```text
build/bin
```

For development mode with live reload:

```bash
wails dev
```

To build for another platform, use Wails cross-compilation support:

```bash
wails build -platform windows/amd64
wails build -platform darwin/amd64
wails build -platform darwin/arm64
wails build -platform linux/amd64
```

Cross-compilation can require platform-specific dependencies, so for release builds it is usually safest to build directly on the target operating system.
