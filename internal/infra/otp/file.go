package otp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// File watches a path until it contains a fresh one-time code.
//
// The counterpart to [Gmail], for when the mailbox is not the thing under test.
// A run driven by a tool rather than a person has no terminal to prompt at, so
// the process waits here while whoever has the email writes the code from
// anywhere — another shell, an editor, a script. It is also the way to tell a
// browser fault from a mail fault: with this source, a failure can only be the
// browser.
//
// Freshness comes from the file's modification time. A code left over from an
// earlier attempt is still sitting on disk and looks identical to a new one, and
// submitting it fails the login in a way that reads like a wrong selector.
//
// The file is deleted once read. It is a credential, and a short-lived one.
type File struct {
	// Path is the file to watch.
	Path string

	// Spec identifies the service; only Label and Digits are used here.
	Spec MailSpec

	// Timeout caps the total wait; Interval is the poll cadence. Zero means the
	// package defaults.
	Timeout  time.Duration
	Interval time.Duration

	// Announce, if set, is called once with the instructions to show the user.
	// Defaults to writing them to stderr.
	Announce func(string)
}

// Describe names this source for log lines and failure messages.
func (f *File) Describe() string {
	if f.Spec.Label != "" {
		return "file " + f.Path + " (" + f.Spec.Label + ")"
	}
	return "file " + f.Path
}

// Fetch blocks until Path is written with a code newer than since.
func (f *File) Fetch(ctx context.Context, since time.Time) (string, error) {
	p := newPoller(f.Timeout, f.Interval, 2*time.Second)

	// The cutoff is stated rather than enforced: nothing here can tell whether
	// the digits someone types came from the newest mail or the one before it.
	f.announce(fmt.Sprintf(
		"\n──── OTP required ────\n"+
			"Check the %s inbox for a code sent after %s, then write just the digits to:\n"+
			"  %s\n"+
			"For example:\n"+
			"  echo 123456 > %s\n"+
			"Waiting up to %s…\n",
		f.Spec.service(), since.Local().Format("15:04:05"), f.Path, f.Path, p.timeout))

	code, err := p.run(ctx, func() (string, error) {
		found, rerr := f.read(since)
		if rerr != nil || found == "" {
			return "", rerr
		}
		// Remove it promptly: it is a credential, and leaving it behind would
		// also make the next run's freshness check ambiguous.
		if xerr := os.Remove(f.Path); xerr != nil && !os.IsNotExist(xerr) {
			f.announce(fmt.Sprintf("  (could not remove %s: %v)\n", f.Path, xerr))
		}
		return found, nil
	})
	if errors.Is(err, errWaitedTooLong) {
		return "", fmt.Errorf("no code written to %s within %s", f.Path, p.timeout)
	}
	return code, err
}

// read returns the code if the file holds a valid one written after since, or
// "" if there is nothing usable yet.
func (f *File) read(since time.Time) (string, error) {
	info, err := os.Stat(f.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("stat %s: %w", f.Path, err)
	}
	// A file predating this attempt holds an earlier code.
	if !info.ModTime().After(since) {
		return "", nil
	}

	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", f.Path, err)
	}
	code := strings.TrimSpace(string(raw))
	if code == "" {
		return "", nil
	}
	if err := validateDigits(code, f.Spec.Digits); err != nil {
		// Report and keep waiting: the writer can simply correct the file.
		f.announce(fmt.Sprintf("  ✗ %s: %v\n", f.Path, err))
		return "", nil
	}
	return code, nil
}

// announce shows the user something, through the caller's hook when there is
// one so a test can read it back.
func (f *File) announce(msg string) {
	if f.Announce != nil {
		f.Announce(msg)
		return
	}
	_, _ = fmt.Fprint(os.Stderr, msg)
}
