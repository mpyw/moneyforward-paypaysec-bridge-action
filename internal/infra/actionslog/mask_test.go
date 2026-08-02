package actionslog

import (
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestMask(t *testing.T) {
	var out strings.Builder
	Masker{Out: &out}.Mask("secret")
	if got := out.String(); got != "::add-mask::secret\n" {
		t.Errorf("Mask() wrote %q", got)
	}
}

// TestMaskSkipsEmpty avoids emitting a directive that would mask nothing — and
// which the runner treats as an error.
func TestMaskSkipsEmpty(t *testing.T) {
	var out strings.Builder
	Masker{Out: &out}.Mask("")
	if out.Len() != 0 {
		t.Errorf("Mask(\"\") wrote %q", out.String())
	}
}

func TestMaskAmount(t *testing.T) {
	var out strings.Builder
	Masker{Out: &out}.MaskAmount(941356)
	if !strings.Contains(out.String(), "::add-mask::941356") {
		t.Errorf("MaskAmount() wrote %q", out.String())
	}
}

func TestMaskAll(t *testing.T) {
	var out strings.Builder
	Masker{Out: &out}.MaskAll("a", "", "b")
	if n := strings.Count(out.String(), "::add-mask::"); n != 2 {
		t.Errorf("MaskAll() emitted %d directives, want 2", n)
	}
}

// stubSource stands in for a one-time-code source.
type stubSource struct {
	code string
	err  error
}

func (s stubSource) Fetch(context.Context, time.Time) (string, error) { return s.code, s.err }
func (s stubSource) Describe() string                                 { return "stub" }

func TestCodeSourceMasksWhatItYields(t *testing.T) {
	var out strings.Builder
	src := CodeSource{Source: stubSource{code: "123456"}, Masker: Masker{Out: &out}}

	got, err := src.Fetch(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "123456" {
		t.Errorf("Fetch() = %q", got)
	}
	if !strings.Contains(out.String(), "::add-mask::123456") {
		t.Errorf("the code was not masked; log was %q", out.String())
	}
}

// TestCodeSourceMasksNothingOnFailure keeps an error path from emitting a
// directive for a code that was never obtained.
func TestCodeSourceMasksNothingOnFailure(t *testing.T) {
	var out strings.Builder
	src := CodeSource{Source: stubSource{err: errors.New("timeout")}, Masker: Masker{Out: &out}}

	if _, err := src.Fetch(t.Context(), time.Now()); err == nil {
		t.Fatal("Fetch() succeeded")
	}
	if out.Len() != 0 {
		t.Errorf("a failed fetch wrote %q", out.String())
	}
}

func TestCodeSourceDescribe(t *testing.T) {
	if got := (CodeSource{Source: stubSource{}}).Describe(); got != "stub" {
		t.Errorf("Describe() = %q", got)
	}
}

// TestMaskAmountSkipsShortFigures is the guard against making the log
// unreadable. An unknown acquisition cost arrives as 0, and registering "0"
// masks every zero the run prints afterwards — "created=0 updated=0" included.
func TestMaskAmountSkipsShortFigures(t *testing.T) {
	for _, yen := range []int64{0, 7, 42, 999, -12} {
		var out strings.Builder
		Masker{Out: &out}.MaskAmount(yen)
		if out.String() != "" {
			t.Errorf("MaskAmount(%d) registered %q; short values mask unrelated output", yen, out.String())
		}
	}
	for _, yen := range []int64{1000, 941356, -1000} {
		var out strings.Builder
		Masker{Out: &out}.MaskAmount(yen)
		if out.String() == "" {
			t.Errorf("MaskAmount(%d) registered nothing; that is a balance", yen)
		}
	}
}

// TestMaskText covers the page's own wording, which reaches the log inside a
// parse error when a figure cannot be read.
func TestMaskTextSkipsPlaceholders(t *testing.T) {
	for _, raw := range []string{"", "—", "0円", "-"} {
		var out strings.Builder
		Masker{Out: &out}.MaskText(raw)
		if out.String() != "" {
			t.Errorf("MaskText(%q) registered %q", raw, out.String())
		}
	}
	var out strings.Builder
	Masker{Out: &out}.MaskText("78万9012円")
	if !strings.Contains(out.String(), "78万9012円") {
		t.Errorf("MaskText() = %q, want the amount registered", out.String())
	}
}

// TestMaskerSharesTheStreamWithLog is the fix for a balance that reached the
// Actions log in plain digits on 2026-08-02.
//
// The masker wrote its directive to stdout and the reporter wrote the value
// with log, which writes to stderr. Both statements are correct on their own
// and the second follows the first, but they travel down separate pipes that
// the runner does not order against each other — so "register, then print"
// became "print, then register" and the mask arrived too late to hide
// anything.
//
// Asserting the two agree, rather than asserting a particular stream: what
// matters is that they are the same one.
func TestMaskerSharesTheStreamWithLog(t *testing.T) {
	var m Masker
	if m.Out != nil {
		t.Fatalf("Masker{}.Out = %v, want nil so the default applies", m.Out)
	}
	if got, want := defaultOut(), log.Writer(); got != want {
		t.Errorf("masker writes to %v, log writes to %v — a directive and the "+
			"line it is meant to hide must share a stream to be ordered at all",
			got, want)
	}
}
