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

	"github.com/Molin-L/bmux/internal/agentexec"
	"github.com/Molin-L/bmux/internal/beads"
	"github.com/Molin-L/bmux/internal/errorsx"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

type BeadsClient interface {
	Ready(ctx context.Context) ([]model.Issue, error)
	List(ctx context.Context, filters map[string]string) ([]model.Issue, error)
	Show(ctx context.Context, issueID string) (model.Issue, error)
	Dependencies(ctx context.Context, issueID string) ([]model.Dependency, error)
	CreateIssue(ctx context.Context, req model.CreateIssueRequest) (model.Issue, error)
	AddDependency(ctx context.Context, issueID, blockedByID, depType string) error
	Claim(ctx context.Context, issueID string) error
	Handoff(ctx context.Context, issueID string) error
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
	SplitPaneOnTargetWithCommand(ctx context.Context, direction, cwd, target, command string) (string, error)
	SendKeys(ctx context.Context, paneID, text string, enter bool) error
	CapturePane(ctx context.Context, paneID string, lines int) (string, error)
	CurrentPaneID(ctx context.Context) (string, error)
	ListPanes(ctx context.Context, target string) ([]string, error)
	SetWindowOptionsForSidebar(ctx context.Context, target string, controlWidth int) error
	SelectLayout(ctx context.Context, target, layout string) error
	SelectLayoutMainVertical(ctx context.Context, target string) error
	SetBuffer(ctx context.Context, bufferName, content string) error
	PasteBuffer(ctx context.Context, bufferName, paneID string) error
	DeleteBuffer(ctx context.Context, bufferName string) error
	GetPaneCurrentCommand(ctx context.Context, paneID string) (string, error)
	GetWindowDimensions(ctx context.Context) (int, int, error)
	GetTerminalDimensions(ctx context.Context) (int, int, error)
	SetPaneBorderStatus(ctx context.Context, target, status string) error
	SetWindowSizeManual(ctx context.Context, target string, width, height int) error
	SetPaneTitle(ctx context.Context, paneID, title string) error
	GetPaneTitle(ctx context.Context, paneID string) (string, error)
	KillPane(ctx context.Context, paneID string) error
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
	MinPaneWidth           int
	MaxPaneWidth           int
	ApeMaxParallel         int
	ExecutionPlanPrompt    string
	ExecutionSelfRunPrompt string
	ExecutionApePrompt     string
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

type MergeStepResult struct {
	Name         string
	SourceBranch string
	TargetBranch string
	RepoPath     string
	BeforeCommit string
	AfterCommit  string
	Message      string
	Success      bool
}

type MergeTaskResult struct {
	IssueID string
	Meta    model.TaskBranchMeta
	Steps   []MergeStepResult
}

func (r MergeTaskResult) DetailLines() []string {
	lines := make([]string, 0, len(r.Steps))
	for _, step := range r.Steps {
		state := "ok"
		if !step.Success {
			state = "failed"
		}
		line := fmt.Sprintf("%s: %s -> %s (%s -> %s) [%s]",
			step.Name,
			trimOrDash(step.SourceBranch),
			trimOrDash(step.TargetBranch),
			shortCommit(step.BeforeCommit),
			shortCommit(step.AfterCommit),
			state,
		)
		if msg := strings.TrimSpace(step.Message); msg != "" {
			line = fmt.Sprintf("%s - %s", line, msg)
		}
		lines = append(lines, line)
	}
	return lines
}

type MergeConflictError struct {
	Stage        string
	SourceBranch string
	TargetBranch string
	RepoPath     string
	StdErr       string
	Err          error
}

func (e *MergeConflictError) Error() string {
	msg := strings.TrimSpace(e.StdErr)
	if msg == "" && e.Err != nil {
		msg = strings.TrimSpace(e.Err.Error())
	}
	if msg == "" {
		msg = "merge conflict detected"
	}
	return fmt.Sprintf("merge conflict at %s (%s -> %s): %s", trimOrDash(e.Stage), trimOrDash(e.SourceBranch), trimOrDash(e.TargetBranch), msg)
}

func (e *MergeConflictError) Unwrap() error {
	return e.Err
}

type Service struct {
	repoRoot      string
	worktreeDir   string
	store         *state.Store
	beads         BeadsClient
	git           GitClient
	planner       BranchPlanner
	promptBuilder PRPromptBuilder
	tmux          TmuxClient
	executor      *agentexec.Executor
}

type RunSummary struct {
	Running   int
	Launched  int
	Finished  int
	ActiveApe bool
}

func NewService(opts Options) *Service {
	svc := &Service{
		repoRoot:      opts.RepoRoot,
		worktreeDir:   opts.WorktreeDir,
		store:         opts.Store,
		beads:         opts.Beads,
		git:           opts.Git,
		planner:       opts.Planner,
		promptBuilder: opts.PromptBuilder,
		tmux:          opts.Tmux,
	}
	svc.executor = agentexec.New(agentexec.Options{
		RepoRoot:               opts.RepoRoot,
		Store:                  opts.Store,
		Beads:                  opts.Beads,
		Tmux:                   opts.Tmux,
		SplitDirection:         opts.SplitDirection,
		ClaudeCommand:          opts.ClaudeCommand,
		CodexCommand:           opts.CodexCommand,
		PromptTemplate:         opts.PromptTemplate,
		TmuxLayout:             opts.TmuxLayout,
		ControlWidth:           opts.ControlWidth,
		MinPaneWidth:           opts.MinPaneWidth,
		MaxPaneWidth:           opts.MaxPaneWidth,
		ApeMaxParallel:         opts.ApeMaxParallel,
		ExecutionPlanPrompt:    opts.ExecutionPlanPrompt,
		ExecutionSelfRunPrompt: opts.ExecutionSelfRunPrompt,
		ExecutionApePrompt:     opts.ExecutionApePrompt,
		OpenTask: func(ctx context.Context, issueID string) (model.TaskBranchMeta, error) {
			return svc.OpenTask(ctx, issueID)
		},
		ReadyIssues: func(ctx context.Context) ([]model.Issue, agentexec.IssueSourceState, error) {
			issues, state, err := svc.ReadyIssuesState(ctx)
			return issues, agentexec.IssueSourceState{
				Available: state.Available,
				Reason:    state.Reason,
			}, err
		},
	})
	return svc
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

func (s *Service) ListIssuesState(ctx context.Context) ([]model.Issue, IssueSourceState, error) {
	issues, err := s.beads.List(ctx, map[string]string{"all": "true", "limit": "0"})
	if err == nil {
		return s.enrichIssueHierarchy(ctx, issues), IssueSourceState{Available: true}, nil
	}
	if reason, unavailable := beads.BDUnavailableReason(err); unavailable {
		return []model.Issue{}, IssueSourceState{Available: false, Reason: reason}, nil
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

func (s *Service) MergeTask(ctx context.Context, issueID string, cleanup bool) (MergeTaskResult, error) {
	issueID = strings.TrimSpace(issueID)
	meta, ok, err := s.store.ByIssueID(issueID)
	if err != nil {
		return MergeTaskResult{}, err
	}
	if !ok {
		return MergeTaskResult{}, fmt.Errorf("issue %s not found in bmux state", issueID)
	}

	result := MergeTaskResult{
		IssueID: issueID,
		Meta:    meta,
		Steps:   []MergeStepResult{},
	}
	addStep := func(step MergeStepResult) {
		result.Steps = append(result.Steps, step)
	}
	readHead := func(path string) (string, string, error) {
		branch, commit, headErr := s.git.CurrentHead(ctx, path)
		if headErr != nil {
			return "", "", headErr
		}
		return strings.TrimSpace(branch), strings.TrimSpace(commit), nil
	}
	mergeStep := func(name, path, sourceBranch, targetBranch string) error {
		beforeBranch, beforeCommit, beforeErr := readHead(path)
		step := MergeStepResult{
			Name:         name,
			SourceBranch: sourceBranch,
			TargetBranch: targetBranch,
			RepoPath:     path,
			BeforeCommit: beforeCommit,
			AfterCommit:  beforeCommit,
			Success:      false,
		}
		if beforeErr != nil {
			step.Message = fmt.Sprintf("failed to read HEAD before merge: %v", beforeErr)
			addStep(step)
			return beforeErr
		}
		if strings.TrimSpace(step.TargetBranch) == "" {
			step.TargetBranch = beforeBranch
		}
		if strings.TrimSpace(step.SourceBranch) == "" {
			step.SourceBranch = sourceBranch
		}
		if mergeErr := s.git.Merge(ctx, path, sourceBranch); mergeErr != nil {
			_, afterCommit, _ := readHead(path)
			if strings.TrimSpace(afterCommit) != "" {
				step.AfterCommit = afterCommit
			}
			step.Message = strings.TrimSpace(commandErrSummary(mergeErr))
			addStep(step)
			if isMergeConflictError(mergeErr) {
				return &MergeConflictError{
					Stage:        name,
					SourceBranch: sourceBranch,
					TargetBranch: step.TargetBranch,
					RepoPath:     path,
					StdErr:       commandErrSummary(mergeErr),
					Err:          mergeErr,
				}
			}
			return mergeErr
		}
		_, afterCommit, afterErr := readHead(path)
		if afterErr == nil && strings.TrimSpace(afterCommit) != "" {
			step.AfterCommit = afterCommit
		}
		step.Success = true
		step.Message = fmt.Sprintf("merged %s into %s", trimOrDash(step.SourceBranch), trimOrDash(step.TargetBranch))
		addStep(step)
		return nil
	}

	if err := mergeStep("Step 1: worktree merge", meta.WorktreePath, meta.BaseBranch, meta.Branch); err != nil {
		return result, err
	}
	if err := mergeStep("Step 2: repo merge", s.repoRoot, meta.Branch, meta.BaseBranch); err != nil {
		return result, err
	}

	closeStep := MergeStepResult{
		Name:         "Step 3: close issue",
		SourceBranch: issueID,
		TargetBranch: "bd",
		Success:      false,
	}
	_, closeBeforeCommit, _ := readHead(s.repoRoot)
	closeStep.BeforeCommit = closeBeforeCommit
	closeStep.AfterCommit = closeBeforeCommit
	closeStep.Message = "bd close issue"

	if err := s.beads.Close(ctx, issueID, "Completed via bmux"); err != nil {
		closeStep.Message = fmt.Sprintf("failed: %s", strings.TrimSpace(commandErrSummary(err)))
		addStep(closeStep)
		meta.Status = "merged_pending_close"
		if upsertErr := s.store.Upsert(meta); upsertErr != nil {
			return result, fmt.Errorf("close issue after merge: %v (persist status: %w)", err, upsertErr)
		}
		result.Meta = meta
		return result, err
	}
	_, closeAfterCommit, _ := readHead(s.repoRoot)
	if strings.TrimSpace(closeAfterCommit) != "" {
		closeStep.AfterCommit = closeAfterCommit
	}
	closeStep.Success = true
	closeStep.Message = "closed issue in bd (Completed via bmux)"
	addStep(closeStep)

	if cleanup {
		shared, err := s.hasOtherActiveRefs(issueID, meta.Branch, meta.WorktreePath)
		if err != nil {
			return result, err
		}
		if !shared {
			if err := s.git.RemoveWorktree(ctx, s.repoRoot, meta.WorktreePath); err != nil {
				return result, err
			}
			if err := s.git.DeleteBranch(ctx, s.repoRoot, meta.Branch); err != nil {
				return result, err
			}
			addStep(MergeStepResult{
				Name:         "Step 4: cleanup",
				SourceBranch: meta.Branch,
				TargetBranch: meta.BaseBranch,
				BeforeCommit: closeStep.AfterCommit,
				AfterCommit:  closeStep.AfterCommit,
				Success:      true,
				Message:      fmt.Sprintf("removed worktree %s and deleted branch %s", meta.WorktreePath, meta.Branch),
			})
		} else {
			addStep(MergeStepResult{
				Name:         "Step 4: cleanup",
				SourceBranch: meta.Branch,
				TargetBranch: meta.BaseBranch,
				BeforeCommit: closeStep.AfterCommit,
				AfterCommit:  closeStep.AfterCommit,
				Success:      true,
				Message:      "skipped cleanup because branch/worktree is shared by another active issue",
			})
		}
	}

	meta.Status = "merged"
	if err := s.store.Upsert(meta); err != nil {
		return result, err
	}
	result.Meta = meta
	return result, nil
}

func (s *Service) CreateMergeConflictTask(ctx context.Context, issueID string, conflict *MergeConflictError) (model.Issue, error) {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return model.Issue{}, errors.New("issue id is required")
	}
	if conflict == nil {
		return model.Issue{}, errors.New("merge conflict context is required")
	}

	title := fmt.Sprintf("Resolve merge conflict for %s", issueID)
	description := buildMergeConflictIssueDescription(issueID, conflict)
	req := model.CreateIssueRequest{
		Title:       title,
		Description: description,
		Type:        "chore",
		Priority:    0,
	}
	created, err := s.beads.CreateIssue(ctx, req)
	if err != nil {
		return model.Issue{}, fmt.Errorf("create merge conflict issue: %w\nManual recovery:\n%s", err, mergeConflictManualRecoveryHint(issueID, title, description, "bd-new-conflict-id"))
	}
	createdID := strings.TrimSpace(created.ID)
	if createdID == "" {
		return model.Issue{}, fmt.Errorf("create merge conflict issue returned empty id\nManual recovery:\n%s", mergeConflictManualRecoveryHint(issueID, title, description, "bd-new-conflict-id"))
	}
	if err := s.beads.AddDependency(ctx, issueID, createdID, "blocks"); err != nil {
		return model.Issue{}, fmt.Errorf("link merge conflict issue: %w\nManual recovery:\n%s", err, mergeConflictManualRecoveryHint(issueID, title, description, createdID))
	}
	return created, nil
}

func (s *Service) hasOtherActiveRefs(issueID, branch, worktreePath string) (bool, error) {
	branch = strings.TrimSpace(branch)
	worktreePath = strings.TrimSpace(worktreePath)
	if branch == "" && worktreePath == "" {
		return false, nil
	}

	metas, err := s.store.All()
	if err != nil {
		return false, err
	}
	for _, meta := range metas {
		if strings.TrimSpace(meta.IssueID) == strings.TrimSpace(issueID) {
			continue
		}
		if strings.TrimSpace(meta.Status) != "active" {
			continue
		}
		if branch != "" && strings.TrimSpace(meta.Branch) == branch {
			return true, nil
		}
		if worktreePath != "" && strings.TrimSpace(meta.WorktreePath) == worktreePath {
			return true, nil
		}
	}
	return false, nil
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
		IssueID:          meta.IssueID,
		Mode:             meta.Mode,
		PaneID:           meta.PaneID,
		Running:          true,
		Pending:          meta.Pending,
		BlockedByIssueID: meta.BlockedByIssueID,
		Agent:            meta.Agent,
		StartedAt:        meta.StartedAt,
	}, true, nil
}

func (s *Service) GetTaskRunMeta(issueID string) (model.TaskRunMeta, bool, error) {
	return s.store.RunLockByIssueID(issueID)
}

func (s *Service) GetApeState() (model.ApeState, bool, error) {
	return s.store.ApeState()
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
	if ape, ok, err := s.store.ApeState(); err == nil && ok && ape.Active {
		sum.ActiveApe = true
		sum.Launched = len(dedupeStrings(ape.LaunchedIssueIDs))
		sum.Finished = len(dedupeStrings(ape.FinishedIssueIDs))
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

		deps := issue.Dependencies
		blockerID, ok := firstBlocksBlocker(issueID, deps, issueSet)
		if !ok {
			if fetched, depErr := s.beads.Dependencies(ctx, issueID); depErr == nil {
				blockerID, ok = firstBlocksBlocker(issueID, fetched, issueSet)
			}
		}
		if ok {
			blockedBy[issueID] = blockerID
		}
	}
	return blockedBy, nil
}

func firstBlocksBlocker(issueID string, deps []model.Dependency, issueSet map[string]struct{}) (string, bool) {
	issueID = strings.TrimSpace(issueID)
	for _, dep := range deps {
		if !strings.EqualFold(strings.TrimSpace(dep.Type), "blocks") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(dep.Direction), "incoming") {
			continue
		}

		candidates := []string{
			strings.TrimSpace(dep.IssueID),
			strings.TrimSpace(dep.TargetID),
		}
		if strings.TrimSpace(dep.IssueID) == issueID {
			candidates = append([]string{strings.TrimSpace(dep.TargetID)}, candidates...)
		}

		for _, blockerID := range dedupeStrings(candidates) {
			if blockerID == "" || blockerID == issueID {
				continue
			}
			if _, ok := issueSet[blockerID]; ok {
				return blockerID, true
			}
		}
	}
	return "", false
}

func (s *Service) StartTaskMode(ctx context.Context, issueID string, mode model.RunMode) (model.TaskRunMeta, error) {
	return s.executor.StartTaskMode(ctx, issueID, mode)
}

func (s *Service) StartTaskModeWaiting(ctx context.Context, issueID string, mode model.RunMode, blockerID string) (model.TaskRunMeta, error) {
	return s.executor.StartTaskModeWaiting(ctx, issueID, mode, blockerID)
}

func (s *Service) HandoffIssue(ctx context.Context, issueID string) error {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return errors.New("issue id is required")
	}
	return s.beads.Handoff(ctx, issueID)
}

