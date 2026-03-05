package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Molin-L/bmux/internal/beads"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

type BeadsClient interface {
	Ready(ctx context.Context) ([]model.Issue, error)
	List(ctx context.Context, filters map[string]string) ([]model.Issue, error)
	Show(ctx context.Context, issueID string) (model.Issue, error)
	Dependencies(ctx context.Context, issueID string) ([]model.Dependency, error)
	CreateIssue(ctx context.Context, req model.CreateIssueRequest) (model.Issue, error)
	UpdateMetadata(ctx context.Context, issueID string, metadata map[string]string) error
	Close(ctx context.Context, issueID, reason string) error
}

type GitClient interface {
	CurrentHead(ctx context.Context, repoRoot string) (string, string, error)
	AddWorktree(ctx context.Context, repoRoot, path, branch, startRef string) error
	RemoveWorktree(ctx context.Context, repoRoot, path string) error
	Merge(ctx context.Context, repoPath, branch string) error
	DeleteBranch(ctx context.Context, repoRoot, branch string) error
	Push(ctx context.Context, repoRoot, branch string) error
}

type BranchDecision struct {
	Branch        string
	ReuseExisting bool
	SourceIssueID string
}

type BranchPlanner interface {
	ResolveBranch(issue model.Issue, metas []model.TaskBranchMeta) BranchDecision
}

type PRPromptBuilder interface {
	Build(issue model.Issue, meta model.TaskBranchMeta) string
}

type TmuxClient interface {
	SplitPane(ctx context.Context, direction, cwd string) (string, error)
	SplitPaneOnTarget(ctx context.Context, direction, cwd, target string) (string, error)
	SendKeys(ctx context.Context, paneID, text string, enter bool) error
	CapturePane(ctx context.Context, paneID string, lines int) (string, error)
	CurrentPaneID(ctx context.Context) (string, error)
	ListPanes(ctx context.Context, target string) ([]string, error)
	SetWindowOptionsForSidebar(ctx context.Context, target string, controlWidth int) error
	SelectLayoutMainVertical(ctx context.Context, target string) error
	SetBuffer(ctx context.Context, bufferName, content string) error
	PasteBuffer(ctx context.Context, bufferName, paneID string) error
	DeleteBuffer(ctx context.Context, bufferName string) error
	GetPaneCurrentCommand(ctx context.Context, paneID string) (string, error)
}

type PlanItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

type PlanPayload struct {
	Goal     string     `json:"goal"`
	Epic     PlanItem   `json:"epic"`
	Task     PlanItem   `json:"task"`
	Subtasks []PlanItem `json:"subtasks"`
}

type HierarchyResult struct {
	EpicID     string
	TaskID     string
	SubtaskIDs []string
}

type Options struct {
	RepoRoot       string
	WorktreeDir    string
	Store          *state.Store
	Beads          BeadsClient
	Git            GitClient
	Planner        BranchPlanner
	PromptBuilder  PRPromptBuilder
	Tmux           TmuxClient
	SplitDirection string
	ClaudeCommand  string
	CodexCommand   string
	PromptTemplate string
	TmuxLayout     string
	ControlWidth   int
}

type DoctorReport struct {
	GitOK          bool
	BDOK           bool
	RepoRoot       string
	WorktreeDir    string
	WorktreeDirOK  bool
	CurrentBranch  string
	CurrentCommit  string
	DetailMessages []string
}

type IssueSourceState struct {
	Available bool
	Reason    string
}

type Service struct {
	repoRoot       string
	worktreeDir    string
	store          *state.Store
	beads          BeadsClient
	git            GitClient
	planner        BranchPlanner
	promptBuilder  PRPromptBuilder
	tmux           TmuxClient
	splitDirection string
	tmuxLayout     string
	controlWidth   int
	agentCommands  map[string]string
	promptTemplate string
}

