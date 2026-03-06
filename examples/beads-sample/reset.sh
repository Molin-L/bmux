#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT_DIR}"

BASELINE_MESSAGE="chore(fixtures): reset beads-sample baseline"

require_cmd() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "error: required command not found: ${cmd}" >&2
    exit 1
  fi
}

ensure_git_repo() {
  if [[ ! -d .git ]]; then
    git init -q -b main
  fi
}

ensure_git_identity() {
  if ! git config --local user.name >/dev/null 2>&1; then
    git config --local user.name "bmux fixture"
  fi
  if ! git config --local user.email >/dev/null 2>&1; then
    git config --local user.email "fixture@local"
  fi
}

write_baseline_files() {
  cat > README.md <<'MARKDOWN'
# beads-sample

Deterministic fixture repository used by bmux testing.

Use `./reset.sh` to rebuild all fixture state.
MARKDOWN

  cat > main.go <<'GO'
package main

import "fmt"

func main() {
	fmt.Println("beads-sample fixture")
}
GO

  cat > .gitignore <<'IGNORE'
.beads/
.bmux/
.worktrees/
.beads/dolt-server*.log
.beads/dolt-server*.pid
.beads/dolt-server*.port
.beads/dolt-server*.activity
.beads/dolt-server*.lock
.dolt/
*.db
AGENTS.md
IGNORE
}

seed_beads() {
  if ! bd init --prefix bd --force --quiet >/dev/null 2>&1; then
    bd dolt killall >/dev/null 2>&1 || true
    bd init --prefix bd --force --quiet
  fi

  # Ensure deterministic seed IDs even if the backing server reused prior state.
  local seed_ids=(bd-100 bd-101 bd-102 bd-103 bd-104 bd-105)
  local seed_id
  for seed_id in "${seed_ids[@]}"; do
    bd delete "${seed_id}" --force --quiet >/dev/null 2>&1 || true
  done

  bd create --id bd-100 --type epic --title "Core workflow epic" --description "Root epic for bmux fixture flows." --priority 1 --quiet
  bd create --id bd-101 --type task --title "Implement auth middleware" --description "Ready task for standard claim flow." --priority 2 --quiet
  bd create --id bd-102 --type task --title "Build planner integration" --description "Represents an in-progress claimed task." --priority 2 --quiet
  bd create --id bd-103 --type task --title "Add integration tests" --description "Blocked task waiting on bd-101." --priority 2 --quiet
  bd create --id bd-104 --type task --title "Write operator runbook" --description "Closed task for filtering scenarios." --priority 3 --quiet
  bd create --id bd-105 --type task --title "Automate release checklist" --description "Second-level blocked task for chain checks." --priority 2 --quiet

  bd update bd-101 --parent bd-100 --quiet
  bd update bd-102 --parent bd-100 --quiet
  bd update bd-103 --parent bd-100 --quiet
  bd update bd-104 --parent bd-100 --quiet
  bd update bd-105 --parent bd-100 --quiet

  bd dep add bd-103 --blocked-by bd-101 --type blocks --quiet
  bd dep add bd-105 --blocked-by bd-103 --type blocks --quiet

  bd update bd-102 --status in_progress --quiet
  bd close bd-104 --reason "seed baseline closed item" --quiet
}

rebuild_git_baseline() {
  ensure_git_identity

  git checkout --orphan fixture-baseline >/dev/null 2>&1 || true
  git add -A
  git commit -q -m "${BASELINE_MESSAGE}"
  git branch -M main
  git worktree prune >/dev/null 2>&1 || true
  while IFS= read -r branch; do
    if [[ -z "${branch}" || "${branch}" == "main" ]]; then
      continue
    fi
    git branch -D "${branch}" >/dev/null 2>&1 || true
  done < <(git for-each-ref --format='%(refname:short)' refs/heads)
  git reset -q --hard
}

print_summary() {
  echo "Fixture reset complete."
  echo "Seeded IDs: bd-100 bd-101 bd-102 bd-103 bd-104 bd-105"

  if command -v jq >/dev/null 2>&1; then
    bd ready --json | jq
  else
    bd ready
  fi
}

require_cmd bd
require_cmd git

rm -rf .beads .bmux .worktrees

ensure_git_repo
write_baseline_files
seed_beads
rebuild_git_baseline
print_summary
