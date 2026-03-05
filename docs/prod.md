# bmux Product Requirements Document (PRD)

## 1. Document Info

- Product: `bmux`
- Version: `v0.1 (draft)`
- Updated: `2026-03-05`
- Authoring basis: direct reference to `/Users/molin/Code/dmux`
- Beads CLI baseline validated against local: `bd version 0.58.0`

## 2. Assumptions For This Draft

Because `bmux` currently has no code and an empty product doc, this PRD assumes:

1. `bmux` is a `dmux`-inspired CLI/TUI for parallel AI coding in isolated git worktrees.
2. `bmux` should keep core `dmux` strengths (parallel panes, safe merge flow, hooks, multi-project support).
3. `bmux` should add an issue-first workflow via Beads (`.beads` is already initialized in this repo).
4. `bmux` targets the current Beads command model: `bd ready/create/update/show/close/dep/...` plus `bd dolt commit|pull|push`.
5. `bd sync` is not available in the local baseline (`bd 0.58.0`) and is not a required integration target.

If assumption #3 is not desired, the Beads sections can be removed and `bmux` can remain strict `dmux` parity.

## 3. Product Vision

`bmux` helps developers run many AI coding tasks in parallel without branch sprawl by combining:

- tmux pane orchestration
- git worktree isolation
- agent automation
- issue-aware execution (via Beads)

Primary promise: "Move multiple coding tasks from idea to merged code with minimal context switching and low merge risk."

## 4. User Segments

1. Solo AI-assisted developer
- Needs fast parallel experimentation.
- Wants low-ceremony branch/worktree management.

2. Tech lead managing multiple streams
- Needs visibility across many active agent tasks.
- Wants predictable merge and cleanup behavior.

3. Agent-heavy OSS maintainer
- Needs reproducible automation hooks and configurable safety.
- Wants issue/task traceability from prompt to merge.

## 5. Problems To Solve

1. Parallel agent sessions create branch/worktree sprawl.
2. Manual task handoff between issue tracker, terminal, and git is slow.
3. Merge conflicts often surface too late (on main branch) and block flow.
4. Running hooks/scripts per task is repetitive and inconsistent.
5. Multi-repo and nested-repo workflows are hard to coordinate.

## 6. Goals And Non-Goals

### Goals

1. Launch parallel agent tasks in isolated worktrees from a single keyboard-first UI.
2. Make merge safer with a two-phase merge process.
3. Provide strong extensibility through lifecycle hooks.
4. Offer issue-centric workflows (create pane from issue, update issue lifecycle on merge, and publish Beads changes).
5. Support multi-project and nested-worktree merge orchestration.

### Non-Goals (v1)

1. Hosted web platform or cloud orchestration.
2. Replacing git, tmux, or Beads with a proprietary alternative.
3. Full enterprise permission/policy engine.
4. Rich IDE plugin ecosystem in v1.

## 7. Core User Flows

### Flow A: Create Task Pane From Prompt

1. User presses `n`.
2. User enters prompt and selects agent(s).
3. `bmux` creates branch + worktree, launches pane.
4. Sidebar tracks pane status (`working`, `waiting`, `idle`).
5. User merges via pane menu (`m`) when done.

### Flow B: Create Task Pane From Beads Issue

1. User selects issue (by ID/search) from integrated issue picker.
2. `bmux` composes initial prompt from issue title/body/context.
3. Branch slug includes issue identifier (example: `bd-123-fix-auth`).
4. On merge success, issue state can auto-transition (configurable).

### Flow C: Compare Agents (A/B)

1. User creates one prompt with two-agent pair.
2. `bmux` creates two independent worktrees.
3. User compares outputs side by side.
4. User merges preferred branch and closes loser pane.

### Flow D: Multi-Project Merge

1. User attaches additional project(s).
2. `bmux` detects nested worktree relationships.
3. Merge queue runs deepest-first to avoid stale parent references.

## 8. Functional Requirements

### 8.1 Session And Project Management

- FR-001: Running `bmux` in a git repo creates/attaches a tmux session scoped to the project.
- FR-002: Project metadata persists under `.bmux/`.
- FR-003: User can attach multiple projects to one session.
- FR-004: UI groups panes by project and supports keyboard navigation across groups.

### 8.2 Pane And Worktree Lifecycle

- FR-010: Each pane maps to one task slug and one git worktree.
- FR-011: Each worktree maps to one branch (branch may differ from slug when prefix is used).
- FR-012: Pane creation supports agent pane and terminal-only pane.
- FR-013: Closing pane supports cleanup options (pane only vs pane + worktree + branch).
- FR-014: Reopen closed worktrees is supported.

### 8.3 Agent Management

Baseline parity target from `dmux`:

- Auto-detect installed agent CLIs via PATH and common install directories.
- Default enabled agents: Claude Code, Codex, OpenCode.
- Optional agents: Cline CLI, Gemini CLI, Qwen CLI, Amp CLI, pi CLI, Cursor CLI, Copilot CLI, Crush CLI.

Requirements:

