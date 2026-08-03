package sync

import (
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/actionslog"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward/manualasset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/pagescan"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// TestMaskFiguresCoversAReconciliationError is the point of the OnRead hook.
//
// Reading.Amount names the amounts that disagree — that is what makes the error
// worth having, and also what would put a balance in a workflow log verbatim.
// ::add-mask:: only affects output that comes after it, so this asserts every
// figure in the message was registered before the message could be printed.
func TestMaskFiguresCoversAReconciliationError(t *testing.T) {
	// 投資元本 + 含み益 does not reach 評価額合計: one of the three is misread.
	reading := paypaysec.Reading{
		Target: selector.Target{Key: "usa", Name: "米国株"},
		Figures: pagescan.Figures{
			TotalPresent: true, TotalRaw: "78万9012円",
			AcquisitionPresent: true, AcquisitionRaw: "60万0000円",
			GainPresent: true, GainRaw: "+1万0000円",
		},
		TotalYen: 789012, HasTotal: true,
		AcquisitionYen: 600000, HasAcquisition: true,
		GainYen: 10000, HasGain: true,
	}

	_, err := reading.Amount()
	if err == nil {
		t.Fatal("Amount() accepted figures that do not add up; this test needs one that fails")
	}

	var registered strings.Builder
	maskFigures(actionslog.Masker{Out: &registered})(reading)
	masked := registered.String()

	// Every figure the message names, and the page's own wording alongside it.
	for _, want := range []string{"789012", "600000", "610000", "78万9012円", "60万0000円"} {
		if !strings.Contains(masked, "::add-mask::"+want) {
			t.Errorf("%q was never registered, so it would reach the log", want)
		}
	}

	// And the message really does contain them, so this test fails if the
	// wording ever stops naming the amounts.
	if !strings.Contains(err.Error(), "789012") {
		t.Errorf("Amount() error = %v; expected it to name the figures", err)
	}
}

// TestMaskFiguresLeavesCountsAlone guards the other direction. A holding with no
// known acquisition cost carries 0, and registering "0" would mask every zero
// the run prints — including the plan summary.
func TestMaskFiguresLeavesCountsAlone(t *testing.T) {
	reading := paypaysec.Reading{
		Holdings: []paypaysec.Holding{
			{Name: "テストAIファンド", Yen: 5432, HasYen: true, InvestText: "5432円"},
		},
	}

	var registered strings.Builder
	maskFigures(actionslog.Masker{Out: &registered})(reading)
	masked := registered.String()

	if strings.Contains(masked, "::add-mask::0\n") {
		t.Error("zero was registered; \"created=0 updated=0\" would print as asterisks")
	}
	if !strings.Contains(masked, "::add-mask::5432") {
		t.Error("the holding's value was not registered")
	}
}

// TestMaskEntriesCoversTheVerificationFailure is the write side of the same
// problem. The recorded value in that message is whatever MoneyForward has,
// which this program did not choose and therefore could not mask in advance.
func TestMaskEntriesCoversTheVerificationFailure(t *testing.T) {
	// What the account already held, before anything was written.
	existing := []manualasset.Entry{
		{Name: "[米国株] テスト電機", Yen: 456789, AcquisitionYen: 400000, HasAcquisition: true},
		{Name: "[投信ミ] テストAIファンド", Yen: 5432},
	}

	var registered strings.Builder
	maskEntries(actionslog.Masker{Out: &registered})(existing)
	masked := registered.String()

	for _, want := range []string{"456789", "400000", "5432"} {
		if !strings.Contains(masked, "::add-mask::"+want) {
			t.Errorf("%q was never registered; a verification failure would name it", want)
		}
	}
	if strings.Contains(masked, "::add-mask::0\n") {
		t.Error("the unknown acquisition cost registered zero")
	}
}
