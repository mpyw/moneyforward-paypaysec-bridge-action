package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
)

// setAll gives every required variable a value, so a test can then remove one.
//
// It also clears every retired name. These tests run in a developer's shell,
// where direnv has already exported the real credentials — including, for as
// long as the rename is in flight, MONEYFORWARD_ASSET_ID. Leaving it in place
// made half of these fail against a value nothing here had set, with an error
// about two names disagreeing.
func setAll(t *testing.T) {
	t.Helper()
	for _, name := range secret.Required {
		t.Setenv(string(name), "value-of-"+string(name))
	}
	for old := range secret.Legacy {
		t.Setenv(string(old), "")
	}
	// One source, so a run is configured at all. Every source is optional but
	// having none is refused, so the baseline for these tests is the one every
	// existing caller has.
	setSource(t, "paypaysec")
	clearSource(t, "manulife")
}

// setSource gives every variable of one source a value.
func setSource(t *testing.T, id string) {
	t.Helper()
	for _, provider := range secret.Providers {
		if provider.ID != id {
			continue
		}
		for _, n := range provider.Names() {
			t.Setenv(string(n), "value-of-"+string(n))
		}
	}
}

// clearSource removes one source's variables, which a developer's direnv has
// already exported. See setAll.
func clearSource(t *testing.T, id string) {
	t.Helper()
	for _, provider := range secret.Providers {
		if provider.ID != id {
			continue
		}
		for _, n := range provider.Names() {
			t.Setenv(string(n), "")
		}
	}
	for _, provider := range secret.Providers {
		if provider.ID == id && provider.AcquisitionYen != "" {
			t.Setenv(string(provider.AcquisitionYen), "")
		}
	}
}

// sourceIn is one configured source, or a failure naming what was there
// instead.
func sourceIn(t *testing.T, c config.Config, id string) config.Source {
	t.Helper()
	src, ok := c.Source(id)
	if !ok {
		var ids []string
		for _, s := range c.Sources {
			ids = append(ids, s.ID)
		}
		t.Fatalf("no %s source; configured: %v", id, ids)
	}
	return src
}

func TestLoad(t *testing.T) {
	setAll(t)
	t.Setenv(config.GmailCredentials, `{"type":"authorized_user"}`)
	t.Setenv(config.CI, "true")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	pp := sourceIn(t, c, "paypaysec")
	if pp.Login.Username != "value-of-PAYPAYSEC_USERNAME" ||
		pp.Login.Password != "value-of-PAYPAYSEC_PASSWORD" {
		t.Errorf("paypaysec login = %+v", pp.Login)
	}
	if c.MoneyForward.Username != "value-of-MONEYFORWARD_EMAIL" ||
		c.MoneyForward.Password != "value-of-MONEYFORWARD_PASSWORD" {
		t.Errorf("MoneyForward = %+v", c.MoneyForward)
	}
	if pp.AssetID != "value-of-MONEYFORWARD_PAYPAYSEC_ASSET_ID" {
		t.Errorf("AssetID = %q", pp.AssetID)
	}
	if !c.CI {
		t.Error("CI was set and not seen")
	}
}

// TestLoadNamesEveryMissingVariable: a misconfigured run should take one round
// trip to fix, not one per variable.
func TestLoadNamesEveryMissingVariable(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.MoneyForwardEmail), "")
	t.Setenv(string(secret.MoneyForwardPass), "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() accepted an environment with two credentials missing")
	}
	for _, want := range []string{"MONEYFORWARD_EMAIL", "MONEYFORWARD_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "PAYPAYSEC_USERNAME") {
		t.Errorf("error = %v names a variable that was set", err)
	}
}

// TestLoadRefusesBeforeReturningAnything keeps a half-populated Config from
// reaching a caller that only checks err on the paths it expects to fail.
func TestLoadRefusesBeforeReturningAnything(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.MoneyForwardPass), "")

	c, err := config.Load()
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !reflect.DeepEqual(c, config.Config{}) {
		t.Errorf("Load() returned %+v alongside the error", c)
	}
}

