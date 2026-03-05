package bootstrap

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Molin-L/bmux/internal/config"
	"github.com/Molin-L/bmux/internal/execx"
	"github.com/Molin-L/bmux/internal/tmux"
)

type tmuxClient interface {
	HasSession(ctx context.Context, name string) (bool, error)
	NewSessionDetached(ctx context.Context, name, cwd, command string) error
	SendKeys(ctx context.Context, paneID, text string, enter bool) error
	AttachSession(ctx context.Context, name string) error
	SetWindowOptionsForSidebar(ctx context.Context, target string, controlWidth int) error
}

func EnsureTmuxSession(ctx context.Context, repoRoot, execPath string, args []string, tmuxCfg config.TmuxConfig) (bool, error) {
	client := tmux.NewClient(execx.New(0))
	return ensureTmuxSessionWithClient(ctx, repoRoot, execPath, args, tmuxCfg, client)
}

func ensureTmuxSessionWithClient(ctx context.Context, repoRoot, execPath string, args []string, tmuxCfg config.TmuxConfig, client tmuxClient) (bool, error) {
	if strings.TrimSpace(os.Getenv("TMUX")) != "" {
		return false, nil
	}
	if !tmuxCfg.AutoAttach {
		return false, nil
	}
	sessionName := buildSessionName(repoRoot, tmuxCfg.SessionPrefix)
	exists, err := client.HasSession(ctx, sessionName)
	if err != nil {
		return false, err
	}
	if !exists {
		if err := client.NewSessionDetached(ctx, sessionName, repoRoot, ""); err != nil {
			return false, err
		}
		command := buildCommandLine(execPath, args)
		if err := client.SendKeys(ctx, sessionName, command, true); err != nil {
			return false, err
		}
		if tmuxCfg.Layout == "sidebar" {
			_ = client.SetWindowOptionsForSidebar(ctx, sessionName, tmuxCfg.ControlPaneWidth)
		}
	}
	if err := client.AttachSession(ctx, sessionName); err != nil {
		return false, err
	}
	return true, nil
}

func buildSessionName(repoRoot, prefix string) string {
	if strings.TrimSpace(prefix) == "" {
		prefix = "bmux-"
	}
	base := filepath.Base(repoRoot)
	sum := md5.Sum([]byte(repoRoot))
	hash := hex.EncodeToString(sum[:])[:8]
	safeBase := sanitizeToken(base)
	return fmt.Sprintf("%s%s-%s", prefix, safeBase, hash)
}

func sanitizeToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "repo"
	}
	repl := strings.NewReplacer(" ", "-", "/", "-", "_", "-", ".", "-")
	s = repl.Replace(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func buildCommandLine(execPath string, args []string) string {
	all := make([]string, 0, len(args)+1)
	all = append(all, execPath)
	all = append(all, args...)
	escaped := make([]string, 0, len(all))
	for _, token := range all {
		escaped = append(escaped, shellQuote(token))
	}
	return strings.Join(escaped, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
