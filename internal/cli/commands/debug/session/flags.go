// Package session holds what every debug subcommand needs: the flags, a
// browser bound to a persisted profile, and the choice of where a one-time code
// comes from.
//
// Its own package because these were the most collision-prone names in the
// command layer — start, finish, report, navigate, mustEnv — sitting in the same
// scope as every subcommand's own helpers.
package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/credentials"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/otp"
)

// defaultDebugDir holds everything the debug commands write: the persisted
// Chrome profile, page dumps, a captured session and any handed-over one-time
// code. Gitignored, because all of that is personal data.
const defaultDebugDir = ".debug"

// Options are the flags shared by every debug subcommand, as one invocation
// parsed them.
//
// Built by [From] from the command urfave already hands to an Action, rather
// than bound into a long-lived struct with Destination and captured by every
// subcommand's closure. That struct was one piece of mutable state shared by
// the whole tree, written during parsing and read afterwards — and the same
// shape one level down had two subcommands sharing a --subclass destination,
// harmless only because one runs at a time.
//
// Not on the context either. A context value is for request-scoped data
// crossing an API boundary; this is a dependency, and putting it there trades a
// compile-time one for a type assertion that can fail.
type Options struct {
	debugDir string
	profile  string
	headless bool
	keepOpen bool
	otpMode  string
	timeout  time.Duration
}

// How the one-time code is obtained.
const (
	otpModeAuto  = "auto"
	otpModeFile  = "file"
	otpModeGmail = "gmail"
)

// Flags declares the shared flag set.
//
// No Destination: the values are read back through [From], from the command
// that parsed them.
func Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "debug-dir",
			Usage: "directory for page dumps and the Chrome profile (gitignored; holds personal data)",
			Value: defaultDebugDir,
		},
		&cli.StringFlag{
			Name:        "profile",
			Usage:       "Chrome profile directory, so a session survives between runs",
			DefaultText: "<debug-dir>/profile",
		},
		&cli.BoolFlag{
			Name:  "headless",
			Usage: "run Chrome without a window, as CI does",
		},
		&cli.BoolFlag{
			Name:  "keep-open",
			Usage: "when headed, wait for Enter before closing Chrome so DevTools can be used",
		},
		&cli.StringFlag{
			Name:  "otp",
			Usage: "where the one-time code comes from: auto, gmail, or file",
			Value: otpModeAuto,
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "overall deadline, generous enough to cover waiting for an OTP email",
			Value: 10 * time.Minute,
		},
	}
}

// From reads one invocation's flags off the command urfave parsed them into.
//
// cmd is whichever subcommand is running; urfave looks a flag up through the
// lineage, so the values declared on `debug` are visible to everything under
// it.
func From(cmd *cli.Command) *Options {
	return &Options{
		debugDir: cmd.String("debug-dir"),
		profile:  cmd.String("profile"),
		headless: cmd.Bool("headless"),
		keepOpen: cmd.Bool("keep-open"),
		otpMode:  cmd.String("otp"),
		timeout:  cmd.Duration("timeout"),
	}
}

// DebugDir is where this command writes page dumps and the Chrome profile.
func (o *Options) DebugDir() string { return o.debugDir }

// CookieFile is where a captured session is stored.
//
// A persisted Chrome profile is not enough on its own: the PayPay 証券 auth
// cookie is a session cookie and dies with the browser, so it has to be written
// out explicitly.
func (o *Options) CookieFile() string {
	return filepath.Join(o.debugDir, "cookies.json")
}

// ProfileDir resolves the Chrome profile location, defaulting inside the debug
// directory.
func (o *Options) ProfileDir() string {
	if o.profile != "" {
		return o.profile
	}
	return filepath.Join(o.debugDir, "profile")
}

// RequireEnv reports every missing variable at once, rather than one failed
// login attempt at a time.
func (o *Options) RequireEnv(keys ...string) error {
	if missing := config.Missing(keys...); len(missing) > 0 {
		return o.Missing(missing)
	}
	return nil
}

// Missing words the failure for someone at a terminal.
//
// The wording belongs here rather than in config, which is also read by the
// scheduled job, where the answer is a repository secret and not direnv.
func (o *Options) Missing(names []string) error {
	return fmt.Errorf("missing %v — export them, or put them in .envrc and run `direnv allow` "+
		"(see .envrc.example)", names)
}

// OTPSource picks how the one-time code will be supplied.
//
// Gmail is the real path and the one CI uses, so it is the default: developing
// against it is the only way to find out it works. The file source is the
// fallback for when credentials are missing, and the escape hatch for isolating
// a browser problem from a mail problem — with --otp file, a failure can only be
// the browser.
func (o *Options) OTPSource(ctx context.Context, spec otp.MailSpec) otp.Source {
	fileSource := &otp.File{
		Path: filepath.Join(o.debugDir, "otp.txt"),
		Spec: spec,
	}
	if o.otpMode == otpModeFile {
		return fileSource
	}

	mailbox, err := o.openMailbox(ctx)
	if err != nil {
		// Asking for Gmail explicitly and not getting it is worth a louder
		// message than falling back from auto, but either way the file source
		// still lets the run continue.
		if o.otpMode == otpModeGmail {
			_, _ = fmt.Fprintf(os.Stderr, "  ✗ Gmail unavailable: %v\n", err)
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "  (Gmail unavailable, falling back to the file source: %v)\n", err)
		}
		return fileSource
	}
	return &otp.Gmail{Mail: mailbox, Spec: spec}
}

// openMailbox opens the Gmail account OTP mail arrives in.
func (o *Options) openMailbox(ctx context.Context) (otp.MailSearcher, error) {
	return credentials.OpenMailbox(ctx, credentials.DefaultCredentialsFile)
}