// TestGmailCredentialsAreOptional: absent means "fall back to the local file",
// which is how a laptop runs it.
func TestGmailCredentialsAreOptional(t *testing.T) {
	setAll(t)
	t.Setenv(config.GmailCredentials, "")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if c.GmailCredentialsJSON != "" {
		t.Errorf("GmailCredentialsJSON = %q", c.GmailCredentialsJSON)
	}
}

// TestMissingChecksOnlyWhatItWasAsked: a debug step signing in to one service
// should not demand the other's password.
func TestMissingChecksOnlyWhatItWasAsked(t *testing.T) {
	t.Setenv(string(secret.MoneyForwardEmail), "someone@example.com")
	t.Setenv(string(secret.MoneyForwardPass), "")

	missing := config.Missing(string(secret.MoneyForwardEmail), string(secret.MoneyForwardPass))
	if len(missing) != 1 || missing[0] != string(secret.MoneyForwardPass) {
		t.Errorf("Missing() = %v, want just the password", missing)
	}
}

// TestEveryRequiredSecretIsLoaded ties the domain's list to what Load actually
// reads. The drift it guards against once had a setup command and the job
// keeping two lists; the command is gone and action.yml is the second list now.
func TestEveryRequiredSecretIsLoaded(t *testing.T) {
	setAll(t)
	for _, name := range secret.Required {
		t.Run(string(name), func(t *testing.T) {
			t.Setenv(string(name), "")
			if _, err := config.Load(); err == nil {
				t.Errorf("Load() succeeded without %s, so nothing reads it", name)
			}
		})
	}
}

// The retired name for the PayPay 証券 account id.
//
// It is still read, because renaming it outright would break every existing
// caller — which costs a major version and a module path change, and this
// project has been through that once already.

func TestLoadAcceptsTheRetiredAssetIDName(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.PayPaySecAssetID), "")
	t.Setenv(string(secret.LegacyAssetID), "from-the-old-name")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v — the old name should still work", err)
	}
	if got := sourceIn(t, c, "paypaysec").AssetID; got != "from-the-old-name" {
		t.Errorf("AssetID = %q", got)
	}
	if len(c.Deprecated) != 1 || c.Deprecated[0] != secret.LegacyAssetID {
		t.Errorf("Deprecated = %v, want it to name %s so the caller can say so once",
			c.Deprecated, secret.LegacyAssetID)
	}
}

// TestLoadPrefersNothingWhenTheTwoNamesDisagree: which account a run writes
// into is not something to guess at.
//
// Reconciling one source's holdings against another account's rows deletes
// them, so the two names holding different values has to stop the run rather
// than resolve to whichever was checked first.
func TestLoadRefusesTwoAssetIDNamesThatDisagree(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.PayPaySecAssetID), "one-account")
	t.Setenv(string(secret.LegacyAssetID), "another-account")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() accepted two asset ids and picked one")
	}
	for _, want := range []string{string(secret.PayPaySecAssetID), string(secret.LegacyAssetID)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}

// TestLoadAcceptsTheTwoNamesAgreeing: the same value under both is somebody
// mid-migration, not a mistake.
func TestLoadAcceptsTheTwoNamesAgreeing(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.PayPaySecAssetID), "one-account")
	t.Setenv(string(secret.LegacyAssetID), "one-account")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := sourceIn(t, c, "paypaysec").AssetID; got != "one-account" {
		t.Errorf("AssetID = %q", got)
	}
	if len(c.Deprecated) != 0 {
		t.Errorf("Deprecated = %v — the current name supplied the value", c.Deprecated)
	}
}

// TestEveryLegacyNameReplacesSomethingReal keeps the alias table honest.
//
// A retired name pointing at one nothing asks for is a rename that resolves to
// a value nobody reads — which looks like it works right up to the run that
// needs it.
func TestEveryLegacyNameReplacesSomethingReal(t *testing.T) {
	known := map[secret.Name]bool{}
	for _, n := range secret.Required {
		known[n] = true
	}
	for _, provider := range secret.Providers {
		for _, n := range provider.Names() {
			known[n] = true
		}
	}
	for old, replacement := range secret.Legacy {
		if !known[replacement] {
			t.Errorf("%s is said to replace %s, which nothing reads", old, replacement)
		}
		if known[old] {
			t.Errorf("%s is listed as retired and as a name something reads", old)
		}
	}
}

