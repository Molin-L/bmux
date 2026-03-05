package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Molin-L/bmux/internal/beads"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/promptx"
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
	RepoRoot               string
	WorktreeDir            string
	Store                  *state.Store
	Beads                  BeadsClient
	Git                    GitClient
	Planner                BranchPlanner
	PromptBuilder          PRPromptBuilder
	Tmux                   TmuxClient
	SplitDirection         string
	ClaudeCommand          string
	CodexCommand           string
	PromptTemplate         string
	TmuxLayout             string
	ControlWidth           int
	ChaosMaxParallel       int
	ExecutionPlanPrompt    string
	ExecutionSelfRunPrompt string
	ExecutionChaosPrompt   string
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
	repoRoot         string
	worktreeDir      string
	store            *state.Store
	beads            BeadsClient
	git              GitClient
	planner          BranchPlanner
	promptBuilder    PRPromptBuilder
	tmux             TmuxClient
	splitDirection   string
	tmuxLayout       string
	controlWidth     int
	agentCommands    map[string]string
	promptTemplate   string
	chaosMaxParallel int
	executionPrompts executionPrompts
}

type executionPrompts struct {
	plan    string
	selfRun string
	chaos   string
}

type RunSummary struct {
	Running     int
	Launched    int
	Finished    int
	ActiveChaos bool
}

