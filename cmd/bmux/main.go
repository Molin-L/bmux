package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/bootstrap"
	"github.com/Molin-L/bmux/internal/config"
	"github.com/Molin-L/bmux/internal/execx"
	"github.com/Molin-L/bmux/internal/tui"
	"github.com/Molin-L/bmux/internal/waitpane"
)

type waitBlockedArgs struct {
	Enabled   bool
	IssueID   string
	BlockedBy string
	Branch    string
}

type mainDeps struct {
	runWaitPane func(opts waitpane.Options) error
	runDefault  func(args []string) error
}

func defaultMainDeps() mainDeps {
	return mainDeps{
		runWaitPane: waitpane.Run,
		runDefault:  runDefaultMain,
	}
}

func main() {
	if err := runMain(os.Args[1:], defaultMainDeps()); err != nil {
		fmt.Fprintln(os.Stderr, "bmux:", err)
		os.Exit(1)
	}
}

func runMain(args []string, deps mainDeps) error {
	if deps.runWaitPane == nil || deps.runDefault == nil {
		return errors.New("main dependencies are not configured")
	}
	waitArgs, err := parseWaitBlockedArgs(args)
	if err != nil {
		return err
	}
	if waitArgs.Enabled {
		return deps.runWaitPane(waitpane.Options{
			IssueID:   waitArgs.IssueID,
			BlockedBy: waitArgs.BlockedBy,
			Branch:    waitArgs.Branch,
		})
	}
	return deps.runDefault(args)
}

func runDefaultMain(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	runner := execx.New(0)
	repoRoot, err := runner.Run(context.Background(), cwd, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("not inside a git repository (run from repo root or any subdir): %w", err)
	}
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	cfg, err := config.Load(repoRoot, home)
	if err != nil {
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	handled, err := bootstrap.EnsureTmuxSession(context.Background(), repoRoot, execPath, args, cfg.Tmux)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	svc, _, err := app.NewDefaultService(repoRoot)
	if err != nil {
		return err
	}

	if err := tui.Run(svc); err != nil {
		return err
	}
	return nil
}

func parseWaitBlockedArgs(args []string) (waitBlockedArgs, error) {
	out := waitBlockedArgs{}
	for i := 0; i < len(args); i++ {
		token := strings.TrimSpace(args[i])
		if token == "" {
			continue
		}
		switch {
		case token == "--wait-blocked":
			out.Enabled = true
		case strings.HasPrefix(token, "--wait-blocked="):
			v := strings.TrimSpace(strings.TrimPrefix(token, "--wait-blocked="))
			out.Enabled = v == "" || strings.EqualFold(v, "true") || v == "1"
		case token == "--issue-id" || token == "--blocked-by" || token == "--blockedby" || token == "--branch":
			if i+1 >= len(args) {
				return out, fmt.Errorf("missing value for %s", token)
			}
			i++
			val := strings.TrimSpace(args[i])
			switch token {
			case "--issue-id":
				out.IssueID = val
			case "--blocked-by", "--blockedby":
				out.BlockedBy = val
			case "--branch":
				out.Branch = val
			}
		case strings.HasPrefix(token, "--issue-id="):
			out.IssueID = strings.TrimSpace(strings.TrimPrefix(token, "--issue-id="))
		case strings.HasPrefix(token, "--blocked-by="):
			out.BlockedBy = strings.TrimSpace(strings.TrimPrefix(token, "--blocked-by="))
		case strings.HasPrefix(token, "--blockedby="):
			out.BlockedBy = strings.TrimSpace(strings.TrimPrefix(token, "--blockedby="))
		case strings.HasPrefix(token, "--branch="):
			out.Branch = strings.TrimSpace(strings.TrimPrefix(token, "--branch="))
		default:
			if out.Enabled && strings.HasPrefix(token, "--") {
				return out, fmt.Errorf("unknown wait-blocked flag: %s", token)
			}
		}
	}
	if out.Enabled {
		if out.IssueID == "" {
			return out, errors.New("wait-blocked mode requires --issue-id")
		}
		if out.BlockedBy == "" {
			return out, errors.New("wait-blocked mode requires --blocked-by")
		}
	}
	return out, nil
}
