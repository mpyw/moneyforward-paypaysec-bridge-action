package gmail

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestExplainDeadCredentialNamesThePublishingStatus is here because the API's own
// message sent a reader nowhere.
//
// "Token has been expired or revoked" does not say which, or why, and the why in
// this project's one real occurrence was a seven-day expiry that only shows up as
// arithmetic on run timestamps. The point of the annotation is that the next person
// checks the publishing status before issuing a replacement that would die the same
// way.
func TestExplainDeadCredentialNamesThePublishingStatus(t *testing.T) {
	// As the token endpoint words it.
	cause := errors.New(`auth: "invalid_grant" "Token has been expired or revoked."`)

	got := explainDeadCredential(cause)
	if !errors.Is(got, cause) {
		t.Error("the original error was not wrapped, so callers lose it")
	}
	for _, want := range []string{"publishing status", "Testing", "seven days"} {
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
