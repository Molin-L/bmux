# bmux

`bmux` is a Beads-aware git worktree orchestrator built with Go and Bubble Tea.

It maps one Beads task to one branch/worktree by default, and can reuse an existing branch for subtasks/dependencies using deterministic precedence.

## Features

- Bubble Tea TUI entrypoint (`bmux`)
- Deterministic branch naming: `task/<issue-id>-<slug>`
- Configurable worktree directory (default: `<project_root>/.worktrees`)
- Branch creation from current HEAD commit with captured base branch/commit metadata
- Dependency-based branch reuse with precedence:
  - `parent-child`
  - `blocks`
  - `discovered-from`
  - `related`
- PR/MR delegation prompt generation (`bmux task pr <issue-id>`)
- Explicit merge and cleanup commands

## Requirements

- Go 1.26+
- `git`
- `bd` (Beads CLI)

## Build

```bash
go build ./cmd/bmux
```

## Commands

```bash
bmux
bmux doctor
bmux config show
bmux task open <issue-id>
bmux task pr <issue-id>
bmux task merge <issue-id>
bmux task cleanup <issue-id>
```

## Configuration

Config files are loaded in this order:

1. `~/.bmux/config.yaml`
2. `.bmux/config.yaml` (project override)

Supported keys:

```yaml
worktree_dir: /absolute/or/project-relative/path
branch_prefix: task/
entrypoint: tui
subtask_share_mode: dependency
agents:
  claude:
    command: claude
  codex:
    command: codex
tmux:
  auto_attach: true
  layout: sidebar
  control_pane_width: 40
  min_pane_width: 50
  max_pane_width: 80
  session_prefix: bmux-
  split_direction: right
planning:
  prompt_template: ""
```

Defaults:

- `worktree_dir`: `<project_root>/.worktrees`
- `branch_prefix`: `task/`
- `entrypoint`: `tui`
- `subtask_share_mode`: `dependency`
- `tmux.split_direction`: `right` (`right` or `below`, used when `tmux.layout=single`; sidebar layout is layout-manager controlled)
- `tmux.auto_attach`: `true` (auto-create/attach project tmux session when launched outside tmux)
- `tmux.layout`: `sidebar` (`sidebar` or `single`)
- `tmux.control_pane_width`: `40`
- `tmux.min_pane_width`: `50` (clamped to `40..300`)
- `tmux.max_pane_width`: `80` (clamped to `40..300`, normalized so `max >= min`)
- `tmux.session_prefix`: `bmux-`
- `agents.claude.command`: empty by default; if empty, `bmux` falls back to `claude` from PATH
- `agents.codex.command`: empty by default; if empty, `bmux` falls back to `codex` from PATH
- `planning.prompt_template`: built-in template when empty

## Fixture

`examples/beads-sample` is a deterministic Beads fixture repository for manual and scripted `bmux` testing.

Reset fixture state with one command:

```bash
cd examples/beads-sample
./reset.sh
```

`reset.sh` is destructive and always rebuilds from scratch. It wipes local `.beads/`, `.bmux/`, and `.worktrees/`, reinitializes Beads with prefix `bd`, reseeds fixed IDs (`bd-100` to `bd-105`) and dependencies, then recreates a clean baseline git commit.

Fixture seed/schema updates are intentionally non-backward-compatible.

## Notes

- `bmux` targets current Beads CLI behavior (`bd 0.58.x` baseline).
- `bd sync` is not used; metadata operations rely on `bd` command primitives and Dolt flows.
- Runtime state is stored in `.bmux/state.json`.
