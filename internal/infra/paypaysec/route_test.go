package paypaysec

import (
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// TestReadRoutesEachTargetByItsOwnFlag pins which of the two readers a target
// gets, for every target there is.
//
// The routing used to live in GetBalances, so `debug paypaysec balance` drove the
// page for 投資信託 while the scheduled job drove the API. A debug tool that reads
// differently from the job cannot reproduce the job's mistakes, and this project
// has already lost entries to a mis-read that only the job could produce.
//
// Neither reader can work here — there is no browser and no session — and that is
// what makes the test cheap: each fails in its own words, so the words say which
// one ran. The API reader borrows cookies from the browser before it does anything
// else; the page reader navigates.
//
// What this pins is that Read honours the flag, not what the flag is set to: the
// expectation comes from the same field the routing does, so flipping one target
// flips both sides and this test agrees with itself. Which target must be read
// which way is [selector.TestSharedURLTargetsAreReadOverTheAPI]'s job, and it
// catches exactly the flip this one does not.
func TestReadRoutesEachTargetByItsOwnFlag(t *testing.T) {
	const borrow = "borrow session"

	for _, target := range selector.Targets {
		t.Run(target.Key, func(t *testing.T) {
			_, err := Read(t.Context(), target)
			if err == nil {
				t.Fatal("Read() succeeded with no browser and no session")
			}
			borrowed := strings.Contains(err.Error(), borrow)

			switch {
			case target.ViaAPI && !borrowed:
				t.Errorf("Read() = %v, want it to have borrowed the session; "+
					"a ViaAPI target that reaches the page is read from a view "+
					"that cannot say which bucket it is showing", err)
			case !target.ViaAPI && borrowed:
				t.Errorf("Read() = %v, but this target has no API to read", err)
			}
		})
	}
}
