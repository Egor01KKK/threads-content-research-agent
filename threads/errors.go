package threads

import "fmt"

// Exit codes, mapped to process exit status by cmd/th.
const (
	ExitOK        = 0
	ExitGeneric   = 1
	ExitUsage     = 2
	ExitNotFound  = 3
	ExitLoginWall = 4
	ExitRateLimit = 5
	ExitNetwork   = 6
)

// CodeError carries an exit code alongside a message so main can map a library
// failure to the documented exit-code table.
type CodeError struct {
	Code int
	Msg  string
	Err  error
}

func (e *CodeError) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *CodeError) Unwrap() error { return e.Err }

func codeErr(code int, format string, args ...any) *CodeError {
	return &CodeError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Code returns the exit code an error maps to: a CodeError's own code, or the
// generic code for anything else. A nil error is ExitOK.
func Code(err error) int {
	if err == nil {
		return ExitOK
	}
	if ce, ok := err.(*CodeError); ok {
		return ce.Code
	}
	return ExitGeneric
}

// Usagef builds a usage CodeError (exit 2) for command-line misuse such as a
// wrong argument count or an unknown flag.
func Usagef(format string, args ...any) *CodeError {
	return codeErr(ExitUsage, format, args...)
}

func errLoginWall() *CodeError {
	return codeErr(ExitLoginWall, "login wall: Threads does not expose this content to anonymous crawlers; a supplied --session/--csrf pair may add access to your own account")
}

func errNotFound(what string) *CodeError {
	return codeErr(ExitNotFound, "not found: %s", what)
}
