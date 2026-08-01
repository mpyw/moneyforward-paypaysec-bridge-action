package paypaysec

import (
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec/pagescan"
)

// The acquisition cost is the figure MoneyForward computes 評価損益 from, and
// establishing it is the most conditional part of this scrape: two routes
// depending on the template, three places a parse can fail. Every one of those
// used to be a silent skip, and none was reachable by a test while it sat behind
// a browser call.

func TestApplyDetailPrefersTheStatedCost(t *testing.T) {
	h := Holding{Name: "テスト電機", Yen: 456789, HasYen: true}
	err := h.applyDetail(pagescan.Detail{
		ValuePresent: true, ValueRaw: "45万6789円",
		AcquisitionPresent: true, AcquisitionRaw: "40万0000円",
		GainRaw: "+3万7952円",
	})
	if err != nil {
		t.Fatalf("applyDetail() error = %v", err)
	}
	if !h.HasAcquisition || h.AcquisitionYen != 400000 {
		t.Errorf("cost = %d (known=%v), want the stated 400000", h.AcquisitionYen, h.HasAcquisition)
	}
	if !h.HasDetailYen || h.DetailYen != 456789 {
		t.Errorf("DetailYen = %d (known=%v)", h.DetailYen, h.HasDetailYen)
	}
}

// TestApplyDetailSubtractsWhenNoCostIsStated covers the other route: the site
// renders the acquisition element only for some products, and the profit on this
// page is unrounded, so the subtraction is exact.
func TestApplyDetailSubtractsWhenNoCostIsStated(t *testing.T) {
	h := Holding{Name: "テスト・グローバル・ファンド", Yen: 345678, HasYen: true}
	err := h.applyDetail(pagescan.Detail{
		ValuePresent: true, ValueRaw: "345678円",
		GainRaw: "+45678円",
	})
	if err != nil {
		t.Fatalf("applyDetail() error = %v", err)
	}
	if !h.HasAcquisition || h.AcquisitionYen != 300000 {
		t.Errorf("cost = %d (known=%v), want 345678-45678", h.AcquisitionYen, h.HasAcquisition)
	}
}

// TestApplyDetailCatchesTheWrongPage is the one check that can notice the scrape
// followed a link to something else.
func TestApplyDetailCatchesTheWrongPage(t *testing.T) {
	h := Holding{Name: "テスト電機", Yen: 456789, HasYen: true}
	err := h.applyDetail(pagescan.Detail{
		ValuePresent: true, ValueRaw: "234567円", // a different holding's figure
		GainRaw: "+1000円",
	})
	if err == nil {
		t.Fatal("applyDetail() accepted a page describing a different holding")
	}
	// Recorded even so, so the masker sees it before the message is printed.
	if h.DetailYen != 234567 {
		t.Errorf("DetailYen = %d; the figure in the message was not recorded", h.DetailYen)
	}
}

// TestApplyDetailRefusesUnreadableFigures is what the mutation testing was for:
// each of these used to be swallowed, leaving a later check disabled or a cost
// silently taken from the other route.
func TestApplyDetailRefusesUnreadableFigures(t *testing.T) {
	tests := map[string]pagescan.Detail{
		"the page's own valuation": {
			ValuePresent: true, ValueRaw: "評価額 12株", GainRaw: "+1000円",
		},
		"a stated acquisition cost": {
			ValuePresent: true, ValueRaw: "456789円",
			AcquisitionPresent: true, AcquisitionRaw: "800,00円",
			GainRaw: "+37952円",
		},
		"the profit": {
			ValuePresent: true, ValueRaw: "456789円", GainRaw: "+3.7万",
		},
	}
	for name, detail := range tests {
		t.Run(name, func(t *testing.T) {
			h := Holding{Name: "テスト電機", Yen: 456789, HasYen: true}
			if err := h.applyDetail(detail); err == nil {
				t.Errorf("applyDetail() accepted an unreadable figure; cost came back %d (known=%v)",
					h.AcquisitionYen, h.HasAcquisition)
			}
		})
	}
}

// TestApplyDetailToleratesAPlaceholder keeps a legitimately empty page from
// failing the run: no figure is not the same as an unreadable one.
func TestApplyDetailToleratesAPlaceholder(t *testing.T) {
	h := Holding{Name: "新規", Yen: 1000, HasYen: true}
	if err := h.applyDetail(pagescan.Detail{ValuePresent: true, ValueRaw: "—", GainRaw: "—"}); err != nil {
		t.Fatalf("applyDetail() error = %v", err)
	}
	if h.HasAcquisition {
		t.Errorf("a placeholder produced a cost of %d", h.AcquisitionYen)
	}
	if h.HasDetailYen {
		t.Error("a placeholder produced a valuation")
	}
}

// TestApplyDetailErrorsDoNotQuoteParsedFigures keeps balances out of a workflow
// log where the raw text is enough to say what went wrong.
func TestApplyDetailErrorsDoNotQuoteParsedFigures(t *testing.T) {
	h := Holding{Name: "テスト電機", Yen: 456789, HasYen: true}
	err := h.applyDetail(pagescan.Detail{ValuePresent: true, ValueRaw: "12株", GainRaw: "+1円"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), "456789") {
		t.Errorf("error = %v quotes the balance", err)
	}
}
