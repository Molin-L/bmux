package errorsx

import "fmt"

type CommandError struct {
	Command  string
	Args     []string
	Dir      string
	StdErr   string
	ExitCode int
	Err      error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("command failed: %s %v (dir=%s, exit=%d): %s", e.Command, e.Args, e.Dir, e.ExitCode, e.StdErr)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}
