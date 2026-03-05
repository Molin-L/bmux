package bootstrap

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Molin-L/bmux/internal/config"
)

type fakeTmux struct {
	exists      bool
	hasErr      error
	newErr      error
	sendErr     error
	attachErr   error
	hasCalls    int
	newCalls    int
	sendCalls   int
	attachCalls int
	layoutCalls int
	lastSession string
	lastCommand string
	lastLayoutW int
}

func (f *fakeTmux) HasSession(context.Context, string) (bool, error) {
	f.hasCalls++
	return f.exists, f.hasErr
}

func (f *fakeTmux) NewSessionDetached(_ context.Context, name, _, _ string) error {
	f.newCalls++
	f.lastSession = name
	return f.newErr
}

func (f *fakeTmux) SendKeys(_ context.Context, _, text string, _ bool) error {
	f.sendCalls++
	f.lastCommand = text
	return f.sendErr
}

func (f *fakeTmux) AttachSession(_ context.Context, name string) error {
	f.attachCalls++
	f.lastSession = name
	return f.attachErr
}

func (f *fakeTmux) SetWindowOptionsForSidebar(_ context.Context, _ string, controlWidth int) error {
	f.layoutCalls++
	f.lastLayoutW = controlWidth
	return nil
}

func TestEnsureTmuxSessionInsideTmuxNoop(t *testing.T) {
	t.Setenv("TMUX", "1")
	cfg := config.TmuxConfig{AutoAttach: true, Layout: "sidebar", ControlPaneWidth: 40, SessionPrefix: "bmux-"}
	handled, err := ensureTmuxSessionWithClient(context.Background(), t.TempDir(), "/bin/bmux", nil, cfg, &fakeTmux{})
	if err != nil {
		t.Fatalf("ensure tmux session: %v", err)
	}
	if handled {
		t.Fatalf("handled = true, want false")
	}
}

func TestEnsureTmuxSessionExistingAttachOnly(t *testing.T) {
	t.Setenv("TMUX", "")
	cfg := config.TmuxConfig{AutoAttach: true, Layout: "sidebar", ControlPaneWidth: 40, SessionPrefix: "bmux-"}
	f := &fakeTmux{exists: true}
	handled, err := ensureTmuxSessionWithClient(context.Background(), t.TempDir(), "/bin/bmux", []string{"--flag"}, cfg, f)
	if err != nil {
		t.Fatalf("ensure tmux session: %v", err)
	}
	if !handled {
		t.Fatalf("handled = false, want true")
	}
	if f.newCalls != 0 || f.sendCalls != 0 || f.attachCalls != 1 {
		t.Fatalf("unexpected calls: new=%d send=%d attach=%d", f.newCalls, f.sendCalls, f.attachCalls)
	}
}

func TestEnsureTmuxSessionCreateAndAttach(t *testing.T) {
	t.Setenv("TMUX", "")
	cfg := config.TmuxConfig{AutoAttach: true, Layout: "sidebar", ControlPaneWidth: 50, SessionPrefix: "bmux-"}
	f := &fakeTmux{exists: false}
	repoRoot := t.TempDir()
	handled, err := ensureTmuxSessionWithClient(context.Background(), repoRoot, "/bin/bmux", []string{"--x", "a b"}, cfg, f)
	if err != nil {
		t.Fatalf("ensure tmux session: %v", err)
	}
	if !handled {
		t.Fatalf("handled = false, want true")
	}
	if f.newCalls != 1 || f.sendCalls != 1 || f.attachCalls != 1 {
		t.Fatalf("unexpected calls: new=%d send=%d attach=%d", f.newCalls, f.sendCalls, f.attachCalls)
	}
	if f.layoutCalls != 1 || f.lastLayoutW != 50 {
		t.Fatalf("layout calls=%d width=%d", f.layoutCalls, f.lastLayoutW)
	}
	if f.lastCommand == "" {
		t.Fatalf("expected command to be sent")
	}
}

func TestEnsureTmuxSessionPropagatesErrors(t *testing.T) {
	t.Setenv("TMUX", "")
	cfg := config.TmuxConfig{AutoAttach: true}
	f := &fakeTmux{hasErr: errors.New("boom")}
	_, err := ensureTmuxSessionWithClient(context.Background(), t.TempDir(), "/bin/bmux", nil, cfg, f)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildSessionName(t *testing.T) {
	name := buildSessionName("/tmp/My Repo", "bmux-")
	if name == "" || name[:5] != "bmux-" {
		t.Fatalf("name = %q", name)
	}
}

func TestShellQuote(t *testing.T) {
	out := shellQuote("a'b")
	if out != "'a'\\''b'" {
		t.Fatalf("quote = %q", out)
	}
}

func TestEnsureTmuxSessionAutoAttachFalse(t *testing.T) {
	t.Setenv("TMUX", "")
	cfg := config.TmuxConfig{AutoAttach: false}
	f := &fakeTmux{}
	handled, err := ensureTmuxSessionWithClient(context.Background(), t.TempDir(), "/bin/bmux", nil, cfg, f)
	if err != nil {
		t.Fatalf("ensure tmux session: %v", err)
	}
	if handled {
		t.Fatalf("handled = true, want false")
	}
}

func TestMainEnvironmentNotMutated(t *testing.T) {
	if os.Getenv("TMUX") == "__never__" {
		t.Fatalf("unexpected env value")
	}
}
