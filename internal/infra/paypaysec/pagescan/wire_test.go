package pagescan

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// The structs in this package exist only to be populated by chromedp
// unmarshalling what a page script returned. Nothing type-checks that pairing:
// a key renamed on either side decodes to a zero value in silence, and a zero
// TotalPresent reads downstream as "the session is not authenticated". These
// tests are the only thing holding the two halves together.

// TestFiguresDecodesWhatTheScriptReturns pins the shape extract_balance.js
// produces.
func TestFiguresDecodesWhatTheScriptReturns(t *testing.T) {
	const sample = `{"totalPresent":true,"totalRaw":"78万9012円",
"acquisitionPresent":true,"acquisitionRaw":"60万0000円",
"gainPresent":true,"gainRaw":"+18万9012円",
"holdings":[{"name":"テスト電機","ref":"/trade/brand/35/0","investText":"45万6789円","gainText":"+3.7万"}]}`

	var f Figures
	if err := json.Unmarshal([]byte(sample), &f); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !f.TotalPresent || f.TotalRaw != "78万9012円" {
		t.Errorf("total = %v %q", f.TotalPresent, f.TotalRaw)
	}
	if !f.AcquisitionPresent || f.AcquisitionRaw != "60万0000円" {
		t.Errorf("acquisition = %v %q", f.AcquisitionPresent, f.AcquisitionRaw)
	}
	if !f.GainPresent || f.GainRaw != "+18万9012円" {
		t.Errorf("gain = %v %q", f.GainPresent, f.GainRaw)
	}
	if len(f.Holdings) != 1 {
		t.Fatalf("holdings = %d rows, want 1", len(f.Holdings))
	}
	if h := f.Holdings[0]; h.Name != "テスト電機" || h.Ref != "/trade/brand/35/0" ||
		h.InvestText != "45万6789円" || h.GainText != "+3.7万" {
		t.Errorf("row = %+v", h)
	}
}

// TestDetailDecodesWhatTheScriptReturns pins the shape extract_holding.js
// produces.
func TestDetailDecodesWhatTheScriptReturns(t *testing.T) {
	const sample = `{"valuePresent":true,"valueRaw":"45万6789円",
"acquisitionPresent":true,"acquisitionRaw":"80万0000円","gainRaw":"+3万7952円"}`

	var d Detail
	if err := json.Unmarshal([]byte(sample), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !d.ValuePresent || d.ValueRaw != "45万6789円" {
		t.Errorf("value = %v %q", d.ValuePresent, d.ValueRaw)
	}
	if !d.AcquisitionPresent || d.AcquisitionRaw != "80万0000円" {
		t.Errorf("acquisition = %v %q", d.AcquisitionPresent, d.AcquisitionRaw)
	}
	if d.GainRaw != "+3万7952円" {
		t.Errorf("gain = %q", d.GainRaw)
	}
}

// TestEveryFieldIsNamedByItsScript is a cheap backstop for the scripts a browser
// test does not drive.
//
// Containment only, which is weak: "name" and "ref" are substrings of
// "selectors.name" and "href", so renaming either passed this happily while
// every holding decoded with an empty name. extract_balance.js is covered
// properly in script_test.go now, by running it; the entries left here are the
// ones no fixture page exercises, where a weak check still beats none.
func TestEveryFieldIsNamedByItsScript(t *testing.T) {
	tests := []struct {
		name  string
		build func() (string, error)
		shape any
	}{
		{"extract_holding.js", selector.ExtractHolding, Detail{}},
		{"page_state.js", func() (string, error) { return selector.PageState(selector.ValueTotal) }, pageState{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := tt.build()
			if err != nil {
				t.Fatalf("build script: %v", err)
			}
			for _, key := range jsonKeysOf(tt.shape) {
				if !strings.Contains(script, key) {
					t.Errorf("%s never mentions %q, which the Go struct expects to decode", tt.name, key)
				}
			}
		})
	}
}

// jsonKeysOf lists the wire names a struct decodes.
func jsonKeysOf(shape any) []string {
	rt := reflect.TypeOf(shape)
	keys := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		name, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// TestSettleTimedOut pins what running out of time means.
//
// This used to return nil in every case, on the reasoning that the caller
// cross-checks the figures anyway. It does not work: while 投資信託 fetches, the
// total, 投資元本, 含み益 and every row all read 0円, and 0 == 0+0 satisfies every
// cross-check there is. A slow load would record real holdings as zero.
func TestSettleTimedOut(t *testing.T) {
	tests := []struct {
		name    string
		state   pageState
		wantErr bool
		mention string
	}{
		{
			name:    "still loading",
			state:   pageState{Loading: true, Present: true, Text: "0円"},
			wantErr: true,
			mention: "placeholder",
		},
		{
			name:    "value never stopped moving",
			state:   pageState{Present: true, Text: "25万1234円"},
			wantErr: true,
			mention: "still changing",
		},
		{
			// Not this function's complaint to make: the read says "not
			// authenticated", which is what the reader needs to hear.
			name:    "the element was never there",
			state:   pageState{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := settleTimedOut(tt.state)
			if tt.wantErr != (err != nil) {
				t.Fatalf("settleTimedOut(%+v) = %v", tt.state, err)
			}
			if err != nil && !strings.Contains(err.Error(), tt.mention) {
				t.Errorf("error = %v, want it to mention %q", err, tt.mention)
			}
			// The value is a balance and this message reaches a workflow log.
			if err != nil && strings.Contains(err.Error(), tt.state.Text) && tt.state.Text != "" {
				t.Errorf("error = %v quotes the figure verbatim", err)
			}
		})
	}
}

// TestPageStateWatchesTheElementItWasGiven pins the fix for a probe that was
// looking at the wrong page.
//
// It always watched #SECURITIES_VALUE_TOTAL, which does not exist on a 銘柄's own
// page — its figure is #SECURITIES_VALUE. So every detail page reported "not
// present" for the full twenty seconds and then read anyway, with no settle
// guarantee at the one place a placeholder becomes an acquisition cost equal to
// the valuation and a 評価損益 of exactly zero.
func TestPageStateWatchesTheElementItWasGiven(t *testing.T) {
	list, err := selector.PageState(selector.ValueTotal)
	if err != nil {
		t.Fatalf("PageState() error = %v", err)
	}
	detail, err := selector.PageState(selector.HoldingValue)
	if err != nil {
		t.Fatalf("PageState() error = %v", err)
	}

	if !strings.Contains(list, selector.ValueTotal) {
		t.Errorf("the list probe does not mention %s", selector.ValueTotal)
	}
	if !strings.Contains(detail, selector.HoldingValue) {
		t.Errorf("the detail probe does not mention %s", selector.HoldingValue)
	}
	// The detail page has no total; watching for one is what the bug was.
	if strings.Contains(detail, selector.ValueTotal) {
		t.Errorf("the detail probe still watches %s, which that page does not have",
			selector.ValueTotal)
	}
	if list == detail {
		t.Error("both probes are identical, so the selector is not reaching the script")
	}
}
