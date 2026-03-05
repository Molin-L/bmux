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
```

Defaults:

- `worktree_dir`: `<project_root>/.worktrees`
- `branch_prefix`: `task/`
- `entrypoint`: `tui`
- `subtask_share_mode`: `dependency`

## Notes

- `bmux` targets current Beads CLI behavior (`bd 0.58.x` baseline).
- `bd sync` is not used; metadata operations rely on `bd` command primitives and Dolt flows.
- Runtime state is stored in `.bmux/state.json`.
