package paypaysec

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	ppsite "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
	ppsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

func probeCommand() *cli.Command {
	var url string
	return &cli.Command{
		Name:  "probe",
		Usage: "load one URL and report what it offers: extraction routes and interactive controls",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "url",
				Usage:       "page to inspect",
				Required:    true,
				Destination: &url,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runProbe(ctx, session.From(cmd), url)
		},
	}
}

// runProbe loads one page and reports everything that might identify the
// figures on it: each extraction route's result, and the interactive controls.
func runProbe(ctx context.Context, opts *session.Options, url string) error {
	s, err := opts.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	if err := s.Open(url); err != nil {
		return err
	}
	s.Report()

	reading, rerr := ppsite.Read(s.Context(), ppsel.Target{Key: "probe", URL: url})
	if rerr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "  read: %v\n", rerr)
	}
	_, _ = fmt.Fprintf(os.Stderr, "\nExtraction routes:\n")
	_, _ = fmt.Fprintf(os.Stderr, "  total        present=%v raw=%q yen=%d\n", reading.Figures.TotalPresent, reading.Figures.TotalRaw, reading.TotalYen)
	_, _ = fmt.Fprintf(os.Stderr, "  acquisition  present=%v raw=%q yen=%d\n", reading.Figures.AcquisitionPresent, reading.Figures.AcquisitionRaw, reading.AcquisitionYen)
	_, _ = fmt.Fprintf(os.Stderr, "  gain         present=%v raw=%q yen=%d\n", reading.Figures.GainPresent, reading.Figures.GainRaw, reading.GainYen)
	_, _ = fmt.Fprintf(os.Stderr, "  cells        count=%d parsed=%d sum=%d\n", reading.HoldingCount(), reading.HoldingsParsed, reading.HoldingsSumYen)
	for i, h := range reading.Holdings {
		_, _ = fmt.Fprintf(os.Stderr, "    [%02d] %-28q invest=%-14q gain=%-10q %s\n",
			i, h.Name, h.InvestText, h.GainText, h.Ref)
	}

	if err := s.ReportInteractive(); err != nil {
		return err
	}
	s.DumpPage("probe")
	return nil
}
