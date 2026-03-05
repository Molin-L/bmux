package execx

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"

	"github.com/Molin-L/bmux/internal/errorsx"
)

const defaultTimeout = 30 * time.Second

type Runner struct {
	Timeout time.Duration
}

func New(timeout time.Duration) *Runner {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Runner{Timeout: timeout}
}

func (r *Runner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	if r == nil {
		r = New(defaultTimeout)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return out.String(), nil
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", context.DeadlineExceeded
	}

	exitCode := -1
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok {
			exitCode = status.ExitStatus()
		}
	}

	return "", &errorsx.CommandError{
		Command:  name,
		Args:     args,
		Dir:      dir,
		StdErr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}
