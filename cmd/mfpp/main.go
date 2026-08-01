// Command mfpp syncs the PayPay 証券 balance into a MoneyForward manual asset.
//
// It is the single entry point for the project: the scheduled job, the one-time
// setup steps, and the development harness are all subcommands. Run `mfpp help`
// for the tree, or see internal/cli/commands, where the commands are defined.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands"
)

func main() {
	// Ctrl-C should close Chrome rather than orphan it, so every subcommand runs
	// under a cancellable context.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := commands.Root().Run(ctx, os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "\n✗ %v\n", err)
		os.Exit(1)
	}
}
