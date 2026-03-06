package beads

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Molin-L/bmux/internal/errorsx"
)

var claimedByPattern = regexp.MustCompile(`(?i)already claimed by\s+(.+)$`)

func ResolveActor(ctx context.Context, repoRoot string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("BD_ACTOR")); v != "" {
		return v, nil
	}

	cmd := exec.CommandContext(ctx, "git", "config", "user.name")
	if strings.TrimSpace(repoRoot) != "" {
		cmd.Dir = repoRoot
	}
	out, err := cmd.Output()
	if err == nil {
		if v := strings.TrimSpace(string(out)); v != "" {
			return v, nil
		}
	}

	if v := strings.TrimSpace(os.Getenv("USER")); v != "" {
		return v, nil
	}

	return "", errors.New("unable to resolve actor (checked BD_ACTOR, git user.name, USER)")
}

func ClaimedByFromError(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	var cmdErr *errorsx.CommandError
	if errors.As(err, &cmdErr) {
		if who, ok := claimedByFromText(cmdErr.StdErr); ok {
			return who, true
		}
	}

	return claimedByFromText(err.Error())
}

func claimedByFromText(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		m := claimedByPattern.FindStringSubmatch(trimmed)
		if len(m) < 2 {
			continue
		}
		who := strings.TrimSpace(m[1])
		if who == "" {
			continue
		}
		return who, true
	}
	return "", false
}
