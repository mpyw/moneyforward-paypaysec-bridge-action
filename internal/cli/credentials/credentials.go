// Package credentials resolves where this program's secrets come from.
//
// One place, so the scheduled job and the debug commands cannot disagree about
// which credential they pick — a difference that would only show up in
// production.
//
// Where the environment came from is not this program's business. It used to
// read a .env itself, which meant a local-development convenience was compiled
// into the scheduled job and every command began by loading a file that does
// not exist in CI. direnv puts the values in the environment before the process
// starts; see .envrc.example.
package credentials

import (
	"context"
	"fmt"
	"os"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/gmail"
)

// DefaultCredentialsFile is where `mfpp gmail authorize` writes the user
// credential it obtains. Gitignored, and absent in CI, where the same JSON
// arrives through GmailCredentialsEnv instead.
const DefaultCredentialsFile = "gmail-credentials.json"

// OpenMailbox resolves Gmail credentials the same way everywhere: the secret
// first, then the file a local authorize wrote.
//
// One order, one place: the scheduled job and the debug commands differing on
// which credential they pick is the kind of difference that only shows up in
// production.
//
// There is deliberately no third fallback to Application Default Credentials.
// gcloud issues those with cloud-platform, so reaching for them would quietly
// read mail with a credential authorized to operate the entire Google Cloud project —
// the blast radius this project documents itself as avoiding. On a developer
// machine that happens to have ADC lying around, the fallback would have been
// invisible; in CI with the secret unset, it turned a missing secret into an
// error about metadata servers.
func OpenMailbox(ctx context.Context, credentialsFile string) (*gmail.Client, error) {
	if blob := config.GmailBlob(); blob != "" {
		_, _ = fmt.Fprintf(os.Stderr, "→ Gmail via %s\n", config.GmailCredentials)
		return gmail.NewFromJSON(ctx, []byte(blob))
	}
	if blob, err := os.ReadFile(credentialsFile); err == nil {
		_, _ = fmt.Fprintf(os.Stderr, "→ Gmail via %s\n", credentialsFile)
		return gmail.NewFromJSON(ctx, blob)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", credentialsFile, err)
	}
	return nil, fmt.Errorf(
		"no Gmail credential: neither $%s nor %s\n"+
			"(run `mfpp gmail authorize` locally, or set the secret in CI)",
		config.GmailCredentials, credentialsFile)
}