func NewService(opts Options) *Service {
	return &Service{
		repoRoot:       opts.RepoRoot,
		worktreeDir:    opts.WorktreeDir,
		store:          opts.Store,
		beads:          opts.Beads,
		git:            opts.Git,
		planner:        opts.Planner,
		promptBuilder:  opts.PromptBuilder,
		tmux:           opts.Tmux,
		splitDirection: normalizeSplitDirection(opts.SplitDirection),
		tmuxLayout:     normalizeTmuxLayout(opts.TmuxLayout),
		controlWidth:   normalizeControlWidth(opts.ControlWidth),
		agentCommands: map[string]string{
			"claude": strings.TrimSpace(opts.ClaudeCommand),
			"codex":  strings.TrimSpace(opts.CodexCommand),
		},
		promptTemplate: planPromptTemplate(opts.PromptTemplate),
	}
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

func planPromptTemplate(v string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return defaultPlanPromptTemplate
}

func (s *Service) ReadyIssues(ctx context.Context) ([]model.Issue, error) {
	return s.beads.Ready(ctx)
}

func (s *Service) ReadyIssuesState(ctx context.Context) ([]model.Issue, IssueSourceState, error) {
	issues, err := s.beads.Ready(ctx)
	if err == nil {
		return issues, IssueSourceState{Available: true}, nil
	}
	if reason, unavailable := beads.BDUnavailableReason(err); unavailable {
		return []model.Issue{}, IssueSourceState{Available: false, Reason: reason}, nil
	}
	if beads.IsNoReadyIssuesError(err) {
		return []model.Issue{}, IssueSourceState{Available: true}, nil
	}
	return nil, IssueSourceState{Available: true}, err
}

func (s *Service) OpenTask(ctx context.Context, issueID string) (model.TaskBranchMeta, error) {
	if issueID == "" {
		return model.TaskBranchMeta{}, errors.New("issue id is required")
	}

	if existing, ok, err := s.store.ByIssueID(issueID); err == nil && ok && existing.Status == "active" {
		if existing.WorktreePath == "" {
			return existing, nil
		}
		if _, statErr := os.Stat(existing.WorktreePath); statErr == nil {
			return existing, nil
		}
	}

	issue, err := s.beads.Show(ctx, issueID)
	if err != nil {
		return model.TaskBranchMeta{}, err
	}
	deps, _ := s.beads.Dependencies(ctx, issueID)
	issue.Dependencies = deps

	all, err := s.store.All()
	if err != nil {
		return model.TaskBranchMeta{}, err
	}
	decision := s.planner.ResolveBranch(issue, all)

	meta, err := s.buildMetaFromDecision(ctx, issue, decision, all)
	if err != nil {
		return model.TaskBranchMeta{}, err
	}

	if err := s.store.Upsert(meta); err != nil {
		return model.TaskBranchMeta{}, err
	}
	_ = s.beads.UpdateMetadata(ctx, issueID, map[string]string{
		"bmux.branch":        meta.Branch,
		"bmux.worktree_path": meta.WorktreePath,
		"bmux.base_branch":   meta.BaseBranch,
		"bmux.base_commit":   meta.BaseCommit,
	})

	return meta, nil
}

func (s *Service) buildMetaFromDecision(ctx context.Context, issue model.Issue, decision BranchDecision, all []model.TaskBranchMeta) (model.TaskBranchMeta, error) {
	now := time.Now().UTC()

	if decision.ReuseExisting {
		for _, m := range all {
			if m.IssueID == decision.SourceIssueID {
				return model.TaskBranchMeta{
					IssueID:      issue.ID,
					Branch:       m.Branch,
					WorktreePath: m.WorktreePath,
					BaseBranch:   m.BaseBranch,
					BaseCommit:   m.BaseCommit,
					CreatedAt:    now,
					Status:       "active",
				}, nil
			}
		}
	}

	baseBranch, baseCommit, err := s.git.CurrentHead(ctx, s.repoRoot)
	if err != nil {
		return model.TaskBranchMeta{}, err
	}

	worktreePath := filepath.Join(s.worktreeDir, sanitizeBranchToken(decision.Branch))
	if err := s.git.AddWorktree(ctx, s.repoRoot, worktreePath, decision.Branch, baseCommit); err != nil {
		return model.TaskBranchMeta{}, err
	}

	return model.TaskBranchMeta{
		IssueID:      issue.ID,
		Branch:       decision.Branch,
		WorktreePath: worktreePath,
		BaseBranch:   baseBranch,
		BaseCommit:   baseCommit,
		CreatedAt:    now,
		Status:       "active",
	}, nil
}

func (s *Service) GeneratePRPrompt(ctx context.Context, issueID string) (string, error) {
	issue, err := s.beads.Show(ctx, issueID)
	if err != nil {
		return "", err
	}
	meta, ok, err := s.store.ByIssueID(issueID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no branch metadata for issue %s; open task first", issueID)
	}

	prompt := s.promptBuilder.Build(issue, meta)
	meta.PRPromptedAt = time.Now().UTC()
	if err := s.store.Upsert(meta); err != nil {
		return "", err
	}
	_ = s.beads.UpdateMetadata(ctx, issueID, map[string]string{
		"bmux.pr_prompt_generated_at": meta.PRPromptedAt.Format(time.RFC3339),
	})
	return prompt, nil
}

func (s *Service) MergeTask(ctx context.Context, issueID string, cleanup bool) (model.TaskBranchMeta, error) {
	meta, ok, err := s.store.ByIssueID(issueID)
	if err != nil {
		return model.TaskBranchMeta{}, err
	}
	if !ok {
		return model.TaskBranchMeta{}, fmt.Errorf("issue %s not found in bmux state", issueID)
	}

	if err := s.git.Merge(ctx, meta.WorktreePath, meta.BaseBranch); err != nil {
		return model.TaskBranchMeta{}, err
	}
	if err := s.git.Merge(ctx, s.repoRoot, meta.Branch); err != nil {
		return model.TaskBranchMeta{}, err
	}
	if cleanup {
		if err := s.git.RemoveWorktree(ctx, s.repoRoot, meta.WorktreePath); err != nil {
			return model.TaskBranchMeta{}, err
		}
		if err := s.git.DeleteBranch(ctx, s.repoRoot, meta.Branch); err != nil {
			return model.TaskBranchMeta{}, err
		}
	}

	meta.Status = "merged"
	if err := s.store.Upsert(meta); err != nil {
		return model.TaskBranchMeta{}, err
	}
	return meta, nil
}

func (s *Service) CleanupTask(ctx context.Context, issueID string) (model.TaskBranchMeta, error) {
	meta, ok, err := s.store.ByIssueID(issueID)
	if err != nil {
		return model.TaskBranchMeta{}, err
	}
	if !ok {
		return model.TaskBranchMeta{}, fmt.Errorf("issue %s not found in bmux state", issueID)
	}

	if err := s.git.RemoveWorktree(ctx, s.repoRoot, meta.WorktreePath); err != nil {
		return model.TaskBranchMeta{}, err
	}
	if err := s.git.DeleteBranch(ctx, s.repoRoot, meta.Branch); err != nil {
		return model.TaskBranchMeta{}, err
	}
	meta.Status = "cleaned"
	if err := s.store.Upsert(meta); err != nil {
		return model.TaskBranchMeta{}, err
	}
	return meta, nil
}

func (s *Service) GetTaskMeta(issueID string) (model.TaskBranchMeta, bool, error) {
	return s.store.ByIssueID(issueID)
}

func (s *Service) StartPlanningPane(ctx context.Context, agent string) (string, string, error) {
	if s.tmux == nil {
		return "", "", errors.New("tmux integration is not configured")
	}
	agent = strings.ToLower(strings.TrimSpace(agent))
	cmd, source, err := s.resolveAgentCommand(agent)
	if err != nil {
		return "", "", err
	}

	paneID, err := s.createPlanningPane(ctx)
	if err != nil {
		return "", "", err
	}
	var details string
	switch agent {
	case "codex":
		details, err = s.startCodexAgent(ctx, paneID, cmd)
		if err != nil {
			return "", "", err
		}
	default:
		if err := s.tmux.SendKeys(ctx, paneID, cmd, true); err != nil {
			return "", "", err
		}
		if err := s.tmux.SendKeys(ctx, paneID, s.promptTemplate, true); err != nil {
			return "", "", err
		}
		details = "Template prompt sent."
	}
	trailing := "Press c to capture plan JSON."
	if agent == "codex" {
		trailing = "Codex will create bd issues directly in this pane."
	}
	status := fmt.Sprintf("Planning pane %s started with %s (%s). %s %s", paneID, agent, source, details, trailing)
	return paneID, status, nil
}

func (s *Service) createPlanningPane(ctx context.Context) (string, error) {
	if s.tmuxLayout != "sidebar" {
		return s.tmux.SplitPane(ctx, s.splitDirection, s.repoRoot)
	}
	currentPane, err := s.tmux.CurrentPaneID(ctx)
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(currentPane)
	panes, err := s.tmux.ListPanes(ctx, "")
	if err == nil {
		for _, pane := range panes {
			p := strings.TrimSpace(pane)
			if p != "" && p != currentPane {
				target = p
			}
		}
	}
	paneID, err := s.tmux.SplitPaneOnTarget(ctx, s.splitDirection, s.repoRoot, target)
	if err != nil {
		return "", err
	}
	_ = s.tmux.SetWindowOptionsForSidebar(ctx, "", s.controlWidth)
	_ = s.tmux.SelectLayoutMainVertical(ctx, "")
	return paneID, nil
}

func (s *Service) startCodexAgent(ctx context.Context, paneID, cmd string) (string, error) {
	statuses := []string{"Waiting for Codex readiness..."}
	launchCmd, err := s.buildCodexPlanningCommand(cmd, s.promptTemplate)
	if err != nil {
		return "", err
	}
	if err := s.tmux.SendKeys(ctx, paneID, launchCmd, true); err != nil {
		return "", err
	}
	statuses = append(statuses, "Started in planning policy (workspace-write/on-request). Direct bd-creation prompt sent as startup argument.")
	_ = s.waitForPaneCommand(ctx, paneID, "codex", 5*time.Second)

	content, _ := s.tmux.CapturePane(ctx, paneID, 100)
	if hasTrustPrompt(content) {
		statuses = append(statuses, "Trust prompt detected, auto-confirming...")
		_ = s.tmux.SendKeys(ctx, paneID, "", true)
		time.Sleep(200 * time.Millisecond)
		again, _ := s.tmux.CapturePane(ctx, paneID, 100)
		if hasTrustPrompt(again) {
			_ = s.tmux.SendKeys(ctx, paneID, "y", true)
			time.Sleep(200 * time.Millisecond)
		}
	}
	return strings.Join(statuses, " "), nil
}

func (s *Service) resolveAgentCommand(agent string) (string, string, error) {
	if cmd := strings.TrimSpace(s.agentCommands[agent]); cmd != "" {
		return cmd, "config command", nil
	}
	if _, err := lookPath(agent); err == nil {
		return agent, "PATH fallback", nil
	}
	return "", "", fmt.Errorf("agent command is not configured and '%s' is not in PATH (set agents.%s.command)", agent, agent)
}

func (s *Service) waitForPaneCommand(ctx context.Context, paneID, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current, err := s.tmux.GetPaneCurrentCommand(ctx, paneID)
		if err == nil && strings.EqualFold(strings.TrimSpace(current), strings.TrimSpace(expected)) {
			return nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return errors.New("timed out waiting for codex startup")
}

func hasTrustPrompt(content string) bool {
	lower := strings.ToLower(content)
	patterns := []string{
		"do you trust",
		"trust this workspace",
		"trust this folder",
		"trust this directory",
		"add this directory to trusted",
		"add to trusted",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (s *Service) buildCodexPlanningCommand(baseCmd, prompt string) (string, error) {
	baseCmd = strings.TrimSpace(baseCmd)
	if baseCmd == "" {
		return "", errors.New("codex command is empty")
	}
	cmd := baseCmd
	if !hasCLIFlag(cmd, "--sandbox") {
		cmd += " --sandbox workspace-write"
	}
	if !hasCLIFlag(cmd, "--ask-for-approval") {
		cmd += " --ask-for-approval on-request"
	}
	if strings.TrimSpace(prompt) != "" {
		cmd += " " + shellQuote(prompt)
	}
	return cmd, nil
}

func hasCLIFlag(cmd, flag string) bool {
	return strings.Contains(cmd, flag+" ") || strings.Contains(cmd, flag+"=") || strings.HasSuffix(strings.TrimSpace(cmd), flag)
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func (s *Service) ExtractPlanJSON(ctx context.Context, paneID string) (PlanPayload, error) {
	if s.tmux == nil {
		return PlanPayload{}, errors.New("tmux integration is not configured")
	}
	raw, err := s.tmux.CapturePane(ctx, paneID, 400)
	if err != nil {
		return PlanPayload{}, err
	}
	jsonBlock, err := extractMarkedJSON(raw)
	if err != nil {
		return PlanPayload{}, err
	}
	var plan PlanPayload
	if err := json.Unmarshal([]byte(jsonBlock), &plan); err != nil {
		return PlanPayload{}, fmt.Errorf("invalid plan json: %w", err)
	}
	if err := validatePlanPayload(plan); err != nil {
		return PlanPayload{}, err
	}
	return plan, nil
}

func (s *Service) CreateHierarchyFromPlan(ctx context.Context, plan PlanPayload) (HierarchyResult, error) {
	if err := validatePlanPayload(plan); err != nil {
		return HierarchyResult{}, err
	}
	epic, err := s.beads.CreateIssue(ctx, model.CreateIssueRequest{
		Title:       plan.Epic.Title,
		Description: plan.Epic.Description,
		Type:        "epic",
		Priority:    plan.Epic.Priority,
	})
	if err != nil {
		return HierarchyResult{}, fmt.Errorf("create epic: %w", err)
	}

	res := HierarchyResult{EpicID: epic.ID}
	task, err := s.beads.CreateIssue(ctx, model.CreateIssueRequest{
		Title:       plan.Task.Title,
		Description: plan.Task.Description,
		Type:        "task",
		Priority:    plan.Task.Priority,
		ParentID:    epic.ID,
	})
	if err != nil {
		return res, fmt.Errorf("create task: %w", err)
	}
	res.TaskID = task.ID
	for idx, st := range plan.Subtasks {
		sub, createErr := s.beads.CreateIssue(ctx, model.CreateIssueRequest{
			Title:       st.Title,
			Description: st.Description,
			Type:        "task",
			Priority:    st.Priority,
			ParentID:    task.ID,
		})
		if createErr != nil {
			return res, fmt.Errorf("create subtask %d: %w", idx+1, createErr)
		}
		res.SubtaskIDs = append(res.SubtaskIDs, sub.ID)
	}
	return res, nil
}

func (s *Service) Doctor(ctx context.Context) (DoctorReport, error) {
	report := DoctorReport{RepoRoot: s.repoRoot, WorktreeDir: s.worktreeDir}

	if _, err := exec.LookPath("git"); err == nil {
		report.GitOK = true
	} else {
		report.DetailMessages = append(report.DetailMessages, "git not found in PATH")
	}
	if _, err := exec.LookPath("bd"); err == nil {
		report.BDOK = true
	} else {
		report.DetailMessages = append(report.DetailMessages, "bd not found in PATH")
	}

	if report.GitOK {
		if branch, commit, err := s.git.CurrentHead(ctx, s.repoRoot); err == nil {
			report.CurrentBranch = branch
			report.CurrentCommit = commit
		} else {
			report.DetailMessages = append(report.DetailMessages, fmt.Sprintf("failed to read git head: %v", err))
		}
	}

	if err := os.MkdirAll(s.worktreeDir, 0o755); err == nil {
		report.WorktreeDirOK = true
	} else {
		report.DetailMessages = append(report.DetailMessages, fmt.Sprintf("worktree dir unusable: %v", err))
	}

	return report, nil
}

func sanitizeBranchToken(branch string) string {
	t := strings.ReplaceAll(branch, "/", "__")
	t = strings.ReplaceAll(t, string(filepath.Separator), "__")
	return strings.TrimSpace(t)
}

func extractMarkedJSON(raw string) (string, error) {
	startMarker := "BMUX_PLAN_JSON_BEGIN"
	endMarker := "BMUX_PLAN_JSON_END"
	start := strings.Index(raw, startMarker)
	end := strings.Index(raw, endMarker)
	if start < 0 || end < 0 || end <= start {
		return "", errors.New("plan json block not found; ask agent to reprint block with markers")
	}
	block := strings.TrimSpace(raw[start+len(startMarker) : end])
	if block == "" {
		return "", errors.New("plan json block is empty")
	}
	return block, nil
}

func validatePlanPayload(plan PlanPayload) error {
	if strings.TrimSpace(plan.Goal) == "" {
		return errors.New("plan.goal is required")
	}
	if err := validatePlanItem("epic", plan.Epic); err != nil {
		return err
	}
	if err := validatePlanItem("task", plan.Task); err != nil {
		return err
	}
	if len(plan.Subtasks) == 0 {
		return errors.New("plan.subtasks must contain at least one item")
	}
	for i, st := range plan.Subtasks {
		if err := validatePlanItem("subtasks["+strconv.Itoa(i)+"]", st); err != nil {
			return err
		}
	}
	return nil
}

func validatePlanItem(name string, item PlanItem) error {
	if strings.TrimSpace(item.Title) == "" {
		return fmt.Errorf("%s.title is required", name)
	}
	if item.Priority < 0 || item.Priority > 4 {
		return fmt.Errorf("%s.priority must be between 0 and 4", name)
	}
	return nil
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

var lookPath = exec.LookPath