// TestLoadRefusesARunWithNoSource: a run with nothing to read is a
// misconfiguration, not a smaller job.
//
// It has to name what a source is, too. "No source is configured" is only
// actionable if the reader is told which sets of variables would make one.
func TestLoadRefusesARunWithNoSource(t *testing.T) {
	setAll(t)
	for _, provider := range secret.Providers {
		clearSource(t, provider.ID)
	}
	for old := range secret.Legacy {
		t.Setenv(string(old), "")
	}

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() accepted a run with no source configured")
	}
	for _, provider := range secret.Providers {
		if !strings.Contains(err.Error(), provider.ID) {
			t.Errorf("error = %v, want it to name the %s source as an option", err, provider.ID)
		}
	}
}

// TestLoadWithOnlyTheOptionalSource: PayPay 証券 is not special.
//
// It was required for as long as it was the only source, which made it look
// like part of the program. Somebody who holds only an insurance contract has a
// perfectly ordinary configuration.
func TestLoadWithOnlyTheOptionalSource(t *testing.T) {
	setAll(t)
	clearSource(t, "paypaysec")
	for old := range secret.Legacy {
		t.Setenv(string(old), "")
	}
	setSource(t, "manulife")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v — a run reading only マニュライフ生命 is a run", err)
	}
	if len(c.Sources) != 1 || c.Sources[0].ID != "manulife" {
		t.Errorf("Sources = %+v, want just the one that was configured", c.Sources)
	}
}

// TestTheRetiredNameSatisfiesItsSource is what keeps the rename non-breaking.
//
// An existing caller supplies MONEYFORWARD_ASSET_ID and not the name that
// replaced it. If the source's completeness check looked at the environment
// directly, that caller's PayPay 証券 source would read as partly configured —
// a hard failure, for exactly the people the alias exists to protect.
func TestTheRetiredNameSatisfiesItsSource(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.PayPaySecAssetID), "")
	t.Setenv(string(secret.LegacyAssetID), "from-the-old-name")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v — the old name has to complete the source", err)
	}
	if got := sourceIn(t, c, "paypaysec").AssetID; got != "from-the-old-name" {
		t.Errorf("AssetID = %q", got)
	}
}

// The optional source, and the distinction the whole arrangement rests on:
// a source nobody configured is not read, which is not the same as a source
// that read nothing. An unread category's entries are left alone; an empty
// one's are deleted.

func TestLoadWithoutTheOptionalSource(t *testing.T) {
	setAll(t)

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v — a caller with no マニュライフ生命 contract is "+
			"a normal caller", err)
	}
	if _, ok := c.Source("manulife"); ok {
		t.Error("the manulife source is configured with none of its variables set")
	}
}

func TestLoadWithTheOptionalSource(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.ManulifeUsername), "someone@example.com")
	t.Setenv(string(secret.ManulifePassword), "a-password")
	t.Setenv(string(secret.ManulifeAssetID), "an-account")
	t.Setenv(string(secret.ManulifeAcquisitionYen), "4000000")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	ml := sourceIn(t, c, "manulife")
	if ml.AssetID != "an-account" {
		t.Errorf("AssetID = %q", ml.AssetID)
	}
	if ml.AcquisitionYen != 4000000 {
		t.Errorf("AcquisitionYen = %d", ml.AcquisitionYen)
	}
	// Both sources, in the order the domain lists them.
	if len(c.Sources) != 2 || c.Sources[0].ID != "paypaysec" || c.Sources[1].ID != "manulife" {
		t.Errorf("Sources = %+v, want both in Providers order", c.Sources)
	}
}

