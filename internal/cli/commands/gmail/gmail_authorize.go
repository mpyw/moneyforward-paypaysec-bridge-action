package gmail

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/credentials"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
	gmailapi "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/gmail"
)

// DefaultClientFile is the OAuth client downloaded from the Google Cloud
// console — the "Desktop app" type, whose redirect is a loopback address.
const DefaultClientFile = "client_secret.json"

func authorizeCommand() *cli.Command {
	var clientFile, out string
	return &cli.Command{
		Name:  "authorize",
		Usage: "run the OAuth consent flow and save a Gmail credential",
		Description: "Deliberately not `gcloud auth application-default login`: gcloud insists on\n" +
			"issuing its credentials with cloud-platform, so one leaked out of CI would\n" +
			"authorize operating the whole Google Cloud project rather than reading mail.\n" +
			"See SETUP.md: the gcloud default OAuth client cannot grant it, so a\n" +
			"Desktop-app client of your own is what this takes.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "client",
				Usage:       "OAuth client JSON from the Google Cloud console (Desktop app)",
				Value:       DefaultClientFile,
				Destination: &clientFile,
			},
			&cli.StringFlag{
				Name:        "out",
				Usage:       "where to write the resulting credential",
				Value:       credentials.DefaultCredentialsFile,
				Destination: &out,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			return runAuthorize(ctx, clientFile, out)
		},
	}
}

func runAuthorize(ctx context.Context, client, out string) error {
	result, err := newAuthorize(clientFile(client), credentialFile(out)).Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stderr, "\n✓ credential written to %s (scope: %s)\n", out, gmailapi.Scope)
	if result.Mailbox != "" {
		_, _ = fmt.Fprintf(os.Stderr, "  it opens %s — check that is the account you meant\n", result.Mailbox)
	}
	_, _ = fmt.Fprintf(os.Stderr, "  ⚠ this file is a long-lived key to the mailbox — it is gitignored;\n"+
		"    upload it as the %s secret and keep it out of anywhere else.\n",
		config.GmailCredentials)
	return nil
}
