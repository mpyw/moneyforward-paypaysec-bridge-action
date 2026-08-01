package config_test

import (
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/config"
)

// setAll gives every required variable a value, so a test can then remove one.
func setAll(t *testing.T) {
	t.Helper()
	for _, name := range secret.Required {
		t.Setenv(string(name), "value-of-"+string(name))
	}
}

func TestLoad(t *testing.T) {
	setAll(t)
	t.Setenv(config.GmailCredentials, `{"type":"authorized_user"}`)
	t.Setenv(config.CI, "true")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if c.PayPaySec.Username != "value-of-PAYPAY_SEC_USERNAME" ||
		c.PayPaySec.Password != "value-of-PAYPAY_SEC_PASSWORD" {
		t.Errorf("PayPaySec = %+v", c.PayPaySec)
	}
	if c.MoneyForward.Username != "value-of-MF_EMAIL" ||
		c.MoneyForward.Password != "value-of-MF_PASSWORD" {
		t.Errorf("MoneyForward = %+v", c.MoneyForward)
	}
	if c.AccountIDHash != "value-of-MF_ASSET_ID" {
		t.Errorf("AccountIDHash = %q", c.AccountIDHash)
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
	t.Setenv(string(secret.AccountIDHash), "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() accepted an environment with two credentials missing")
	}
	for _, want := range []string{"MF_EMAIL", "MF_ASSET_ID"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "PAYPAY_SEC_USERNAME") {
		t.Errorf("error = %v names a variable that was set", err)
	}
}

// TestLoadRefusesBeforeReturningAnything keeps a half-populated Config from
// reaching a caller that only checks err on the paths it expects to fail.
func TestLoadRefusesBeforeReturningAnything(t *testing.T) {
	setAll(t)
	t.Setenv(string(secret.PayPaySecPassword), "")

	c, err := config.Load()
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if c != (config.Config{}) {
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
