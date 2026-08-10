package gmail

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestExplainDeadCredentialOffersEveryCause is here because the API's own message
// sends a reader nowhere, and because a message naming one cause sent this reader
// somewhere wrong.
//
// "Token has been expired or revoked" says neither which nor why. Two attempts at
// saying it anyway were wrong: the publishing status, which turned out to be In
// production, and then a claim that the account's permissions page tells a revoke
// from the rest — made without checking, and contradicted the first time it was
// used. So what is required here is that every cause is offered and that the page is
// not sold as a test.
func TestExplainDeadCredentialOffersEveryCause(t *testing.T) {
	// As the token endpoint words it.
	cause := errors.New(`auth: "invalid_grant" "Token has been expired or revoked."`)

	got := explainDeadCredential(cause)
	if !errors.Is(got, cause) {
		t.Error("the original error was not wrapped, so callers lose it")
	}
	for _, want := range []string{
		"Testing",                          // the seven-day cap
		"password changed",                 // invalidates Gmail-scoped tokens
		"myaccount.google.com/permissions", // a manual revoke
		"OAuth client",                     // deleted or rotated
		"does not tell them apart",         // the page is not a discriminator
	} {
		if !strings.Contains(got.Error(), want) {
			t.Errorf("message does not mention %q:\n%v", want, got)
		}
	}
}

// TestExplainDeadCredentialLeavesEverythingElseAlone keeps the annotation from
// attaching itself to unrelated failures, where it would be a wrong lead.
func TestExplainDeadCredentialLeavesEverythingElseAlone(t *testing.T) {
	if got := explainDeadCredential(nil); got != nil {
		t.Errorf("explainDeadCredential(nil) = %v, want nil", got)
	}

	other := fmt.Errorf("googleapi: Error 503: backend error")
	if got := explainDeadCredential(other); got != other {
		t.Errorf("explainDeadCredential() = %v, want the error unchanged", got)
	}
}