// TestLoadRefusesAPartlyConfiguredSource is the one that matters.
//
// A mistyped variable name would otherwise leave the source unconfigured, and
// unconfigured is silent: the run succeeds, that account is never touched, and
// its figure quietly stops being updated. A stale figure nobody is told about
// is worse than a failure, because a failure sends mail.
func TestLoadRefusesAPartlyConfiguredSource(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.ManulifeUsername), "someone@example.com")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() accepted a source with one of its three variables set")
	}
	for _, want := range []string{
		string(secret.ManulifeUsername),
		string(secret.ManulifePassword),
		string(secret.ManulifeAssetID),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}

// TestAcquisitionYenIsOptionalWithinTheSource: without it the ledger reports a
// profit of exactly zero, which is wrong — but not a reason to refuse to record
// the valuation, which is right.
func TestAcquisitionYenIsOptionalWithinTheSource(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.ManulifeUsername), "someone@example.com")
	t.Setenv(string(secret.ManulifePassword), "a-password")
	t.Setenv(string(secret.ManulifeAssetID), "an-account")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := sourceIn(t, c, "manulife").AcquisitionYen; got != 0 {
		t.Errorf("AcquisitionYen = %d, want it optional within a configured source", got)
	}
}

// TestAcquisitionYenRefusesWhatItCannotRead: refused rather than dropped.
//
// Somebody who wrote "4,000,000" meant to supply a cost basis, and treating it
// as absent would record a profit of exactly zero while saying nothing about
// the comma.
func TestAcquisitionYenRefusesWhatItCannotRead(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.ManulifeUsername), "someone@example.com")
	t.Setenv(string(secret.ManulifePassword), "a-password")
	t.Setenv(string(secret.ManulifeAssetID), "an-account")
	t.Setenv(string(secret.ManulifeAcquisitionYen), "4,000,000")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() accepted an acquisition cost it could not read")
	}
}

// TestAcquisitionYenAloneIsAPartlyConfiguredSource.
//
// Somebody who set it was configuring the source. Treating that as "no source"
// discards a variable that was deliberately set, and does it in silence — which
// is the failure mode the all-or-nothing rule exists to prevent, arriving
// through the one variable the rule was not looking at.
func TestAcquisitionYenAloneIsAPartlyConfiguredSource(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.ManulifeAcquisitionYen), "4000000")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() ignored a source variable that was set")
	}
	if !strings.Contains(err.Error(), "manulife") {
		t.Errorf("error = %v, want it to name the source", err)
	}
}

// TestAcquisitionYenRefusesANegative: a cost basis below zero becomes a profit
// larger than the holding, which is a plausible number.
func TestAcquisitionYenRefusesANegative(t *testing.T) {
	setAll(t)
	setSource(t, "manulife")
	t.Setenv(string(secret.ManulifeAcquisitionYen), "-4000000")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() accepted a negative acquisition cost")
	}
}

// TestLoadRefusesTwoSourcesSharingOneAccount.
//
// They would each reconcile against the other's rows, and reconciliation
// deletes what it does not recognise. The coverage check would stop it, in
// terms that say nothing about the cause.
func TestLoadRefusesTwoSourcesSharingOneAccount(t *testing.T) {
	setAll(t)
	setSource(t, "manulife")
	t.Setenv(string(secret.PayPaySecAssetID), "one-account")
	t.Setenv(string(secret.ManulifeAssetID), "one-account")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() accepted two sources writing into one account")
	}
	for _, want := range []string{"paypaysec", "manulife"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}

// TestEveryProvidersExtraIsAnInput keeps action.yml's contract complete.
//
// Inputs() used to name MANULIFE_ACQUISITION_YEN directly, so a second source
// with a figure of its own would have been read and never declared — which
// works for whoever sets it by hand and fails for everyone using the action as
// documented.
func TestEveryProvidersExtraIsAnInput(t *testing.T) {
	declared := map[string]bool{}
	for _, name := range config.Inputs() {
		declared[name] = true
	}
	for _, provider := range secret.Providers {
		if provider.AcquisitionYen == "" {
			continue
		}
		if !declared[string(provider.AcquisitionYen)] {
			t.Errorf("%s is read for the %s source but Inputs() does not list it",
				provider.AcquisitionYen, provider.ID)
		}
	}
}
