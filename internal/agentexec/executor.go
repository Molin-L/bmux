package agentexec

import (
	"strings"

	"github.com/Molin-L/bmux/internal/layout"
	"github.com/Molin-L/bmux/internal/state"
)

type Executor struct {
	repoRoot         string
	store            *state.Store
	beads            BeadsClient
	tmux             TmuxClient
	openTask         OpenTaskFunc
	readyIssues      ReadyIssuesFunc
	layoutManager    *layout.Manager
	splitDirection   string
	tmuxLayout       string
	controlWidth     int
	minPaneWidth     int
	maxPaneWidth     int
	agentCommands    map[string]string
	promptTemplate   string
	apeMaxParallel int
	executionPrompts executionPrompts
}

type executionPrompts struct {
	plan    string
	selfRun string
	ape   string
}

func New(opts Options) *Executor {
	planPrompt, selfRunPrompt, apePrompt := executionPromptTemplates(opts.ExecutionPlanPrompt, opts.ExecutionSelfRunPrompt, opts.ExecutionApePrompt)
	controlWidth := normalizeControlWidth(opts.ControlWidth)
	minPaneWidth, maxPaneWidth := normalizePaneWidths(opts.MinPaneWidth, opts.MaxPaneWidth)
	tmuxLayout := normalizeTmuxLayout(opts.TmuxLayout)

	e := &Executor{
		repoRoot:       opts.RepoRoot,
		store:          opts.Store,
		beads:          opts.Beads,
		tmux:           opts.Tmux,
		openTask:       opts.OpenTask,
		readyIssues:    opts.ReadyIssues,
		splitDirection: normalizeSplitDirection(opts.SplitDirection),
		tmuxLayout:     tmuxLayout,
		controlWidth:   controlWidth,
		minPaneWidth:   minPaneWidth,
		maxPaneWidth:   maxPaneWidth,
		agentCommands: map[string]string{
			"claude": strings.TrimSpace(opts.ClaudeCommand),
			"codex":  strings.TrimSpace(opts.CodexCommand),
		},
		promptTemplate:   planPromptTemplate(opts.PromptTemplate),
		apeMaxParallel: normalizeApeMaxParallel(opts.ApeMaxParallel),
		executionPrompts: executionPrompts{
			plan:    planPrompt,
			selfRun: selfRunPrompt,
			ape:   apePrompt,
		},
	}

	if e.tmux != nil && e.tmuxLayout == "sidebar" {
		e.layoutManager = layout.NewManager(e.tmux, layout.Config{
			SidebarWidth:       e.controlWidth,
			MinPaneWidth:       e.minPaneWidth,
			MaxPaneWidth:       e.maxPaneWidth,
			MinPaneHeight:      15,
			MinSpacerPaneWidth: 20,
		})
	}
	return e
}

func normalizeSplitDirection(v string) string {
	if v == "below" {
		return "below"
	}
	return "right"
}

func normalizeTmuxLayout(v string) string {
	if strings.TrimSpace(v) == "single" {
		return "single"
	}
	return "sidebar"
}

func normalizeControlWidth(v int) int {
	if v <= 0 {
		return 40
	}
	return v
}

func normalizePaneWidth(v, fallback int) int {
	if v == 0 {
		v = fallback
	}
	if v < 40 {
		return 40
	}
	if v > 300 {
		return 300
	}
	return v
}

func normalizePaneWidths(minPaneWidth, maxPaneWidth int) (int, int) {
	minPaneWidth = normalizePaneWidth(minPaneWidth, 50)
	maxPaneWidth = normalizePaneWidth(maxPaneWidth, 80)
	if minPaneWidth > maxPaneWidth {
		maxPaneWidth = minPaneWidth
	}
	return minPaneWidth, maxPaneWidth
}

func normalizeApeMaxParallel(v int) int {
	if v <= 0 {
		return 2
	}
	return v
}

func planPromptTemplate(v string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return defaultPlanPromptTemplate
}

func executionPromptTemplates(plan, selfRun, ape string) (string, string, string) {
	if strings.TrimSpace(plan) == "" {
		plan = defaultModePlanPromptTemplate
	}
	if strings.TrimSpace(selfRun) == "" {
		selfRun = defaultModeSelfRunPromptTemplate
	}
	if strings.TrimSpace(ape) == "" {
		ape = defaultModeApePromptTemplate
	}
	return plan, selfRun, ape
}

const defaultPlanPromptTemplate = `You are a Beads issue planner.
Goal: create bd issues directly (epic -> task -> subtasks). Do NOT return JSON.

Workflow:
1) Ask concise clarifying questions to gather missing context:
   - desired goal/outcome
   - scope and non-scope
   - constraints/priorities
2) You may use any bd command needed to inspect current state and plan correctly
   (for example: bd ready/list/show/query/dep/create/update/close/types).
3) After context is clear, create issues by running bd commands directly in this terminal:
   - create one epic (type=epic)
   - create one task under that epic (type=task, --parent <epic-id>)
   - create 1+ subtasks under that task (type=task, --parent <task-id>)
4) Use clear, specific titles/descriptions tied to user context; avoid generic placeholders.
5) If command fails, diagnose and retry with corrected command.
6) Final response must include:
   - created issue IDs
   - parent-child relationships
   - short summary of each issue.

Important:
- This is issue creation only, not implementation planning.
- Execute bd create commands; do not ask the user to run them manually.`

const defaultModePlanPromptTemplate = `You are running in bmux PLAN mode for one task.

Task:
- ID: {{issue_id}}
- Title: {{issue_title}}
- Status: {{issue_status}}
- Branch: {{branch}}
- Worktree: {{worktree_path}}

Rules:
1) Ask concise clarifying questions first.
2) Produce an implementation plan only.
3) Do not edit files unless the user explicitly asks for implementation.
4) Include acceptance tests and risks.
`

const defaultModeSelfRunPromptTemplate = `You are running in bmux SELF-RUN mode for one task.

Task:
- ID: {{issue_id}}
- Title: {{issue_title}}
- Status: {{issue_status}}
- Branch: {{branch}}
- Worktree: {{worktree_path}}

Execution contract:
1) Implement only this task.
2) Run relevant lint/tests before finishing.
3) Report: changed files, commands, test output summary, and risks/open questions.
4) Stay within this worktree and branch.
`

const defaultModeApePromptTemplate = `You are running in bmux APE mode for one task from a queue.

Task:
- ID: {{issue_id}}
- Title: {{issue_title}}
- Status: {{issue_status}}
- Branch: {{branch}}
- Worktree: {{worktree_path}}

Execution contract:
1) Implement only this task and do not switch tasks.
2) Run relevant lint/tests before finishing.
3) Report: changed files, commands, test output summary, and risks/open questions.
4) Exit to prompt when done so bmux can schedule next tasks.
`
