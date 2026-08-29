package manulife

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	mlsite "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife"
)

func loginCommand() *cli.Command {
	return &cli.Command{
		Name:  "login",
		Usage: "log in and leave the session in the profile",
		Description: "With --otp file the code is read from <debug-dir>/otp.txt instead of\n" +
			"the mailbox, so a failure here can only be the browser.\n\n" +
			"Every attempt costs a one-time code, and services stop sending those after\n" +
			"a handful in quick succession — so a failure dumps the page it died on\n" +
			"rather than leaving the next attempt to find out the same thing again.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runLogin(ctx, session.From(cmd))
		},
	}
}

// runLogin authenticates and leaves the session in the persisted profile.
func runLogin(ctx context.Context, opts *session.Options) error {
	in, err := newSignIn(ctx, opts)
	if err != nil {
		return err
	}
	s, err := opts.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	_, _ = fmt.Fprintf(os.Stderr, "→ submitting credentials…\n")
	result, err := in.client.Login(s.Context(), in.codes)
	if err != nil {
		s.ReportStepFailure(err)
		return err
	}

	_, _ = fmt.Fprintf(os.Stderr, "\n✓ login OK\n")
	s.Report()
	s.SaveSession()
	report(result)
	return nil
}

// report says whether a code was needed.
func report(r mlsite.LoginResult) {
	if r.OTPRequired {
		_, _ = fmt.Fprintf(os.Stderr, "  OTP challenge completed\n")
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "  no OTP challenge (the site recognised this session)\n")
}