func NewService(opts Options) *Service {
	planPrompt, selfRunPrompt, chaosPrompt := executionPromptTemplates(opts.ExecutionPlanPrompt, opts.ExecutionSelfRunPrompt, opts.ExecutionChaosPrompt)
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
		promptTemplate:   planPromptTemplate(opts.PromptTemplate),
		chaosMaxParallel: normalizeChaosMaxParallel(opts.ChaosMaxParallel),
		executionPrompts: executionPrompts{
			plan:    planPrompt,
			selfRun: selfRunPrompt,
			chaos:   chaosPrompt,
		},
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

func normalizeChaosMaxParallel(v int) int {
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

func executionPromptTemplates(plan, selfRun, chaos string) (string, string, string) {
	if strings.TrimSpace(plan) == "" {
		plan = defaultModePlanPromptTemplate
	}
	if strings.TrimSpace(selfRun) == "" {
		selfRun = defaultModeSelfRunPromptTemplate
	}
	if strings.TrimSpace(chaos) == "" {
		chaos = defaultModeChaosPromptTemplate
	}
	return plan, selfRun, chaos
}

func (s *Service) ReadyIssues(ctx context.Context) ([]model.Issue, error) {
	return s.beads.Ready(ctx)
}

func (s *Service) ReadyIssuesState(ctx context.Context) ([]model.Issue, IssueSourceState, error) {
	issues, err := s.beads.Ready(ctx)
	if err == nil {
		return s.enrichIssueHierarchy(ctx, issues), IssueSourceState{Available: true}, nil
	}
	if reason, unavailable := beads.BDUnavailableReason(err); unavailable {
		return []model.Issue{}, IssueSourceState{Available: false, Reason: reason}, nil
	}
	if beads.IsNoReadyIssuesError(err) {
		return []model.Issue{}, IssueSourceState{Available: true}, nil
	}
	return nil, IssueSourceState{Available: true}, err
}

func (s *Service) enrichIssueHierarchy(ctx context.Context, issues []model.Issue) []model.Issue {
	if len(issues) == 0 {
		return issues
	}

	cache := make(map[string]model.Issue, len(issues))
	for _, issue := range issues {
		if strings.TrimSpace(issue.ID) == "" {
			continue
		}
		cache[issue.ID] = issue
	}

	enriched := make([]model.Issue, len(issues))
	for i, issue := range issues {
		epicID, epicTitle, depth := s.resolveIssueEpic(ctx, issue, cache)
		issue.EpicID = epicID
		issue.EpicTitle = epicTitle
		issue.HierarchyDepth = depth
		enriched[i] = issue
	}
	return enriched
}

func (s *Service) resolveIssueEpic(ctx context.Context, issue model.Issue, cache map[string]model.Issue) (string, string, int) {
	current := issue
	depth := 0
	visited := map[string]struct{}{}

	for {
		if strings.EqualFold(strings.TrimSpace(current.IssueType), "epic") {
			return current.ID, current.Title, depth
		}

		currentID := strings.TrimSpace(current.ID)
		if currentID != "" {
			if _, seen := visited[currentID]; seen {
				return "", "", depth
			}
			visited[currentID] = struct{}{}
		}

		parentID := resolveParentID(current)
		if parentID == "" {
			return "", "", depth
		}

		depth++
		if _, seen := visited[parentID]; seen {
			return "", "", depth
		}

		parent, ok := cache[parentID]
		if !ok {
			fetched, err := s.beads.Show(ctx, parentID)
			if err != nil {
				return "", "", depth
			}
			parent = fetched
			cache[parentID] = parent
		}
		current = parent
	}
}

func resolveParentID(issue model.Issue) string {
	parentID := strings.TrimSpace(issue.ParentID)
	if parentID != "" {
		return parentID
	}

	for _, dep := range issue.Dependencies {
		if dep.Type != "parent-child" {
			continue
		}

		candidate := strings.TrimSpace(dep.IssueID)
		if candidate != "" && candidate != issue.ID {
			return candidate
		}

		candidate = strings.TrimSpace(dep.TargetID)
		if candidate != "" && candidate != issue.ID {
			return candidate
		}
	}

	return ""
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

func (s *Service) GetLiveRun(issueID string) (model.LiveRun, bool, error) {
	meta, ok, err := s.store.RunLockByIssueID(issueID)
	if err != nil {
		return model.LiveRun{}, false, err
	}
	if !ok {
		return model.LiveRun{}, false, nil
	}
	return model.LiveRun{
		IssueID:   meta.IssueID,
		Mode:      meta.Mode,
		PaneID:    meta.PaneID,
		Running:   true,
		Agent:     meta.Agent,
		StartedAt: meta.StartedAt,
	}, true, nil
}

func (s *Service) GetTaskRunMeta(issueID string) (model.TaskRunMeta, bool, error) {
	return s.store.RunLockByIssueID(issueID)
}

func (s *Service) GetChaosState() (model.ChaosState, bool, error) {
	return s.store.ChaosState()
}

func (s *Service) RunSummary(issueIDs []string) (RunSummary, error) {
	runs, err := s.store.RunLockAll()
	if err != nil {
		return RunSummary{}, err
	}
	allowed := map[string]struct{}{}
	for _, issueID := range issueIDs {
		if v := strings.TrimSpace(issueID); v != "" {
			allowed[v] = struct{}{}
		}
	}
	sum := RunSummary{}
	for _, run := range runs {
		if len(allowed) > 0 {
			if _, ok := allowed[run.IssueID]; !ok {
				continue
			}
		}
		sum.Running++
	}
	if chaos, ok, err := s.store.ChaosState(); err == nil && ok && chaos.Active {
		sum.ActiveChaos = true
		sum.Launched = len(dedupeStrings(chaos.LaunchedIssueIDs))
		sum.Finished = len(dedupeStrings(chaos.FinishedIssueIDs))
	}
	return sum, nil
}

func (s *Service) ComputeBlockedBy(ctx context.Context, issues []model.Issue) (map[string]string, error) {
	blockedBy := map[string]string{}
	if len(issues) == 0 {
		return blockedBy, nil
	}

	issueSet := map[string]struct{}{}
	for _, issue := range issues {
		if v := strings.TrimSpace(issue.ID); v != "" {
			issueSet[v] = struct{}{}
		}
	}

	for _, issue := range issues {
		issueID := strings.TrimSpace(issue.ID)
		if issueID == "" {
			continue
		}
		candidates := make([]string, 0, 3)
		if parentID := strings.TrimSpace(issue.ParentID); parentID != "" && parentID != issueID {
			if _, ok := issueSet[parentID]; ok {
				candidates = append(candidates, parentID)
			}
		}

		deps := issue.Dependencies
		if len(deps) == 0 {
			if fetched, depErr := s.beads.Dependencies(ctx, issueID); depErr == nil {
				deps = fetched
			}
		}
		for _, dep := range deps {
			if dep.Type != "blocks" || dep.Direction != "outgoing" {
				continue
			}
			blockerID := strings.TrimSpace(dep.IssueID)
			if blockerID == "" || blockerID == issueID {
				continue
			}
			if _, ok := issueSet[blockerID]; ok {
				candidates = append(candidates, blockerID)
			}
		}

		for _, blockerID := range dedupeStrings(candidates) {
			blockedBy[issueID] = blockerID
			break
		}
	}
	return blockedBy, nil
}

func (s *Service) StartTaskMode(ctx context.Context, issueID string, mode model.RunMode) (model.TaskRunMeta, error) {
	return s.startTaskModeInternal(ctx, issueID, mode, "", false)
}

func (s *Service) startTaskModeInternal(ctx context.Context, issueID string, mode model.RunMode, chaosSessionID string, allowQueued bool) (model.TaskRunMeta, error) {
	_ = allowQueued
	if mode != model.RunModePlan && mode != model.RunModeSelfRun && mode != model.RunModeChaos {
		return model.TaskRunMeta{}, fmt.Errorf("unsupported run mode: %s", mode)
	}
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return model.TaskRunMeta{}, errors.New("issue id is required")
	}
	if s.tmux == nil {
		return model.TaskRunMeta{}, errors.New("tmux integration is not configured")
	}

	_ = s.ReconcileRunLocks(ctx)
	if existing, ok, err := s.store.RunLockByIssueID(issueID); err == nil && ok {
		paneID := strings.TrimSpace(existing.PaneID)
		if paneID == "" {
			paneID = "(unknown)"
		}
		return model.TaskRunMeta{}, fmt.Errorf("task already running in pane %s", paneID)
	}

	taskMeta, err := s.OpenTask(ctx, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	issue, err := s.beads.Show(ctx, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	deps, _ := s.beads.Dependencies(ctx, issueID)
	issue.Dependencies = deps

	prompt := s.renderModePrompt(mode, issue, taskMeta)
	paneID, err := s.createPlanningPane(ctx)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	cmd, _, err := s.resolveAgentCommand("codex")
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	launchCmd, err := s.buildCodexModeCommand(cmd, prompt, mode, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	if err := s.tmux.SendKeys(ctx, paneID, launchCmd, true); err != nil {
		return model.TaskRunMeta{}, err
	}
	_ = s.waitForPaneCommand(ctx, paneID, "codex", 5*time.Second)

	content, _ := s.tmux.CapturePane(ctx, paneID, 100)
	if hasTrustPrompt(content) {
		_ = s.tmux.SendKeys(ctx, paneID, "", true)
		time.Sleep(200 * time.Millisecond)
		again, _ := s.tmux.CapturePane(ctx, paneID, 100)
		if hasTrustPrompt(again) {
			_ = s.tmux.SendKeys(ctx, paneID, "y", true)
			time.Sleep(200 * time.Millisecond)
		}
	}

	now := time.Now().UTC()
	run := model.TaskRunMeta{
		IssueID:         issueID,
		Mode:            mode,
		Agent:           "codex",
		PaneID:          paneID,
		StartedAt:       now,
		UpdatedAt:       now,
		ExpectedProcess: "codex",
		ChaosSessionID:  chaosSessionID,
	}
	if err := s.store.RunLockUpsert(run); err != nil {
		return model.TaskRunMeta{}, err
	}
	return run, nil
}

func (s *Service) StartChaos(ctx context.Context) (string, error) {
	issues, sourceState, err := s.ReadyIssuesState(ctx)
	if err != nil {
		return "", err
	}
	if !sourceState.Available {
		if sourceState.Reason == "" {
			return "", errors.New("ready issues source is unavailable")
		}
		return "", fmt.Errorf("ready issues source is unavailable: %s", sourceState.Reason)
	}
	if len(issues) == 0 {
		return "", errors.New("no ready issues for chaos mode")
	}
	sessionID := fmt.Sprintf("chaos-%d", time.Now().UTC().UnixNano())
	now := time.Now().UTC()
	chaos := model.ChaosState{
		SessionID:        sessionID,
		Active:           true,
		LaunchedIssueIDs: []string{},
		FinishedIssueIDs: []string{},
		ActiveIssueIDs:   []string{},
		StartedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.store.SetChaosState(&chaos); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *Service) TickChaos(ctx context.Context) (string, bool, error) {
	chaos, ok, err := s.store.ChaosState()
	if err != nil {
		return "", false, err
	}
	if !ok || !chaos.Active {
		return "Chaos idle", true, nil
	}

	if err := s.ReconcileRunLocks(ctx); err != nil {
		return "", false, err
	}

	runList, err := s.store.RunLockAll()
	if err != nil {
		return "", false, err
	}
	runs := map[string]model.TaskRunMeta{}
	for _, run := range runList {
		runs[run.IssueID] = run
	}
	launchedSet := stringsToSet(chaos.LaunchedIssueIDs)
	finishedSet := stringsToSet(chaos.FinishedIssueIDs)
	activeSet := stringsToSet(chaos.ActiveIssueIDs)

	for issueID := range activeSet {
		run, exists := runs[issueID]
		if !exists || run.ChaosSessionID != chaos.SessionID {
			delete(activeSet, issueID)
			finishedSet[issueID] = struct{}{}
		}
	}
	for _, run := range runs {
		if run.ChaosSessionID != chaos.SessionID {
			continue
		}
		activeSet[run.IssueID] = struct{}{}
		launchedSet[run.IssueID] = struct{}{}
	}

	readyIssues, sourceState, err := s.ReadyIssuesState(ctx)
	if err != nil {
		return "", false, err
	}
	if !sourceState.Available {
		return "Chaos paused: ready issues source unavailable", false, nil
	}

	issueSet := map[string]model.Issue{}
	for _, issue := range readyIssues {
		if strings.TrimSpace(issue.ID) == "" {
			continue
		}
		issueSet[issue.ID] = issue
	}

	runningCount := 0
	for issueID := range activeSet {
		if _, ok := issueSet[issueID]; ok {
			runningCount++
		}
	}
	capacity := s.chaosMaxParallel - runningCount
	if capacity < 0 {
		capacity = 0
	}

	queued := 0
	blocked := 0
	for _, issue := range readyIssues {
		issueID := strings.TrimSpace(issue.ID)
		if issueID == "" {
			continue
		}
		if _, done := finishedSet[issueID]; done {
			continue
		}
		if _, active := activeSet[issueID]; active {
			continue
		}

		blockers := make([]string, 0, 2)
		if parentID := strings.TrimSpace(issue.ParentID); parentID != "" {
			if _, ok := issueSet[parentID]; ok {
				blockers = append(blockers, parentID)
			}
		}
		deps, _ := s.beads.Dependencies(ctx, issueID)
		for _, dep := range deps {
			if dep.Type != "blocks" || dep.Direction != "outgoing" {
				continue
			}
			blockerID := strings.TrimSpace(dep.IssueID)
			if blockerID == "" {
				continue
			}
			if _, ok := issueSet[blockerID]; ok {
				blockers = append(blockers, blockerID)
			}
		}

		ready := true
		for _, blockerID := range dedupeStrings(blockers) {
			if _, done := finishedSet[blockerID]; !done {
				ready = false
				break
			}
		}
		if !ready {
			blocked++
			continue
		}
		if capacity <= 0 {
			queued++
			continue
		}

		if _, err := s.startTaskModeInternal(ctx, issueID, model.RunModeChaos, chaos.SessionID, true); err != nil {
			// Do not relaunch forever within the same chaos session.
			finishedSet[issueID] = struct{}{}
			continue
		}
		launchedSet[issueID] = struct{}{}
		activeSet[issueID] = struct{}{}
		capacity--
		runningCount++
	}

	pending := 0
	for _, issue := range readyIssues {
		issueID := strings.TrimSpace(issue.ID)
		if issueID == "" {
			continue
		}
		if _, done := finishedSet[issueID]; done {
			continue
		}
		if _, active := activeSet[issueID]; active {
			continue
		}
		pending++
	}
	done := pending == 0 && runningCount == 0
	chaos.Active = !done
	chaos.LaunchedIssueIDs = setToSortedSlice(launchedSet)
	chaos.FinishedIssueIDs = setToSortedSlice(finishedSet)
	chaos.ActiveIssueIDs = setToSortedSlice(activeSet)
	if done {
		chaos.ActiveIssueIDs = []string{}
	}
	chaos.UpdatedAt = time.Now().UTC()
	if err := s.store.SetChaosState(&chaos); err != nil {
		return "", false, err
	}
	status := fmt.Sprintf(
		"Chaos %s: running=%d queued=%d blocked=%d finished=%d",
		chaos.SessionID,
		runningCount,
		queued,
		blocked,
		len(finishedSet),
	)
	return status, done, nil
}

func (s *Service) ReconcileRunLocks(ctx context.Context) error {
	runs, err := s.store.RunLockAll()
	if err != nil {
		return err
	}
	if len(runs) == 0 || s.tmux == nil {
		return nil
	}
	panes, err := s.tmux.ListPanes(ctx, "")
	if err != nil {
		return err
	}
	paneSet := map[string]struct{}{}
	for _, pane := range panes {
		if v := strings.TrimSpace(pane); v != "" {
			paneSet[v] = struct{}{}
		}
	}

	for _, run := range runs {
		remove := false
		if strings.TrimSpace(run.PaneID) == "" {
			remove = true
		} else if _, ok := paneSet[run.PaneID]; !ok {
			remove = true
		} else if strings.TrimSpace(run.ExpectedProcess) != "" {
			currentCmd, cmdErr := s.tmux.GetPaneCurrentCommand(ctx, run.PaneID)
			if cmdErr == nil && !strings.EqualFold(strings.TrimSpace(currentCmd), strings.TrimSpace(run.ExpectedProcess)) {
				remove = true
			}
		}
		if remove {
			if err := s.store.RunLockDelete(run.IssueID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) ReconcileRuns(ctx context.Context) error {
	return s.ReconcileRunLocks(ctx)
}

func (s *Service) renderModePrompt(mode model.RunMode, issue model.Issue, taskMeta model.TaskBranchMeta) string {
	template := s.executionPrompts.selfRun
	switch mode {
	case model.RunModePlan:
		template = s.executionPrompts.plan
	case model.RunModeChaos:
		template = s.executionPrompts.chaos
	}
	replacer := strings.NewReplacer(
		"{{issue_id}}", issue.ID,
		"{{issue_title}}", issue.Title,
		"{{issue_description}}", issue.Description,
		"{{issue_status}}", issue.Status,
		"{{branch}}", taskMeta.Branch,
		"{{worktree_path}}", taskMeta.WorktreePath,
		"{{base_branch}}", taskMeta.BaseBranch,
		"{{base_commit}}", taskMeta.BaseCommit,
	)
	return strings.TrimSpace(replacer.Replace(template))
}

func (s *Service) buildCodexModeCommand(baseCmd, prompt string, mode model.RunMode, issueID string) (string, error) {
	baseCmd = strings.TrimSpace(baseCmd)
	if baseCmd == "" {
		return "", errors.New("codex command is empty")
	}

	cmd := baseCmd
	switch mode {
	case model.RunModePlan:
		if !hasCLIFlag(cmd, "--sandbox") {
			cmd += " --sandbox workspace-write"
		}
		if !hasCLIFlag(cmd, "--ask-for-approval") {
			cmd += " --ask-for-approval on-request"
		}
	case model.RunModeSelfRun, model.RunModeChaos:
		if !hasCLIFlag(cmd, "--dangerously-bypass-approvals-and-sandbox") {
			cmd += " --dangerously-bypass-approvals-and-sandbox"
		}
	default:
		return "", fmt.Errorf("unsupported run mode: %s", mode)
	}

	promptArg := shellQuote(prompt)
	if promptPath, err := promptx.WritePromptFile(s.repoRoot, issueID+"-"+string(mode), prompt); err == nil {
		snippet := promptx.BuildReadAndDeleteSnippet(promptPath)
		return fmt.Sprintf(`%s; %s "%s"`, snippet, cmd, "$BMUX_PROMPT_CONTENT"), nil
	}
	return cmd + " " + promptArg, nil
}

func stringsToSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}

func setToSortedSlice(m map[string]struct{}) []string {
	if len(m) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
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

const defaultModeChaosPromptTemplate = `You are running in bmux CHAOS mode for one task from a queue.

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

var lookPath = exec.LookPath
