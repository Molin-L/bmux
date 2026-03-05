package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

type BeadsClient interface {
	Ready(ctx context.Context) ([]model.Issue, error)
	List(ctx context.Context, filters map[string]string) ([]model.Issue, error)
	Show(ctx context.Context, issueID string) (model.Issue, error)
	Dependencies(ctx context.Context, issueID string) ([]model.Dependency, error)
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

type Options struct {
	RepoRoot      string
	WorktreeDir   string
	Store         *state.Store
	Beads         BeadsClient
	Git           GitClient
	Planner       BranchPlanner
	PromptBuilder PRPromptBuilder
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

type Service struct {
	repoRoot      string
	worktreeDir   string
	store         *state.Store
	beads         BeadsClient
	git           GitClient
	planner       BranchPlanner
	promptBuilder PRPromptBuilder
}

func NewService(opts Options) *Service {
	return &Service{
		repoRoot:      opts.RepoRoot,
		worktreeDir:   opts.WorktreeDir,
		store:         opts.Store,
		beads:         opts.Beads,
		git:           opts.Git,
		planner:       opts.Planner,
		promptBuilder: opts.PromptBuilder,
	}
}

func (s *Service) ReadyIssues(ctx context.Context) ([]model.Issue, error) {
	return s.beads.Ready(ctx)
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