- FR-020: Agent selector supports single and A/B pair choices.
- FR-021: Configurable `defaultAgent` bypasses selector when available.
- FR-022: Permission mode maps to agent-specific flags (`plan`, `acceptEdits`, `bypassPermissions`, empty).
- FR-023: Autopilot can auto-accept safe option dialogs when enabled.

### 8.4 Status Detection And Interaction

- FR-030: Pane status updates at ~1s intervals without blocking UI.
- FR-031: Detect status states: `in_progress`, `open_prompt`, `option_dialog`, `idle`.
- FR-032: Support queued prompt sending to active panes.
- FR-033: Provide explicit warning when risk is detected during autopilot decision flow.

### 8.5 Merge System

- FR-040: Merge is two-phase:
1. `main -> worktree` (conflicts resolved in isolation)
2. `worktree -> main`

- FR-041: Auto-commit uncommitted worktree changes before merge.
- FR-042: AI-generated commit messages when LLM provider is configured; deterministic fallback if not.
- FR-043: On conflict in phase 1, abort merge and expose options:
1. manual resolution
2. AI-assisted resolution
3. cancel

- FR-044: Post-merge cleanup removes worktree and safely deletes branch.
- FR-045: Multi-worktree merge supports nested repositories and deepest-first ordering.

### 8.6 Hooks And Automation

`bmux` should keep `dmux`-style hook coverage (11 hooks):

1. `before_pane_create`
2. `pane_created`
3. `worktree_created`
4. `before_pane_close`
5. `pane_closed`
6. `before_worktree_remove`
7. `worktree_removed`
8. `pre_merge`
9. `post_merge`
10. `run_test`
11. `run_dev`

Requirements:

- FR-050: Hook discovery priority:
1. `.bmux-hooks/` (project root)
2. `.bmux/hooks/` (project data)
3. `~/.bmux/hooks/` (global)

- FR-051: Hooks receive structured env context (pane, worktree, branch, project).
- FR-052: Non-zero exit in blocking hooks aborts operation with clear feedback.
- FR-053: Hook authoring assistant flow exists (AI-assisted creation/editing).

### 8.7 Beads Integration (bmux Differentiator)

- FR-060: `bmux` issue selection is backed by `bd ready --json` and filtered `bd list --json`.
- FR-061: Creating a pane from issue ID must fetch canonical issue details with `bd show <id> --json`.
- FR-062: Prompt composition from issue must include title, description, status, priority, and dependencies when present.
- FR-063: Branch slug format supports issue linkage (e.g. `bd-<id>-<slug>`).
- FR-064: On merge success, configurable issue lifecycle actions:
1. transition issue state via `bd update` or `bd close`
2. commit/publish issue data via `bd dolt commit`, `bd dolt pull`, and `bd dolt push` when configured
- FR-065: If Beads CLI is missing, incompatible, or command execution fails, `bmux` degrades to prompt-only mode and surfaces actionable guidance.

### 8.8 Settings And Configuration

Adopt layered settings model from `dmux`:

- Global defaults file
- Project overrides file
- Runtime pane tracking file

Requirements:

- FR-070: Project settings override global settings.
- FR-071: Supported keys include:
1. `tmux.control_pane_width`
2. `tmux.min_pane_width`
3. `tmux.max_pane_width`
4. `tmux.layout`
5. `tmux.split_direction`

- FR-071A: In `tmux.layout=sidebar`, pane placement is layout-manager controlled (dmux-style grid fitting with optional spacer pane).
- FR-071B: `tmux.split_direction` remains effective for `tmux.layout=single`.

- FR-072: Validate `baseBranch` and `branchPrefix` as safe git branch names.
- FR-073: Validate pane width ranges and normalize `min <= max`.

### 8.9 Keyboard-First UX

- FR-080: Single-key shortcuts for high-frequency actions (`n`, `t`, `j`, `m`, `x`, `p`, `s`, `q`, `?`).
- FR-081: Contextual action menu per pane (merge, close, rename, add agent, add terminal, copy path, toggle autopilot).
- FR-082: Fast prompt input with multiline support and paste safety.

### 8.10 Beads CLI Integration Baseline

Canonical `bd` commands to invoke from `bmux`:

1. `bd ready --json`
2. `bd list --json` (with filters)
3. `bd show <id> --json`
4. `bd create "..." ... --json`
5. `bd update <id> --claim --json`
6. `bd close <id> --reason "..." --json`
7. `bd dep add ...` or `bd create ... --deps discovered-from:<id> --json`
8. `bd dolt commit`
9. `bd dolt pull`
10. `bd dolt push`

Invocation and safety rules:

1. Machine integrations must parse only `--json` command output for issue operations.
2. Integration paths must not use interactive commands (`bd edit`, interactive forms).
3. User-provided strings passed to shell commands must be quoted/sanitized safely.
4. Command execution must enforce strict timeout and non-zero exit handling.

Error-handling contract:

1. On unavailable command, incompatible CLI surface, timeout, or non-zero exit:
2. `bmux` logs the specific command failure
3. `bmux` falls back to prompt-only pane flow
4. `bmux` shows actionable remediation (install/check `bd`, run `bd --help`, run `bd dolt --help`, verify `bd version`)

