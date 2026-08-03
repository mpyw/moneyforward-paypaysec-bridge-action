package investapi

import (
	"errors"
	"testing"
)

// TestReadMiniAppFetchesItsClientNumber pins the one piece of state these calls
// need, and where it comes from.
func TestReadMiniAppFetchesItsClientNumber(t *testing.T) {
	s := &stub{}
	c := serve(t, s)
	if _, err := c.Read(t.Context(), MiniApp); err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if s.fields[miniTop]["MINI_CLIENT_SEQ_NO"] != "900000001" {
		t.Errorf("MINI_CLIENT_SEQ_NO = %q, want the one info returned",
			s.fields[miniTop]["MINI_CLIENT_SEQ_NO"])
	}
	if s.fields[miniTop]["APP_ID"] != "6" {
		t.Errorf("APP_ID = %q, want the mini bucket", s.fields[miniTop]["APP_ID"])
	}

	// Asked as the mini bucket, not as the app bucket whose path it borrows. The
	// page has one transport per bucket and reaches this endpoint through the mini
	// one whenever the mini tab is showing; asking as the app bucket is a
	// different question, and the answer to it made the service refuse the top
	// call outright.
	if got := s.fields[appInfo]["APP_ID"]; got != "6" {
		t.Errorf("info APP_ID = %q, want the mini bucket's", got)
	}
	if _, sent := s.fields[appInfo]["MINI_CLIENT_SEQ_NO"]; !sent {
		t.Error("the mini defaults declare MINI_CLIENT_SEQ_NO, so even the call " +
			"that asks for it carries it")
	}

	// Asked once, then remembered: a second bucket read must not re-fetch it.
	before := len(s.mu)
	if _, err := c.Read(t.Context(), MiniApp); err != nil {
		t.Fatalf("second Read() error = %v", err)
	}
	for _, p := range s.mu[before:] {
		if p == appInfo {
			t.Error("the client number was fetched again")
		}
	}
}

// TestReadReportsAnAbsentMiniBucketAsAbsent covers both halves of the page's own
// test for whether this bucket exists.
//
// The distinction it protects is the whole point: absent must not read as failed,
// which would stop a run every weekday for anyone without the ミニアプリ, and must
// not read as empty either, which is a licence to delete everything recorded under
// the category. It is a third answer, and callers check for this sentinel.
//
// What the endpoints do for such an account is not known and cannot be observed
// from an account that has the bucket. Not asking is the part that does not need
// the observation.
func TestReadReportsAnAbsentMiniBucketAsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name string
		stub *stub
	}{
		// A client number of zero: the field is numeric, so this is the shape
		// absence takes in it.
		{"no client number", &stub{noMiniClient: true}},
		{"not on offer", &stub{miniNotUsable: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := serve(t, tc.stub).Read(t.Context(), MiniApp)
			if !errors.Is(err, ErrNoMiniApp) {
				t.Errorf("Read() error = %v, want ErrNoMiniApp", err)
			}
		})
	}
}
