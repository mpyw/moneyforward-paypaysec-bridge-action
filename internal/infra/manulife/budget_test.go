package manulife

import "testing"

// TestEachBudgetFitsInsideTheOneAroundIt.
//
// A budget nested inside a shorter one never applies: the handler wait was 30
// seconds inside a 20-second click, so it was 20, and the reason it was 30 —
// room for a slow day — was discarded without a word. The PayPay 証券 reader
// made the same mistake once, and CLAUDE.md carries the rule that came out of
// it: the inner waits have to fit inside the outer one, or the outer expires
// first and reports a deadline that names nothing.
//
// ReadCard runs its steps in sequence, each with its own budget, so what has to
// hold is that their sum leaves room inside the job's.
func TestEachBudgetFitsInsideTheOneAroundIt(t *testing.T) {
	steps := []struct {
		name   string
		budget int64
	}{
		{"ready", int64(readyTimeout)},
		{"click", int64(clickTimeout)},
		{"navigate", int64(navigateTimeout)},
		{"extract", int64(extractTimeout)},
	}

	var total int64
	for _, s := range steps {
		if s.budget <= 0 {
			t.Errorf("%s has no budget", s.name)
		}
		total += s.budget
	}

	// The scheduled job allows eighteen minutes for everything, and this is one
	// contract on one of two sources. A quarter of it is already generous.
	const share = int64(4 * 60 * 1e9) // four minutes
	if total > share {
		t.Errorf("one contract's budgets sum to %v, which is more than the %v this "+
			"step can have of an eighteen-minute job", total, share)
	}
}
