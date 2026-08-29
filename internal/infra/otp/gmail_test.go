package otp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/gmail"
)

// fakeMailbox serves canned results, one batch per poll.
type fakeMailbox struct {
	batches [][]gmail.Message
	err     error
	calls   int
	queries []string
}

func (f *fakeMailbox) Search(_ context.Context, query string, _ int64) ([]gmail.Message, error) {
	f.queries = append(f.queries, query)
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if len(f.batches) == 0 {
		return nil, nil
	}
	batch := f.batches[0]
	if len(f.batches) > 1 {
		f.batches = f.batches[1:]
	}
	return batch, nil
}

// quiet keeps a source's transient warnings — a failed search, an unparsable
// code — out of the test log. They are expected in most of the cases below.
func quiet(g *Gmail) *Gmail {
	g.Warn = func(string, ...any) {}
	return g
}

func TestGmailFetch(t *testing.T) {
	since := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		messages []gmail.Message
		want     string
	}{
		{
			name: "plain six digit code",
			messages: []gmail.Message{
				{ID: "a", Received: since.Add(time.Minute), Body: "認証コード: 123456 を入力してください"},
			},
			want: "123456",
		},
		{
			// PayPay 証券 prefixes the code with two letters for its anti-phishing
			// check. Only the digits go into the form.
			name: "two letter prefix is not part of the code",
			messages: []gmail.Message{
				{ID: "a", Received: since.Add(time.Minute), Body: "認証コード AB-123456 (10分以内)"},
			},
			want: "123456",
		},
		{
			name: "html only body",
			messages: []gmail.Message{
				{ID: "a", Received: since.Add(time.Minute), Body: "<p>コード</p><b>112233</b>"},
			},
			want: "112233",
		},
		{
			name: "newest qualifying message wins",
			messages: []gmail.Message{
				{ID: "old", Received: since.Add(time.Minute), Body: "111111"},
				{ID: "new", Received: since.Add(5 * time.Minute), Body: "222222"},
			},
			want: "222222",
		},
		{
			// The previous run's mail is still in the mailbox and looks the same.
			name: "message older than the cutoff is ignored",
			messages: []gmail.Message{
				{ID: "stale", Received: since.Add(-time.Hour), Body: "999999"},
				{ID: "fresh", Received: since.Add(time.Minute), Body: "123456"},
			},
			want: "123456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := quiet(&Gmail{
				Mail:     &fakeMailbox{batches: [][]gmail.Message{tt.messages}},
				Spec:     MailSpec{Query: "from:example", Label: "Test", Digits: 6},
				Timeout:  time.Second,
				Interval: time.Millisecond,
			})
			got, err := src.Fetch(t.Context(), since)
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Fetch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGmailFetchWaitsForArrival(t *testing.T) {
	since := time.Now()
	mailbox := &fakeMailbox{batches: [][]gmail.Message{
		{}, // nothing yet
		{}, // still nothing
		{{ID: "a", Received: since.Add(time.Minute), Body: "123456"}},
	}}

	src := quiet(&Gmail{
		Mail: mailbox, Spec: MailSpec{Query: "from:example", Digits: 6},
		Timeout: 5 * time.Second, Interval: time.Millisecond,
	})
	got, err := src.Fetch(t.Context(), since)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "123456" {
		t.Errorf("Fetch() = %q, want %q", got, "123456")
	}
	if mailbox.calls < 3 {
		t.Errorf("polled %d times, expected it to keep polling until the mail arrived", mailbox.calls)
	}
}

// TestGmailFetchAcceptsMailStampedInTheSameSecond is the production failure of
// 2026-08-01, which every other test here missed by placing the mail a whole
// minute after since.
//
// internalDate is a whole second; since is time.Now() with nanoseconds. A mail
// sent in response to the login and delivered 300ms later is stamped with the
// floor of that second, which is before since. Requiring strictly-newer made
// that mail unusable forever: the poll ran for 100 seconds with the message it
// wanted in every single result set.
func TestGmailFetchAcceptsMailStampedInTheSameSecond(t *testing.T) {
	// Mid-second, as time.Now() almost always is.
	since := time.Date(2026, 8, 1, 18, 6, 46, 700_000_000, time.UTC)
	// What Gmail records for a mail that arrives a moment later.
	stamped := time.Date(2026, 8, 1, 18, 6, 46, 0, time.UTC)

	src := quiet(&Gmail{
		Mail: &fakeMailbox{batches: [][]gmail.Message{{
			{ID: "otp", Received: stamped, Body: "認証コード: 123456"},
		}}},
		Spec:    MailSpec{Query: "from:example", Digits: 6},
		Timeout: 50 * time.Millisecond, Interval: time.Millisecond,
	})

	got, err := src.Fetch(t.Context(), since)
	if err != nil {
		t.Fatalf("Fetch() error = %v — the mail is in the result set and it is this run's", err)
	}
	if got != "123456" {
		t.Errorf("Fetch() = %q, want %q", got, "123456")
	}
}

