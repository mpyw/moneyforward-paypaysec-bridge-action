package gmail

import (
	"strings"
	"testing"
)

// TestNewFromJSONTakesOnlyAnAuthorizedUser pins the one credential kind this
// program has.
//
// GMAIL_CREDENTIALS is a secret, but it is still a blob handed in from outside
// the binary, and detection would take whichever kind it happened to name. An
// external_account configuration is the reason this matters: it carries the URLs
// its token is fetched from, so accepting one is accepting a redirect chosen by
// whoever wrote the secret. Only a refresh token for a personal mailbox is ever
// correct here — domain-wide delegation is Workspace-only — so anything else is
// a misconfiguration worth naming at the point it is read.
func TestNewFromJSONTakesOnlyAnAuthorizedUser(t *testing.T) {
	for _, tc := range []struct {
		name string
		blob string
	}{
		{"service_account", `{"type":"service_account","project_id":"p","private_key_id":"k",
			"private_key":"-----BEGIN PRIVATE KEY-----\nnope\n-----END PRIVATE KEY-----\n",
			"client_email":"a@b.iam.gserviceaccount.com","client_id":"1","token_uri":"https://oauth2.googleapis.com/token"}`},
		{"external_account", `{"type":"external_account","audience":"//iam.googleapis.com/x",
			"subject_token_type":"urn:ietf:params:oauth:token-type:jwt",
			"token_url":"https://example.invalid/token","credential_source":{"file":"/etc/passwd"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewFromJSON(t.Context(), []byte(tc.blob)); err == nil {
				t.Fatalf("NewFromJSON() accepted a %s credential", tc.name)
			} else if !strings.Contains(err.Error(), "parse credentials") {
				t.Errorf("error = %v, want it to name the credentials as the problem", err)
			}
		})
	}
}

// TestNewFromJSONTakesTheOneItIsGiven is the other half: the shape this program
// actually writes is accepted without reaching the network.
func TestNewFromJSONTakesTheOneItIsGiven(t *testing.T) {
	blob := `{"type":"authorized_user","client_id":"id","client_secret":"secret","refresh_token":"token"}`
	if _, err := NewFromJSON(t.Context(), []byte(blob)); err != nil {
		t.Fatalf("NewFromJSON() error = %v", err)
	}
}
