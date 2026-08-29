// Package probe inspects a page nothing here has selectors for yet.
//
// The site commands have their own probes, and those read their site: they run
// its extraction routes and report what each one found. That is the right tool
// once a site package exists. Before one does — a service nobody here has ever
// driven — the only honest report is what the page itself offers, because every
// selector that could be reported on is the thing being looked for.
//
// So this one knows nothing about any site. It opens a URL, optionally hands
// the browser to whoever is at the terminal so they can sign in by hand, and
// then says where the browser ended up, what it offers to click or type into,
// and writes the page out.
//
// It lives beside the site groups rather than inside one because it belongs to
// none of them: the first thing it is ever pointed at is a service that has no
// group yet.
package probe

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
)

// Command builds the site-agnostic probe.
func Command() *cli.Command {
	var (
		url         string
		manual      bool
		saveSession bool
		captureNet  bool
		label       string
	)
	return &cli.Command{
		Name:  "probe",
		Usage: "open any URL and report what the page offers (knows no site)",
		Description: "Points at a service this program cannot drive yet.\n\n" +
			"With --manual it opens a window and captures the signed-out page, then\n" +
			"waits: sign in and navigate by hand, press Enter to capture wherever you\n" +
			"are, and repeat. Type q to finish. That loop exists because the sign-in\n" +
			"is the expensive part — one one-time code should buy every page worth\n" +
			"looking at, and which those are is rarely known in advance.\n\n" +
			"--save-session keeps the login for later runs, rewriting it as you go: a\n" +
			"session cookie dies with the browser, and a run abandoned at the terminal\n" +
			"would otherwise cost another code.\n\n" +
			"The dumps are the deliverable, and they hold whatever the pages held.\n" +
			"Delete the debug directory when you are done with it.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "url",
				Usage:       "page to open; omit to start where the saved session lands",
				Destination: &url,
			},
			&cli.BoolFlag{
				Name:        "manual",
				Usage:       "wait for you to sign in and navigate before inspecting anything",
				Destination: &manual,
			},
			&cli.BoolFlag{
				Name: "save-session",
				Usage: "keep writing the cookies out while you work, so the next run is " +
					"already signed in even if this one is abandoned",
				Destination: &saveSession,
			},
			&cli.BoolFlag{
				Name: "capture-network",
				Usage: "record the XHR/fetch calls the page makes — where the figures " +
					"actually come from, when the DOM offers no stable handle",
				Destination: &captureNet,
			},
			&cli.StringFlag{
				Name:        "label",
				Usage:       "names the page dump",
				Value:       "probe",
				Destination: &label,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return run(ctx, session.From(cmd), options{
				url:         url,
				manual:      manual,
				saveSession: saveSession,
				captureNet:  captureNet,
				label:       label,
			})
		},
	}
}

// options is one invocation's own flags, as distinct from the shared ones.
type options struct {
	url         string
	manual      bool
	saveSession bool
	captureNet  bool
	label       string
}

func run(ctx context.Context, shared *session.Options, o options) error {
	s, err := shared.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	// Started before the first navigation, because the calls that fill a page
	// are made while it loads and there is no second chance at them.
	if o.captureNet {
		stopRecording, err := s.RecordNetwork(o.label)
		if err != nil {
			return err
		}
		defer stopRecording()
	}

	if o.url != "" {
		if err := s.Open(o.url); err != nil {
			return err
		}
	}

	if !o.manual {
		return inspect(s, o.label)
	}

	// The sign-in form is only on the screen before anyone signs in, and it is
	// half of what this run is for: the login this service will eventually be
	// driven with has to be written against those selectors.
	if err := inspect(s, o.label+"-signed-out"); err != nil {
		return err
	}

	// Held for the whole manual stretch, captures included: what is being waited
	// for is a person, and a person can be called away. Stopping it waits for
	// the writer, so the run cannot exit through the middle of a write — see
	// [session.Session.HoldSession].
	stop := func() {}
	if o.saveSession {
		stop = s.HoldSession()
	}
	defer stop()

	// A loop rather than one capture, because a sign-in is the expensive part.
	// Both services already known here stop mailing one-time codes after about
	// five logins in quick succession, and there is no reason to expect a third
	// to be more generous — so one code should buy every page worth looking at,
	// not one of them. The pages that matter are rarely known in advance: the
	// list, whatever it links to, and whatever that turns out to need.
	for i := 1; ; i++ {
		typed, waited := s.Pause(fmt.Sprintf(
			"Navigate to a page worth capturing (capture %d), then press Enter here.\n"+
				"Type q and press Enter when there is nothing left to capture.", i))
		if !waited || strings.EqualFold(typed, "q") {
			return nil
		}
		if err := inspect(s, fmt.Sprintf("%s-%02d", o.label, i)); err != nil {
			return err
		}
	}
}

// inspect reports where the browser is and what the page offers, and writes the
// page out under label.
func inspect(s *session.Session, label string) error {
	s.Report()
	if err := s.ReportInteractive(); err != nil {
		return err
	}
	s.DumpPage(label)
	return nil
}
