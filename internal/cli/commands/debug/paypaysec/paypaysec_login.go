package paypaysec

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
)

func loginCommand() *cli.Command {
	return &cli.Command{
		Name: "login",
		Usage: "log in and leave the session in the profile, so the balance step " +
			"can be re-run without another one-time code",
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
	if !result.OTPRequired {
		_, _ = fmt.Fprintf(os.Stderr, "  no OTP challenge (the site recognised this device)\n")
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "  OTP challenge completed (page prefix %q)\n", result.OTPPrefix)
	return nil
}
