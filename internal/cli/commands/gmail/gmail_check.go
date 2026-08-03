package gmail

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/credentials"
	gmailapi "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/gmail"
)

func checkCommand() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "confirm the credentials work and report which mailbox they open",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runCheck(ctx)
		},
	}
}

func runCheck(ctx context.Context) error {
	client, err := credentials.OpenMailbox(ctx, credentials.DefaultCredentialsFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	addr, err := client.Profile(ctx)
	if err != nil {
		// A scope error here is the common first failure, and the message Google
		// returns does not say which scope is missing.
		_, _ = fmt.Fprintf(os.Stderr,
			"\nIf that mentions scopes, the credential was minted without %s.\n"+
				"See SETUP.md: the gcloud default OAuth client cannot grant it, so a\n"+
				"Desktop client from your own project is needed.\n", gmailapi.Scope)
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "✓ credentials work — mailbox %s\n", addr)
	return nil
}