// TestGmailFetchStillRejectsThePreviousSecond pins the other side: widening the
// cutoff to a whole second must not widen it to two.
func TestGmailFetchStillRejectsThePreviousSecond(t *testing.T) {
	since := time.Date(2026, 8, 1, 18, 6, 46, 700_000_000, time.UTC)
	previous := time.Date(2026, 8, 1, 18, 6, 45, 0, time.UTC)

	src := quiet(&Gmail{
		Mail: &fakeMailbox{batches: [][]gmail.Message{{
			{ID: "last-run", Received: previous, Body: "認証コード: 999999"},
		}}},
		Spec:    MailSpec{Query: "from:example", Digits: 6},
		Timeout: 50 * time.Millisecond, Interval: time.Millisecond,
	})

	if got, err := src.Fetch(t.Context(), since); err == nil {
		t.Fatalf("Fetch() = %q, want a timeout rather than the previous run's code", got)
	}
}

func TestGmailFetchRejectsStaleOnly(t *testing.T) {
	since := time.Now()
	src := quiet(&Gmail{
		Mail: &fakeMailbox{batches: [][]gmail.Message{{
			{ID: "stale", Received: since.Add(-time.Hour), Body: "999999"},
		}}},
		Spec:    MailSpec{Query: "from:example", Digits: 6},
		Timeout: 50 * time.Millisecond, Interval: time.Millisecond,
	})

	if got, err := src.Fetch(t.Context(), since); err == nil {
		t.Fatalf("Fetch() = %q, want a timeout rather than the stale code", got)
	}
}

func TestGmailFetchSurvivesTransientErrors(t *testing.T) {
	since := time.Now()
	// A failing mailbox must not end the wait early; the deadline is the backstop.
	src := quiet(&Gmail{
		Mail:    &fakeMailbox{err: errors.New("429 rate limited")},
		Spec:    MailSpec{Query: "from:example", Digits: 6},
		Timeout: 30 * time.Millisecond, Interval: time.Millisecond,
	})
	_, err := src.Fetch(t.Context(), since)
	if err == nil {
		t.Fatal("Fetch() succeeded despite a permanently failing mailbox")
	}
	if !strings.Contains(err.Error(), "no message at or after") {
		t.Errorf("Fetch() error = %v, want the timeout message", err)
	}
}

func TestGmailFetchIgnoresWrongLengthCodes(t *testing.T) {
	since := time.Now()
	src := quiet(&Gmail{
		Mail: &fakeMailbox{batches: [][]gmail.Message{{
			{ID: "a", Received: since.Add(time.Minute), Body: "order 12345678 shipped"},
		}}},
		Spec:    MailSpec{Query: "from:example", Digits: 6},
		Timeout: 30 * time.Millisecond, Interval: time.Millisecond,
	})
	if got, err := src.Fetch(t.Context(), since); err == nil {
		t.Fatalf("Fetch() = %q, want no match for an eight digit run", got)
	}
}

