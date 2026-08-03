package investapi

import (
	"strings"
	"testing"
)

// TestReadRefusesASignedOutSession is the reason the envelope is checked.
//
// A session that has expired answers with LOGIN_STATUS 1 and no holdings. Taken
// at face value that is a category that emptied — and this program deletes those,
// which it has done twice for other reasons. An expired cookie must not be able
// to look like a sale.
func TestReadRefusesASignedOutSession(t *testing.T) {
	_, err := serve(t, &stub{loginOut: true}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() accepted a signed-out session's empty portfolio")
	}
	if !strings.Contains(err.Error(), "signed out") {
		t.Errorf("error = %v, want it to say the session is signed out", err)
	}
}

// TestReadRefusesAnErrorStatus keeps a declared field from going unread, which
// is how the acquisition checks in this project used to go quiet.
func TestReadRefusesAnErrorStatus(t *testing.T) {
	_, err := serve(t, &stub{status: 9}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() ignored a non-zero STATUS")
	}
	if !strings.Contains(err.Error(), "だめ") {
		t.Errorf("error = %v, want the service's own message", err)
	}
}
