// Command th is a single-binary command line for threads.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Egor01KKK/threads-content-research-agent/cli"
	"github.com/Egor01KKK/threads-content-research-agent/threads"
	"github.com/charmbracelet/fang"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.Root()
	// fang gives styled help, errors, and shell completion for free; the command
	// tree lives in the cli package, and a library CodeError maps to its exit code.
	if err := fang.Execute(ctx, root,
		fang.WithVersion(cli.Version),
		fang.WithNotifySignal(os.Interrupt, syscall.SIGTERM),
	); err != nil {
		var ce *threads.CodeError
		if errors.As(err, &ce) {
			os.Exit(ce.Code)
		}
		// Cobra reports an unknown command or flag as a plain error; map those to
		// the usage exit code so misuse is always 2, never the generic 1.
		if msg := err.Error(); strings.HasPrefix(msg, "unknown command") ||
			strings.HasPrefix(msg, "unknown flag") ||
			strings.HasPrefix(msg, "unknown shorthand flag") {
			os.Exit(threads.ExitUsage)
		}
		os.Exit(1)
	}
}
