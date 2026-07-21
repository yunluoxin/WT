// Package errors defines the typed error hierarchy for wt.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors for branch points in business logic.
var (
	ErrNotARepo          = errors.New("not a git repository")
	ErrDetachedHEAD      = errors.New("detached HEAD")
	ErrWorktreeNotFound  = errors.New("worktree not found")
	ErrInvalidBranch     = errors.New("invalid branch")
	ErrMergeFailed       = errors.New("merge failed")
	ErrRebaseFailed      = errors.New("rebase failed")
	ErrHookFailed        = errors.New("hook failed")
	ErrConfig            = errors.New("config error")
	ErrSession           = errors.New("session error")
	ErrAborted           = errors.New("operation aborted")
	ErrAmbiguousTarget   = errors.New("ambiguous target")
	ErrProtectedWorktree = errors.New("cannot delete main repository worktree")
	ErrAIUnavailable     = errors.New("AI tool unavailable")
)

// Error is the base error type carrying a user-facing message.
type Error struct {
	Kind    error
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Kind }

// New creates a new Error wrapping a sentinel kind.
func New(kind error, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap creates a new Error with an underlying cause.
func Wrap(kind error, cause error, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...), Cause: cause}
}

// GitError describes a failed git subprocess invocation.
type GitError struct {
	Args     []string
	Dir      string
	ExitCode int
	Stderr   string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %v failed (exit %d): %s", e.Args, e.ExitCode, e.Stderr)
}

func (e *GitError) Unwrap() error { return ErrInvalidBranch }
