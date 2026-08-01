// Package actionslog keeps values out of a GitHub Actions run log.
//
// Separate from the work that produces those values, because masking is a
// property of where the program is running, not of what it is doing. Locally
// these directives are inert text.
package actionslog

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Masker registers values with the Actions log masker.
//
// A type rather than a package-level func so tests can capture what was masked,
// and so the destination is explicit — which turned out to be the whole
// problem. See [Masker.Out].
type Masker struct {
	// Out defaults to os.Stderr, which is where log writes.
	//
	// It was os.Stdout, on the belief that the runner only reads directives
	// there. It reads them from either — checked on a runner, all four
	// combinations of directive-stream and value-stream came back masked — and
	// the belief cost a real balance.
	//
	// The two streams are separate pipes and the runner does not order them
	// against each other. The same check showed a line written to stdout
	// rendering ahead of one written to stderr before it. So a directive on
	// stdout followed microseconds later by log.Printf on stderr is a race, and
	// on 2026-08-02 it lost: the portfolio total went into the log in plain
	// digits, one line after the call that was supposed to hide it.
	//
	// Sharing one stream with log makes the ordering the program already assumes
	// the ordering it actually gets.
	Out io.Writer
}

// defaultOut is where directives go when Out is unset: the stream log uses, so
// that a directive and the line it hides are ordered.
func defaultOut() io.Writer { return os.Stderr }

// Mask hides one value from the log.
//
// Registering is idempotent and cheap, so callers mask early — before the value
// can reach a log line or an error message — rather than reasoning about which
// paths might print it.
func (m Masker) Mask(value string) {
	if value == "" {
		return
	}
	out := m.Out
	if out == nil {
		out = defaultOut()
	}
	_, _ = fmt.Fprintf(out, "::add-mask::%s\n", value)
}

// minMaskable is the shortest value worth registering.
//
// ::add-mask:: replaces every later occurrence of the exact string, everywhere
// in the run. Registering "0" — which is how an unknown acquisition cost used to
// arrive here — turns "created=0 updated=0 unchanged=5" into a row of asterisks,
// and takes every other zero in the log with it. Four characters is where a
// value stops being something the log is likely to say for other reasons.
const minMaskable = 4

// MaskAmount hides a yen figure.
//
// Figures below four digits are deliberately left alone; see [minMaskable].
// That is a trade: a three-digit balance goes unmasked rather than making the
// whole log unreadable. Nothing else this project masks is ever that short.
func (m Masker) MaskAmount(yen int64) {
	text := strconv.FormatInt(yen, 10)
	if len(strings.TrimPrefix(text, "-")) < minMaskable {
		return
	}
	m.Mask(text)
}

// MaskText hides a value read off a page, such as "69万1356円".
//
// The raw text matters as well as the parsed number: a figure that fails to
// parse reaches the log inside the parse error, still in the page's own words.
// Short values are skipped for the same reason as in [MaskAmount] — "—" and "0円"
// say nothing and would mask a lot.
func (m Masker) MaskText(value string) {
	if len([]rune(value)) < minMaskable {
		return
	}
	m.Mask(value)
}

// MaskAll hides several values at once.
func (m Masker) MaskAll(values ...string) {
	for _, v := range values {
		m.Mask(v)
	}
}

// CodeSource wraps a one-time-code source so every code it yields is masked
// before the caller can print it.
//
// The wrapper exists because the code arrives deep inside a login flow: by the
// time it reaches anything that might log, it has already passed through several
// hands. Masking at the source is the only point where that is guaranteed.
type CodeSource struct {
	Source interface {
		Fetch(ctx context.Context, since time.Time) (string, error)
		Describe() string
	}
	Masker Masker
}

// Describe names the underlying source.
func (c CodeSource) Describe() string { return c.Source.Describe() }

// Fetch obtains a code and masks it.
func (c CodeSource) Fetch(ctx context.Context, since time.Time) (string, error) {
	code, err := c.Source.Fetch(ctx, since)
	if err == nil {
		c.Masker.Mask(code)
	}
	return code, err
}
