package otp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeOTPFile drops a code into path with a modification time relative to now.
func writeOTPFile(t *testing.T, path, code string, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, []byte(code), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	when := time.Now().Add(age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("set mtime on %s: %v", path, err)
	}
}

func newFileSource(t *testing.T, path string) *File {
	t.Helper()
	return &File{
		Path:     path,
		Spec:     MailSpec{Label: "TestService", Digits: 6},
		Timeout:  2 * time.Second,
		Interval: 5 * time.Millisecond,
		Announce: func(string) {},
	}
}

func TestFileFetch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "otp.txt")
	writeOTPFile(t, path, "123456\n", time.Minute)

	got, err := newFileSource(t, path).Fetch(t.Context(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "123456" {
		t.Errorf("Fetch() = %q, want %q", got, "123456")
	}
}

// TestFileFetchConsumesTheFile checks the code does not outlive its use. It is a
// credential, and leaving it behind would also make the next run's freshness
// check ambiguous.
func TestFileFetchConsumesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "otp.txt")
	writeOTPFile(t, path, "123456", time.Minute)

	if _, err := newFileSource(t, path).Fetch(t.Context(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the file survived the read (stat err = %v)", err)
	}
}

// TestFileFetchIgnoresAStaleFile is the freshness rule: a code left from an
// earlier attempt is still on disk and looks identical to a new one.
func TestFileFetchIgnoresAStaleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "otp.txt")
	writeOTPFile(t, path, "999999", -time.Hour)

	src := newFileSource(t, path)
	src.Timeout = 60 * time.Millisecond

	if got, err := src.Fetch(t.Context(), time.Now()); err == nil {
		t.Fatalf("Fetch() = %q, want a timeout rather than the stale code", got)
	}
}

func TestFileFetchWaitsForTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "otp.txt")
	since := time.Now().Add(-time.Second)

	go func() {
		time.Sleep(30 * time.Millisecond)
		writeOTPFile(t, path, "112233", 0)
	}()

	got, err := newFileSource(t, path).Fetch(t.Context(), since)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "112233" {
		t.Errorf("Fetch() = %q, want %q", got, "112233")
	}
}

// TestFileFetchKeepsWaitingAfterBadInput lets the writer correct a typo rather
// than failing the whole login over it.
func TestFileFetchKeepsWaitingAfterBadInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "otp.txt")
	writeOTPFile(t, path, "abc", time.Minute)

	go func() {
		time.Sleep(30 * time.Millisecond)
		writeOTPFile(t, path, "445566", time.Minute)
	}()

	got, err := newFileSource(t, path).Fetch(t.Context(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "445566" {
		t.Errorf("Fetch() = %q, want the corrected code", got)
	}
}

func TestFileFetchHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	src := newFileSource(t, filepath.Join(t.TempDir(), "otp.txt"))
	src.Timeout = time.Minute

	done := make(chan error, 1)
	go func() {
		_, err := src.Fetch(ctx, time.Now())
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Fetch() returned nil on a cancelled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fetch() did not return after cancellation")
	}
}

// TestFileFetchAnnouncesTheCutoff guards the only thing standing between the
// writer and a stale code: freshness cannot be enforced for a hand-supplied
// value, so the instructions have to state the cutoff.
func TestFileFetchAnnouncesTheCutoff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "otp.txt")
	writeOTPFile(t, path, "123456", time.Minute)

	var announced strings.Builder
	src := newFileSource(t, path)
	src.Announce = func(msg string) { announced.WriteString(msg) }

	// Relative to now, not a fixed instant.
	//
	// This read `time.Date(2026, 8, 1, 14, 30, 15, 0, time.Local)` while the
	// file's own timestamp came from writeOTPFile as now+1m — so whether the
	// file counted as fresh depended on which side of 14:30:15 the clock
	// happened to be on. It passed in JST evenings and failed on a UTC runner
	// in the morning, which is where it was eventually caught.
	since := time.Now().Add(-time.Hour)
	if _, err := src.Fetch(t.Context(), since); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	msg := announced.String()
	for _, want := range []string{since.Local().Format("15:04:05"), "TestService", path} {
		if !strings.Contains(msg, want) {
			t.Errorf("instructions %q do not mention %q", msg, want)
		}
	}
}

func TestFileDescribe(t *testing.T) {
	src := &File{Path: "/tmp/otp.txt", Spec: MailSpec{Label: "PayPay 証券"}}
	if got := src.Describe(); !strings.Contains(got, "/tmp/otp.txt") || !strings.Contains(got, "PayPay 証券") {
		t.Errorf("Describe() = %q", got)
	}
	if got := (&File{Path: "/tmp/otp.txt"}).Describe(); !strings.Contains(got, "/tmp/otp.txt") {
		t.Errorf("Describe() with no label = %q", got)
	}
}

func TestMailSpecDefaults(t *testing.T) {
	if got := (MailSpec{}).pattern(); got != DefaultCodePattern {
		t.Error("pattern() with none set should fall back to the default")
	}
	if got := (MailSpec{}).service(); got == "" {
		t.Error("service() with no label should still name something")
	}
	if got := (MailSpec{Label: "X"}).service(); got != "X" {
		t.Errorf("service() = %q, want %q", got, "X")
	}
}
