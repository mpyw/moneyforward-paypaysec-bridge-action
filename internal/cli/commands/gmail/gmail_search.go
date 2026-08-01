package gmail

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/credentials"
)

func searchCommand() *cli.Command {
	var query string
	var showBody bool
	return &cli.Command{
		Name:  "search",
		Usage: "list recent messages matching a Gmail query",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "query",
				Usage:       "Gmail search syntax, e.g. 'from:noreply@example.com newer_than:1d'",
				Required:    true,
				Destination: &query,
			},
			&cli.BoolFlag{
				Name: "body",
				Usage: "also print the message body — it contains the one-time code, " +
					"so only do this when you need to work out the pattern",
				Destination: &showBody,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSearch(ctx, query, showBody)
		},
	}
}

func runSearch(ctx context.Context, query string, showBody bool) error {
	client, err := credentials.OpenMailbox(ctx, credentials.DefaultCredentialsFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	msgs, err := client.Search(ctx, query, 10)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "no messages matched %q\n", query)
		return nil
	}

	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "RECEIVED\tFROM\tSUBJECT")
	for _, m := range msgs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", m.Received.Local().Format("2006-01-02 15:04:05"), m.From, m.Subject)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	if showBody {
		for _, m := range msgs {
			_, _ = fmt.Fprintf(os.Stderr, "\n──── %s ────\n%s\n", m.ID, m.Body)
		}
	}
	return nil
}