func (s *Service) StartApe(ctx context.Context) (string, error) {
	return s.executor.StartApe(ctx)
}

func (s *Service) TickApe(ctx context.Context) (string, bool, error) {
	return s.executor.TickApe(ctx)
}

func (s *Service) ReconcileRunLocks(ctx context.Context) error {
	return s.executor.ReconcileRunLocks(ctx)
}

func (s *Service) PromotePendingRuns(ctx context.Context) (int, int, error) {
	return s.executor.PromotePendingRuns(ctx)
}

func (s *Service) ReconcileRuns(ctx context.Context) error {
	return s.executor.ReconcileRuns(ctx)
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
	return s.executor.StartPlanningPane(ctx, agent)
}

func commandErrSummary(err error) string {
	if err == nil {
		return ""
	}
	var cmdErr *errorsx.CommandError
	if errors.As(err, &cmdErr) && strings.TrimSpace(cmdErr.StdErr) != "" {
		return strings.TrimSpace(cmdErr.StdErr)
	}
	return strings.TrimSpace(err.Error())
}

func isMergeConflictError(err error) bool {
	lower := strings.ToLower(commandErrSummary(err))
	if lower == "" {
		return false
	}
	patterns := []string{
		"conflict",
		"automatic merge failed",
		"fix conflicts",
		"merge conflict",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func buildMergeConflictIssueDescription(issueID string, conflict *MergeConflictError) string {
	stderr := strings.ReplaceAll(strings.TrimSpace(conflict.StdErr), "\n", " | ")
	if stderr == "" && conflict.Err != nil {
		stderr = strings.ReplaceAll(strings.TrimSpace(conflict.Err.Error()), "\n", " | ")
	}
	if len(stderr) > 400 {
		stderr = stderr[:400] + "..."
	}
	lines := []string{
		fmt.Sprintf("Resolve merge conflict for %s.", issueID),
		fmt.Sprintf("Stage: %s", trimOrDash(conflict.Stage)),
		fmt.Sprintf("Merge: %s -> %s", trimOrDash(conflict.SourceBranch), trimOrDash(conflict.TargetBranch)),
		fmt.Sprintf("Repo path: %s", trimOrDash(conflict.RepoPath)),
		fmt.Sprintf("Error: %s", trimOrDash(stderr)),
		"",
		"After resolving conflicts, complete merge and close the source issue.",
	}
	return strings.Join(lines, "\n")
}

func mergeConflictManualRecoveryHint(issueID, title, description, conflictIssueID string) string {
	if strings.TrimSpace(conflictIssueID) == "" {
		conflictIssueID = "bd-new-conflict-id"
	}
	createCmd := fmt.Sprintf("bd create --title %s --description %s --type chore --priority P0 --json", shellQuote(title), shellQuote(description))
	depCmd := fmt.Sprintf("bd dep add %s %s --type blocks --json", shellQuote(issueID), shellQuote(conflictIssueID))
	return createCmd + "\n" + depCmd
}

func shortCommit(hash string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "-"
	}
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func trimOrDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
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
