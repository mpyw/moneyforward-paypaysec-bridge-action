package moneyforward

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/config"
	mfsite "github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward"
	mfsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward/selector"
)

func loginCommand() *cli.Command {
	return &cli.Command{
		Name:  "login",
		Usage: "log in and confirm the OTP selectors",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runLogin(ctx, session.From(cmd))
		},
	}
}

func runLogin(ctx context.Context, opts *session.Options) error {
	if err := opts.RequireEnv(string(secret.MoneyForwardEmail), string(secret.MoneyForwardPass)); err != nil {
		return err
	}

	s, err := opts.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	client := &mfsite.Client{
		Email:    config.Value(secret.MoneyForwardEmail),
		Password: config.Value(secret.MoneyForwardPass),
	}
	result, err := client.Login(s.Context(), opts.OTPSource(ctx, mfsel.OTPMail))
	if err != nil {
		s.ReportStepFailure(err)
		return err
	}

	_, _ = fmt.Fprintf(os.Stderr, "\n✓ login OK\n")
	s.Report()
	s.SaveSession()
	if result.AlreadyAuthenticated {
		_, _ = fmt.Fprintf(os.Stderr, "  the browser already had a live session; no credentials were sent\n")
		return nil
	}
	if !result.OTPRequired {
		_, _ = fmt.Fprintf(os.Stderr, "  no OTP challenge (the profile still held a valid session)\n")
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "\n  Confirmed — paste into internal/infra/moneyforward/selector:\n")
	_, _ = fmt.Fprintf(os.Stderr, "\tOTPInputCandidates  = `%s`\n", mfsel.OTPInputCandidates[result.OTPInputKey])
	if result.OTPSubmitKey != "" {
		_, _ = fmt.Fprintf(os.Stderr, "\tOTPSubmitCandidates = `%s`\n", mfsel.OTPSubmitCandidates[result.OTPSubmitKey])
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "\t(no submit button matched; the Enter fallback worked — keep it)\n")
	}
	return nil
}
