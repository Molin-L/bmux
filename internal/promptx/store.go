package promptx

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxSlugPrefixLength = 64

func init() {
	rand.Seed(time.Now().UnixNano())
}

func WritePromptFile(repoRoot, slug, prompt string) (string, error) {
	promptsDir := filepath.Join(repoRoot, ".bmux", "prompts")
	if err := os.MkdirAll(promptsDir, 0o700); err != nil {
		return "", fmt.Errorf("create prompt dir: %w", err)
	}

	safeSlug := sanitizeSlug(slug)
	filename := fmt.Sprintf("%s--%d-%s.txt", safeSlug, time.Now().UnixMilli(), randomSuffix())
	promptPath := filepath.Join(promptsDir, filename)

	if err := os.WriteFile(promptPath, []byte(prompt), 0o600); err != nil {
		return "", fmt.Errorf("write prompt file: %w", err)
	}
	return promptPath, nil
}

func BuildReadAndDeleteSnippet(promptPath string) string {
	quoted := ShellQuote(promptPath)
	return fmt.Sprintf(`BMUX_PROMPT_FILE=%s; BMUX_PROMPT_CONTENT="$(cat "$BMUX_PROMPT_FILE" 2>/dev/null || true)"; rm -f "$BMUX_PROMPT_FILE"`, quoted)
}

func ShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\\''`) + "'"
}

func sanitizeSlug(slug string) string {
	normalized := strings.ToLower(strings.TrimSpace(slug))
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-", ".", "-")
	normalized = replacer.Replace(normalized)

	var b strings.Builder
	lastDash := false
	for _, r := range normalized {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !valid {
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
			continue
		}
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		b.WriteRune(r)
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "task"
	}
	if len(out) > maxSlugPrefixLength {
		out = strings.Trim(out[:maxSlugPrefixLength], "-")
		if out == "" {
			out = "task"
		}
	}
	return out
}

func randomSuffix() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, 6)
	for i := range buf {
		buf[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(buf)
}
