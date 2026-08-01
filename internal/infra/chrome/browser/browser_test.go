package browser

import (
	"testing"
)

func TestDefaultsFor(t *testing.T) {
	if opts := DefaultsFor(false); opts.Headless || opts.NoSandbox {
		t.Errorf("DefaultsFor(false) = %+v, want a headed, sandboxed browser", opts)
	}

	opts := DefaultsFor(true)
	if !opts.Headless {
		t.Error("DefaultsFor(true) should be headless; a hosted runner has no display")
	}
	if !opts.NoSandbox {
		t.Error("DefaultsFor(true) needs NoSandbox; the runner's namespaces are restricted")
	}
}

func TestSanitizeDumpLabel(t *testing.T) {
	tests := map[string]string{
		"await-home": "await-home",
		"fetch_otp":  "fetch_otp",
		"a/b c":      "a-b-c",
		"評価額":        "---",
		"":           "page",
		"probe.html": "probe-html",
	}
	for in, want := range tests {
		if got := sanitizeDumpLabel(in); got != want {
			t.Errorf("sanitizeDumpLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithoutQuery(t *testing.T) {
	tests := map[string]string{
		"https://moneyforward.com/sign_in": "https://moneyforward.com/sign_in",
		// The sign-in redirect carries OAuth state, nonces and code challenges.
		// None of it identifies the page, and a workflow log is not the place
		// for URL parameters of any kind.
		"https://id.moneyforward.com/sign_in?client_id=abc&nonce=deadbeef&state=xyz": "https://id.moneyforward.com/sign_in",
		"https://www.paypay-sec.co.jp/trade?country=usa":                             "https://www.paypay-sec.co.jp/trade",
		"https://example.test/page#section":                                          "https://example.test/page",
		"":                                                                           "",
	}
	for in, want := range tests {
		if got := withoutQuery(in); got != want {
			t.Errorf("withoutQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWithLocationLeavesNilAlone keeps the annotation off the success path: it
// costs a CDP round trip, and a nil error has no page worth naming.
func TestWithLocationLeavesNilAlone(t *testing.T) {
	if err := (Page{}).WithLocation(nil); err != nil {
		t.Errorf("WithLocation(nil) = %v, want nil", err)
	}
}
