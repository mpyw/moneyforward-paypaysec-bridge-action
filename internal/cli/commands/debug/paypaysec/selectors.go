package paypaysec

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	ppsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

//declscope:package // registered by the parent command
func selectorsCommand() *cli.Command {
	return &cli.Command{
		Name:  "selectors",
		Usage: "check the login-page selectors (no credentials needed)",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSelectors(ctx, session.From(cmd))
		},
	}
}

// runSelectors verifies the login form without credentials.
// selectorsProbeTimeout is how long one selector gets to appear when checking
// that the confirmed ones still resolve.
const selectorsProbeTimeout = 15 * time.Second

func runSelectors(ctx context.Context, opts *session.Options) error {
	s, err := opts.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	if err := s.Open(ppsel.LoginURL); err != nil {
		return err
	}
	s.Report()

	for name, selector := range map[string]string{
		"SelectorMemberIDInput": ppsel.MemberIDInput,
		"SelectorPasswordInput": ppsel.PasswordInput,
		"SelectorLoginSubmit":   ppsel.LoginSubmit,
	} {
		if _, err := browser.PageOf(s.Context()).WaitForAny(selectorsProbeTimeout, map[string]string{name: selector}); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "  ✗ %-24s %s\n", name, selector)
			continue
		}
		_, _ = fmt.Fprintf(os.Stderr, "  ✓ %-24s %s\n", name, selector)
	}
	return nil
}