// TestGmailFetchBoundsTheQueryByTime pins the shape of the time filter.
//
// An earlier version used `after:<epoch>`. Gmail accepts that without
// complaint and then matches nothing, so every poll came back empty and the
// login sat waiting for a code that was already sitting in the inbox. The
// failure looked exactly like a mail that had not arrived, which is why it took
// a live run to notice.
func TestGmailFetchBoundsTheQueryByTime(t *testing.T) {
	since := time.Now()
	mailbox := &fakeMailbox{batches: [][]gmail.Message{{
		{ID: "a", Received: since.Add(time.Minute), Body: "123456"},
	}}}
	src := quiet(&Gmail{
		Mail: mailbox, Spec: MailSpec{Query: "from:noreply@example.com", Digits: 6},
		Timeout: time.Second, Interval: time.Millisecond,
	})
	if _, err := src.Fetch(t.Context(), since); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(mailbox.queries) == 0 {
		t.Fatal("no query recorded")
	}
	q := mailbox.queries[0]
	if !strings.Contains(q, "from:noreply@example.com") {
		t.Errorf("query %q lost the caller's filter", q)
	}
	if !strings.Contains(q, "newer_than:") {
		t.Errorf("query %q has no time bound, so every old mail would be fetched", q)
	}
	if strings.Contains(q, "after:") {
		t.Errorf("query %q uses after:, which Gmail silently matches nothing for", q)
	}
}

func TestGmailRequiresMailbox(t *testing.T) {
	src := &Gmail{Spec: MailSpec{Query: "from:example", Digits: 6}}
	if _, err := src.Fetch(t.Context(), time.Now()); err == nil {
		t.Fatal("Fetch() succeeded with no mailbox configured")
	}
}

func TestGmailDescribe(t *testing.T) {
	if got := (&Gmail{Spec: MailSpec{Label: "PayPay 証券"}}).Describe(); got != "Gmail (PayPay 証券)" {
		t.Errorf("Describe() = %q", got)
	}
	if got := (&Gmail{}).Describe(); got != "Gmail" {
		t.Errorf("Describe() with no label = %q", got)
	}
}

// TestGmailFetchSaysWhenThePatternIsWhatFailed separates the two ways a poll
// produces nothing.
//
// They need opposite responses — wait, or go and fix the pattern — and for a
// while both printed the same line: "N recent message(s), none at or after T
// yet", which is a statement about arrival even when the mail has plainly
// arrived. Reading it at face value against a マニュライフ生命 code that had
// arrived on time cost an afternoon, and the pattern was wrong for a reason no
// amount of waiting would change: Go's \s does not match an ideographic space.
func TestGmailFetchSaysWhenThePatternIsWhatFailed(t *testing.T) {
	since := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)

	var lines []string
	g := &Gmail{
		Mail: &fakeMailbox{batches: [][]gmail.Message{{{
			ID: "m1",
			// After the cutoff, so arrival is not the problem — the body is.
			Received: since.Add(10 * time.Second),
			Body:     "there is no code in here at all",
		}}}},
		Spec:     MailSpec{Label: "テスト", Digits: 6},
		Timeout:  50 * time.Millisecond,
		Interval: 10 * time.Millisecond,
		Warn:     func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) },
	}

	if _, err := g.Fetch(context.Background(), since); err == nil {
		t.Fatal("Fetch found a code in a message that carries none")
	}
	if len(lines) == 0 {
		t.Fatal("the wait said nothing at all")
	}
	last := lines[len(lines)-1]
	if strings.Contains(last, "none at or after") {
		t.Errorf("reported an arrival problem for a message that had arrived: %q", last)
	}
	for _, want := range []string{"at or after", "none carried a 6-digit code"} {
		if !strings.Contains(last, want) {
			t.Errorf("warning = %q, want it to mention %q", last, want)
		}
	}
}

// TestGmailFetchStillReportsArrivalWhenNothingIsFresh is the other half: with
// no message past the cutoff, the wait is a wait and should say so.
func TestGmailFetchStillReportsArrivalWhenNothingIsFresh(t *testing.T) {
	since := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)

	var lines []string
	g := &Gmail{
		Mail: &fakeMailbox{batches: [][]gmail.Message{{{
			ID:       "old",
			Received: since.Add(-time.Minute),
			Body:     "認証コード: 112233",
		}}}},
		Spec:     MailSpec{Label: "テスト", Digits: 6},
		Timeout:  50 * time.Millisecond,
		Interval: 10 * time.Millisecond,
		Warn:     func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) },
	}

	if _, err := g.Fetch(context.Background(), since); err == nil {
		t.Fatal("Fetch accepted a message from before the cutoff")
	}
	if len(lines) == 0 {
		t.Fatal("the wait said nothing at all")
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, "none at or after") {
		t.Errorf("warning = %q, want it to report that nothing has arrived yet", last)
	}
}
