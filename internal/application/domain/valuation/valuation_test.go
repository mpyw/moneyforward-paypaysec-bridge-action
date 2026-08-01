package valuation

import (
	"errors"
	"strings"
	"testing"
)

// These were tests of a scraper, because the rule lived in one. It is
// arithmetic about money, and every case below is a shape some source could
// report — a page, an API, a file — not a shape one particular site produces.

func figures(total, cost, gain Amount, positions ...Position) Figures {
	return Figures{Total: total, Acquisition: cost, Gain: gain, Positions: positions}
}

func position(name string, value, cost int64) Position {
	return Position{Name: name, Value: Yen(value), Cost: Yen(cost)}
}

func TestReconciled(t *testing.T) {
	f := figures(Yen(691356), Yen(600000), Yen(91356),
		position("テスト電機", 456789, 400000),
		position("テスト商事", 234567, 200000),
	)
	got, err := f.Reconciled()
	if err != nil {
		t.Fatalf("Reconciled() error = %v", err)
	}
	if got != 691356 {
		t.Errorf("Reconciled() = %d, want 691356", got)
	}
}

// TestReconciledRefusesOnDisagreement is the whole point of asking three ways.
// Every figure below would look entirely reasonable in a financial record.
func TestReconciledRefusesOnDisagreement(t *testing.T) {
	tests := map[string]struct {
		figures Figures
		mention string
	}{
		"cost plus gain does not reach the total": {
			figures: figures(Yen(691356), Yen(600000), Yen(10000),
				position("テスト電機", 691356, 600000)),
			mention: "one of the three is misread",
		},
		"the holdings do not add up": {
			figures: figures(Yen(691356), Absent(), Absent(),
				position("テスト電機", 456789, 400000)),
			mention: "holdings sum to",
		},
		"the costs do not add up": {
			figures: figures(Yen(691356), Yen(600000), Yen(91356),
				position("テスト電機", 456789, 400000),
				position("テスト商事", 234567, 100000),
			),
			mention: "costs sum to",
		},
		"one holding has no cost to check": {
			figures: figures(Yen(691356), Yen(600000), Yen(91356),
				position("テスト電機", 456789, 400000),
				Position{Name: "テスト商事", Value: Yen(234567), Cost: Placeholder()},
			),
			mention: "テスト商事",
		},
		"money with nothing listed under it": {
			figures: figures(Yen(691356), Absent(), Absent()),
			mention: "positions are missing",
		},
		"a placeholder is not a total": {
			figures: figures(Placeholder(), Absent(), Absent()),
			mention: "placeholder",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tt.figures.Reconciled()
			if err == nil {
				t.Fatalf("Reconciled() = %d, want a refusal", got)
			}
			if !strings.Contains(err.Error(), tt.mention) {
				t.Errorf("Reconciled() error = %v, want it to mention %q", err, tt.mention)
			}
		})
	}
}

// TestReconciledOnAnUnreportedTotal keeps "the source had no such figure" apart
// from "the figure is zero". For a scraped page the first almost always means
// the session is not authenticated, and only the caller can say so.
func TestReconciledOnAnUnreportedTotal(t *testing.T) {
	_, err := figures(Absent(), Absent(), Absent()).Reconciled()
	if !errors.Is(err, ErrNotReported) {
		t.Fatalf("Reconciled() error = %v, want ErrNotReported", err)
	}
}

// TestReconciledOnAnEmptyCategory is the legitimate version of "a total with
// nothing under it": nothing held, nothing to check.
func TestReconciledOnAnEmptyCategory(t *testing.T) {
	got, err := figures(Yen(0), Yen(0), Yen(0)).Reconciled()
	if err != nil || got != 0 {
		t.Errorf("Reconciled() = %d, %v; want 0 and no error", got, err)
	}
}

// TestReconciledSkipsPlaceholderPositions keeps an empty holding out of the sum
// without failing: "—" is a legitimate state, not a misread.
func TestReconciledSkipsPlaceholderPositions(t *testing.T) {
	f := figures(Yen(456789), Absent(), Absent(),
		position("テスト電機", 456789, 400000),
		Position{Name: "空っぽ", Value: Placeholder(), Cost: Placeholder()},
	)
	if got, err := f.Reconciled(); err != nil || got != 456789 {
		t.Errorf("Reconciled() = %d, %v", got, err)
	}
}

// TestLabelsReachTheMessage covers what the labels are for: a refusal an
// operator can read against the page in front of them.
func TestLabelsReachTheMessage(t *testing.T) {
	f := figures(Yen(691356), Yen(600000), Yen(10000),
		position("テスト電機", 691356, 600000))
	f.Labels = Labels{Total: "評価額合計", Acquisition: "投資元本", Gain: "含み益"}

	_, err := f.Reconciled()
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, want := range []string{"評価額合計", "投資元本", "含み益"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to use the source's word %q", err, want)
		}
	}
}

// TestAmountsCoverEveryFigureAMessageNames is what keeps a balance out of a
// workflow log: the scheduled job registers these with the masker before
// anything can print them, and a sum computed only inside a refusal exists
// nowhere else.
func TestAmountsCoverEveryFigureAMessageNames(t *testing.T) {
	f := figures(Yen(691356), Yen(600000), Yen(10000),
		position("テスト電機", 456789, 400000),
		position("テスト商事", 234567, 200000),
	)
	_, err := f.Reconciled()
	if err == nil {
		t.Fatal("expected a refusal to inspect")
	}

	known := map[string]bool{}
	for _, yen := range f.Amounts() {
		known[itoa(yen)] = true
	}
	// 610000 is cost + gain, computed in the message and stored nowhere.
	for _, want := range []string{"691356", "600000", "610000"} {
		if !known[want] {
			t.Errorf("Amounts() does not include %s, which the refusal names", want)
		}
	}
	if !strings.Contains(err.Error(), "610000") {
		t.Errorf("error = %v; expected it to name the derived sum this test guards", err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
