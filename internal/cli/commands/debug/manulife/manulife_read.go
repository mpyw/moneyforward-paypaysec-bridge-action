package manulife

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	mlsite "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife"
)

func readCommand() *cli.Command {
	return &cli.Command{
		Name:  "read",
		Usage: "log in, then read every contract in the list",
		Description: "Reads only. Prints the contracts, the labels each figure came\n" +
			"from, and whether the yen figure and the contract-currency one agree\n" +
			"once the page's own rate is applied.\n\n" +
			"Amounts are printed in full: this is a local harness for one person's\n" +
			"own account, and a masked figure would say nothing about whether the\n" +
			"read is right — which is the only question it exists to answer. The\n" +
			"scheduled job masks them; see the sync command.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runRead(ctx, session.From(cmd))
		},
	}
}

func runRead(ctx context.Context, opts *session.Options) error {
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
	if _, err := in.client.Login(s.Context(), in.codes); err != nil {
		s.ReportStepFailure(err)
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "✓ login OK\n")
	s.SaveSession()

	cards, err := mlsite.Cards(s.Context())
	if err != nil {
		s.ReportStepFailure(err)
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "\n%d contract(s) in the list:\n", len(cards))
	for _, card := range cards {
		number, nerr := card.Number()
		if nerr != nil {
			number = fmt.Sprintf("(%v)", nerr)
		}
		_, _ = fmt.Fprintf(os.Stderr, "   %-24s %-16s in-force=%v\n", card.Title, number, card.InForce())
	}

	// Every contract, including the ones that are not in force. Skipping them
	// here would hide the one thing worth checking about the status: that it
	// was read at all, and that it says what the screen says.
	for i, card := range cards {
		if i > 0 {
			// Reading a contract navigates away from the list, so the next one
			// starts by coming back. Not needed before the first, which is
			// where the browser already is.
			if err := mlsite.BackToList(s.Context()); err != nil {
				s.ReportStepFailure(err)
				return err
			}
		}
		if err := readOne(s, card); err != nil {
			return err
		}
	}
	return nil
}

// readOne opens one contract and reports what it yielded.
//
// A failure on one contract stops the run. This is a harness for finding out
// why a read is wrong, and carrying on past the first wrong thing buries it.
func readOne(s *session.Session, card mlsite.Card) error {
	_, _ = fmt.Fprintf(os.Stderr, "\n── %s ──\n", card.Title)

	reading, err := mlsite.ReadCard(s.Context(), card)
	if err != nil {
		s.ReportStepFailure(err)
		return err
	}

	_, _ = fmt.Fprintf(os.Stderr, "  種類-証券番号  %s\n", reading.Number)
	_, _ = fmt.Fprintf(os.Stderr, "  保険種類       %s\n", reading.PolicyType)
	_, _ = fmt.Fprintf(os.Stderr, "  円支払         %q → %d (read=%v)\n", reading.YenText, reading.Yen, reading.HasYen)
	_, _ = fmt.Fprintf(os.Stderr, "  契約通貨支払   %q → %d/100 (read=%v)\n", reading.FCYText, reading.FCYHundredths, reading.HasFCY)
	_, _ = fmt.Fprintf(os.Stderr, "  円換算レート   %q → %d/100 (read=%v)\n", reading.RateText, reading.RateHundredths, reading.HasRate)

	amount, err := reading.Amount()
	if err != nil {
		// Reported rather than returned: the figures above are what someone
		// needs in order to see why the two routes disagree, and stopping here
		// would print them and then hide the verdict.
		_, _ = fmt.Fprintf(os.Stderr, "  ✗ %v\n", err)
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "  ✓ %d 円 — the yen figure and the contract-currency one agree\n", amount)
	return nil
}