PRD-level adapter contract (conceptual):

1. `listReadyIssues()`
2. `getIssue(id)`
3. `claimIssue(id)`
4. `createIssue(payload)`
5. `closeIssue(id, reason)`
6. `linkDiscovered(childId, parentId)`
7. `flushIssueVCS()` mapped to `bd dolt commit|pull|push`

## 9. Non-Functional Requirements

1. Performance
- NFR-001: pane creation p50 < 2.5s (excluding agent startup time).
- NFR-002: sidebar refresh and input latency should feel real-time (<100ms UI response).

2. Reliability
- NFR-010: failed merge phase must not corrupt main branch state.
- NFR-011: cleanup should be idempotent on retries.

3. Security/Safety
- NFR-020: explicit warning for dangerous permission modes.
- NFR-021: shell command construction must validate untrusted inputs (`branchPrefix`, `baseBranch`, issue IDs).

4. Compatibility
- NFR-030: tmux `>=3.0`, Node.js `>=18`, Git `>=2.20`.
- NFR-031: macOS/Linux first-class; Windows via WSL is best-effort in v1.

## 10. Data Model (High Level)

1. Project
- repo path, project name, settings scope, attached status

2. Pane
- pane id, tmux pane id, prompt, agent, status, autopilot flag, worktree path, branch name, timestamps

3. Worktree
- root repo path, branch, nested depth, parent relationship, dirty state

4. IssueLink (bmux extension)
- issue id, source tracker (`beads`), pane id(s), branch name, merge commit hash, sync state

## 11. Success Metrics

1. Activation
- % of users creating first pane within 5 minutes of install.

2. Throughput
- merged panes per active day.
- median time from pane creation to merge.

3. Quality/Safety
- merge failure rate.
- conflict resolution success rate (manual + AI-assisted).

4. Adoption of differentiators
- % panes created from Beads issues.
- % merges syncing issue state successfully.

## 12. Rollout Plan

### Phase 1: dmux Parity Core

- Session/pane/worktree lifecycle
- Agent launch + selector + autopilot baseline
- Two-phase merge + cleanup
- Keyboard shortcuts + settings

### Phase 2: Hooks + Multi-Project Hardening

- Full 11-hook lifecycle
- Nested worktree discovery and multi-merge orchestration
- Stability and recovery improvements

### Phase 3: Beads-Native Workflow

- Issue picker + issue-to-pane creation
- Merge-to-issue lifecycle updates + `bd dolt commit|pull|push` publish flow
- Issue-aware branch naming and summaries

## 13. Risks And Mitigations

1. Risk: unsafe autonomous behavior under permissive flags.
- Mitigation: strong warnings, safer defaults option, per-pane autopilot toggle.

2. Risk: shell injection through settings or issue-derived branch names.
- Mitigation: strict branch/input validation and escaping.

3. Risk: merge edge cases in nested repos.
- Mitigation: deterministic merge queue, dry-run validation, robust rollback paths.

4. Risk: issue sync failures create inconsistent state.
- Mitigation: best-effort sync with retry queue and explicit failure visibility.

5. Risk: version drift between Beads docs/examples and installed CLI command surface.
- Mitigation: runtime capability detection with `bd --help`, `bd dolt --help`, and `bd version`; feature flags for optional behaviors; never hardcode deprecated flows like `bd sync`.

## 14. Open Questions

1. Should `bmux` default to safer permission mode than `dmux` (`bypassPermissions`)?
2. Which Beads state transitions should be default on pane create/merge?
3. Should issue sync be opt-in globally, per project, or per pane?
4. Is a lightweight dashboard/API needed in v1, or strictly terminal-first?

## 15. dmux Reference Mapping

The following areas are directly informed by `/Users/molin/Code/dmux` docs and source:

1. Worktree-centric pane model
2. Two-phase merge strategy
3. 11-hook lifecycle model
4. Layered settings with validation
5. Multi-project and nested worktree merge orchestration
6. Multi-agent selection and A/B workflows
7. Keyboard-first control surface

`bmux`-specific proposal in this PRD:

1. Native Beads issue integration as a first-class task source and merge sync target.

## 16. Beads Integration Validation Scenarios

1. Happy path
- Given ready work exists, `bmux` claims issue via `bd update <id> --claim --json`, work completes, issue closes via `bd close`, and configured `bd dolt commit/push` succeeds.

2. Discovered work linkage
- While working a parent issue, `bmux` creates new issue with `--deps discovered-from:<parent-id>` (or equivalent `bd dep add`) and confirms link appears in issue graph.

3. Command mismatch resilience
- In an environment where `bd sync` is absent, `bmux` still completes lifecycle using `bd dolt commit|pull|push` without blocking core workflows.

4. Missing Beads CLI fallback
- If `bd` is not installed or command execution fails, `bmux` warns clearly and continues with prompt-only pane flow.

5. Non-interactive safety
- Integration test asserts `bmux` never invokes interactive-only commands (`bd edit`, interactive form commands) in automation paths.

6. JSON contract enforcement
- Integration test asserts all parsed issue command outputs come from `--json` invocations; non-JSON output is treated as error.
