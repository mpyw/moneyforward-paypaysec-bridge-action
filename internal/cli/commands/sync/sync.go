// Package sync is the scheduled job's command.
//
// What is left here after the work moved to the use case is what a command is
// actually for: reading configuration, wiring dependencies, and reporting.
package sync

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/secret"
)

const (
	// jobTimeout sits below the workflow's own limit, so the job fails with
	// these diagnostics rather than being killed by the runner.
	jobTimeout = 18 * time.Minute

	// otpTimeout caps the wait for a one-time code; otpInterval is the poll
	// cadence.
	otpTimeout  = 5 * time.Minute
	otpInterval = 5 * time.Second
)

// requiredEnv are the credentials the job cannot run without.
//
// From the domain rather than listed again here: this and `secrets setup` were
// two copies of one set, kept in step by nothing but care, and a credential in
// only one of them is a run that fails at whichever step first needs it.
//
// Deliberately not flags: a value on a command line is visible to anything that
// can read the process list.
var requiredEnv = secret.RequiredNames()

// Command builds the sync subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "read the PayPay 証券 holdings and record them on the MoneyForward account",
		Description: "Credentials come from the environment (GitHub Secrets in CI). They are\n" +
			"deliberately not flags: a value on the command line is visible in the\n" +
			"process list. Required: " + fmt.Sprint(requiredEnv) + ".",
		Action: func(ctx context.Context, _ *cli.Command) error {
			log.SetFlags(log.Ltime)
			return run(ctx)
		},
	}
}

func run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()

	// Everything this needs is assembled by wire, from the providers next door.
	// What is left here is the order of the two phases, which is the use case's
	// business, and the deadline, which is this command's.
	sync, cleanup, err := newSync(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	_, err = sync.Run(ctx)
	return err
}
