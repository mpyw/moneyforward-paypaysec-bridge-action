// Package config is the one place this program reads its environment.
//
// Every value it needs from outside is named here once and read here once.
// They were spread over six files, three of them naming a variable with a
// string literal while the domain already had a constant for it — which is how
// `secrets setup` and the scheduled job came to keep two lists of the same
// credentials.
//
// It is also the contract. This is meant to be usable as a GitHub Action from
// another repository, and an action's inputs are exactly this: a fixed set of
// names, each documented, each either required or defaulted. What is declared
// below is what action.yml declares, and nothing else should be reachable
// through os.Getenv.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/secret"
)

// Variables not already named by [secret], because they are not credentials.
const (
	// GmailCredentials holds an authorized_user JSON blob. In CI it comes from a
	// secret; locally the file written by `mfpp gmail authorize` is used
	// instead.
	GmailCredentials = "GMAIL_CREDENTIALS"

	// CI is set by every hosted runner, and decides whether Chrome runs headless
	// and unsandboxed. Read rather than guessed: dropping the sandbox is a real
	// reduction in isolation for a browser pointed at brokerage sites, and a
	// local run should not do it.
	CI = "CI"
)

// Config is everything one run was given.
type Config struct {
	// PayPaySec and MoneyForward are the two sign-ins.
	PayPaySec    Login
	MoneyForward Login

	// AssetID identifies the manual account the entries live in.
	AssetID string

	// GmailCredentialsJSON is the authorized_user blob, empty when the run is
	// expected to fall back to the local file.
	GmailCredentialsJSON string

	// CI says this is a hosted runner.
	CI bool
}

// Login is one service's credentials.
type Login struct {
	Username string
	Password string
}

// Load reads the environment.
//
// Nothing is defaulted and nothing is guessed: a missing credential is reported
// here, by name, rather than at whichever step first needs it — where the error
// is about that step and says nothing about what is actually wrong.
//
// Every missing name at once, so a misconfigured run takes one round trip to
// fix rather than one per variable.
func Load() (Config, error) {
	c := Config{
		PayPaySec: Login{
			Username: get(secret.PayPaySecUsername),
			Password: get(secret.PayPaySecPassword),
		},
		MoneyForward: Login{
			Username: get(secret.MoneyForwardEmail),
			Password: get(secret.MoneyForwardPass),
		},
		AssetID:              get(secret.AssetID),
		GmailCredentialsJSON: os.Getenv(GmailCredentials),
		CI:                   os.Getenv(CI) != "",
	}
	if missing := Missing(secret.RequiredNames()...); len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s",
			strings.Join(missing, ", "))
	}
	return c, nil
}

// Value reads one credential, named the way the domain names it.
//
// For the debug commands, which need one or two rather than a whole [Config] —
// `debug mf list` has no use for a PayPay 証券 password and should not demand
// one.
func Value(name secret.Name) string { return os.Getenv(string(name)) }

// get is Value, for use inside this package.
func get(name secret.Name) string { return Value(name) }

// GmailBlob is the authorized_user JSON, or "" when the run is expected to fall
// back to the local file.
func GmailBlob() string { return os.Getenv(GmailCredentials) }

// Missing lists the variables among names that are unset or empty.
//
// Separate from [Load] for the commands that need only some of them: a debug
// step that signs in to one service should say so rather than demanding the
// other service's password.
func Missing(names ...string) []string {
	var missing []string
	for _, n := range names {
		if os.Getenv(n) == "" {
			missing = append(missing, n)
		}
	}
	return missing
}

// IsCI reports whether this is a hosted runner, for the places that need it
// before a full [Load] — the browser, which is started by commands that do not
// all need credentials.
func IsCI() bool { return os.Getenv(CI) != "" }
