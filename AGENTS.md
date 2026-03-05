# Repository Guidelines

## Project Structure & Module Organization
- `cmd/bmux`: CLI entrypoint and Cobra command wiring.
- `internal/app`: core orchestration and task lifecycle logic.
- `internal/tui`: Bubble Tea model/update/view code for the terminal UI.
- `internal/{beads,gitx,tmux,branching,state,config,pr}`: external integrations and domain services.
- `docs/`: product notes and requirements (see `docs/prod.md`).
- `scripts/build.sh`: reproducible debug/release build script.

Runtime artifacts live in `.bmux/` and `.worktrees/` and should remain uncommitted.

## Compatibility Policy
- This repository is pre-release and does **not** require backward compatibility.
- Prefer simpler, clean designs over compatibility shims, aliases, or migration layers.
- When renaming fields/APIs/config keys internally, update all call sites and tests directly.

## Build, Test, and Development Commands
- `go test ./...`: run the full unit test suite.
- `go test -cover ./...`: run tests with coverage output.
- `go build ./cmd/bmux`: compile the CLI quickly during development.
- `go run ./cmd/bmux`: run the app locally without installing.
- `./scripts/build.sh --debug`: build `bin/bmux-debug` with debug flags.
- `./scripts/build.sh --release`: build optimized `bin/bmux` for distribution.
- `./scripts/format.sh`: format all source files (Go and shell when `shfmt` is available).

## Coding Style & Naming Conventions
- Follow idiomatic Go and keep files `gofmt`-formatted.
- Use `./scripts/format.sh` as the standard formatting entrypoint before submitting changes.
- Use default Go formatting (tabs, standard import grouping); do not hand-format alignment.
- Keep package names lowercase and focused (examples: `gitx`, `tmux`, `errorsx`).
- Use descriptive lowercase filenames (examples: `service.go`, `planner_test.go`).
- Exported identifiers use `CamelCase`; internal helpers use `camelCase`.
- Branch naming follows `task/<issue-id>-<slug>` (example: `task/bd-123-auth-flow`).

## Testing Guidelines
- Place tests beside implementation files as `*_test.go`.
- Name tests with `TestXxx` and prefer table-driven cases for branching/decision logic.
- Use `t.Parallel()` for independent tests where safe.
- Cover both happy paths and failure modes for command wrappers and integrations.

## Commit & Pull Request Guidelines
- All commit messages must follow Conventional Commits (`type(scope): subject` or `type: subject`).
- Common types include: `feat`, `fix`, `refactor`, `chore`, `docs`, `test`.
- Keep commits scoped to one concern or package group.
- PRs should include: summary of behavior changes, linked task/issue ID (for example `bd-123`), and test evidence (`go test ./...`).
- For visible TUI changes, attach a short terminal screenshot or capture.
