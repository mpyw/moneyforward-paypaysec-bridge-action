package authorizegmail_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/credential"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/usecase/authorizegmail"
)

type grants struct {
	cred credential.Gmail
	err  error
}

func (g grants) Obtain(context.Context) (credential.Gmail, error) { return g.cred, g.err }

type kept struct {
	cred  credential.Gmail
	saved bool
	err   error
}

func (k *kept) Store(_ context.Context, cred credential.Gmail) error {
	if k.err != nil {
		return k.err
	}
	k.cred, k.saved = cred, true
	return nil
}

type opens struct {
	mailbox string
	err     error
	asked   bool
}

func (o *opens) OpenMailbox(context.Context, credential.Gmail) (string, error) {
	o.asked = true
	return o.mailbox, o.err
}

func usable() credential.Gmail {
	return credential.Gmail{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh"}
}

func TestAuthorize(t *testing.T) {
	store := &kept{}
	verify := &opens{mailbox: "someone@example.com"}

	result, err := authorizegmail.Authorize{
		Consent: grants{cred: usable()}, Store: store, Verify: verify,
	}.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !store.saved || store.cred.RefreshToken != "refresh" {
		t.Errorf("stored %+v", store.cred)
	}
	if result.Mailbox != "someone@example.com" {
		t.Errorf("Mailbox = %q", result.Mailbox)
	}
}

// TestAuthorizeRefusesACredentialThatNeedsAPerson is the point of the whole use
// case.
//
// Google completes the flow without a refresh token when the account has
// consented before, and it all looks like success — a token comes back, the
// mailbox opens, a credential is written. It expires within the hour, and every
// scheduled run after that fails on authentication, a long way from here.
func TestAuthorizeRefusesACredentialThatNeedsAPerson(t *testing.T) {
	store := &kept{}
	granted := usable()
	granted.RefreshToken = ""

	_, err := authorizegmail.Authorize{Consent: grants{cred: granted}, Store: store}.Run(t.Context())
	if !errors.Is(err, credential.ErrNotUnattended) {
		t.Fatalf("Run() error = %v, want ErrNotUnattended", err)
	}
	if store.saved {
		t.Error("it was stored anyway; somebody would assume it works")
	}
}

// TestAuthorizeVerifiesBeforeStoring keeps a credential that does not work off
// the disk, where its presence is taken for proof.
func TestAuthorizeVerifiesBeforeStoring(t *testing.T) {
	store := &kept{}
	verify := &opens{err: errors.New("401")}

	_, err := authorizegmail.Authorize{
		Consent: grants{cred: usable()}, Store: store, Verify: verify,
	}.Run(t.Context())
	if err == nil {
		t.Fatal("Run() accepted a credential that could not open a mailbox")
	}
	if !strings.Contains(err.Error(), "does not work") {
		t.Errorf("error = %v", err)
	}
	if store.saved {
		t.Error("it was stored despite not working")
	}
}

// TestAuthorizeWithoutVerificationIsAllowed keeps the check optional, for a
// caller with no network.
func TestAuthorizeWithoutVerificationIsAllowed(t *testing.T) {
	store := &kept{}
	result, err := authorizegmail.Authorize{Consent: grants{cred: usable()}, Store: store}.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !store.saved {
		t.Error("nothing was stored")
	}
	if result.Mailbox != "" {
		t.Errorf("Mailbox = %q with no verifier", result.Mailbox)
	}
}

func TestAuthorizeStopsOnARefusedConsent(t *testing.T) {
	store := &kept{}
	_, err := authorizegmail.Authorize{
		Consent: grants{err: errors.New("access_denied")}, Store: store,
	}.Run(t.Context())
	if err == nil {
		t.Fatal("Run() succeeded despite a refused consent")
	}
	if store.saved {
		t.Error("something was stored after consent was refused")
	}
}
