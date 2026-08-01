package moneyforward

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
)

func probeCommand() *cli.Command {
	var url string
	return &cli.Command{
		Name:  "probe",
		Usage: "load one page with the persisted session and report what it offers",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "url",
				Usage:       "page to inspect; defaults to the manual account from MF_ASSET_ID",
				Destination: &url,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runProbe(ctx, session.From(cmd), url)
		},
	}
}

func runProbe(ctx context.Context, opts *session.Options, url string) error {
	if url == "" {
		var err error
		if url, err = accountURL(opts); err != nil {
			return err
		}
	}

	s, err := opts.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	if err := s.Open(url); err != nil {
		return err
	}
	s.Report()

	if err := s.ReportInteractive(); err != nil {
		return err
	}
	s.DumpPage("mf-probe")
	return nil
}
