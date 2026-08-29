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
	"strconv"
	"strings"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/secret"
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

	// AllowEmptyingCategories lifts the refusal to delete every entry in a
	// category, for one run.
	//
	// The refusal exists because every mis-read this scraper has had took that
	// shape: a category holding two 銘柄 came back empty and both were deleted.
	// But selling out of a category completely is a thing people do, and a stop
	// with no way past it is the same mistake as the share-based limit it
	// replaced — which told the reader to raise a limit that nothing could raise.
	//
	// So this exists, it is off by default, and it is meant to be passed once
	// from a manual run rather than left on.
	AllowEmptyingCategories = "ALLOW_EMPTYING_CATEGORIES"
)

// Inputs is every environment variable action.yml is expected to supply.
//
// One list, because there are two statements of this contract — this package and
// action.yml — and a test compares them. Without somewhere authoritative to
// compare against, the comparison becomes a third list to keep in step.
func Inputs() []string {
	out := secret.RequiredNames()
	// The retired names too. They are still read, so a caller still has to be
	// able to supply them — and an input the action does not declare is one that
	// works for whoever sets it by hand and fails for everyone using the action
	// as documented.
	for old := range secret.Legacy {
		out = append(out, string(old))
	}
	// The optional sources. Not required, and read exactly when a caller
	// supplies them.
	for _, provider := range secret.Providers {
		for _, n := range provider.Names() {
			out = append(out, string(n))
		}
	}
	for _, provider := range secret.Providers {
		if provider.AcquisitionYen != "" {
			out = append(out, string(provider.AcquisitionYen))
		}
	}
	return append(out, GmailCredentials, AllowEmptyingCategories)
}

// Resolution is one credential's value and which name supplied it.
type Resolution struct {
	Value string

	// Deprecated names the retired variable the value came from, and is empty
	// when the current name supplied it. Reported rather than logged here: this
	// package is read by the scheduled job and by a person at a terminal, and
	// what to say about it differs.
	Deprecated secret.Name
}

// Resolve reads a credential, accepting the name it used to have.
//
// A rename that breaks every existing caller costs a major version and a module
// path change; accepting both names costs this function. See [secret.Legacy].
//
// Both names set to different values is an error rather than a preference.
// Choosing one would be guessing which account a run is meant to write into,
// and the wrong guess reconciles one source's holdings against another's rows —
// which deletes them.
func Resolve(name secret.Name) (Resolution, error) {
	current := os.Getenv(string(name))

	for old, replacement := range secret.Legacy {
		if replacement != name {
			continue
		}
		legacy := os.Getenv(string(old))
		switch {
		case legacy == "":
		case current == "":
			return Resolution{Value: legacy, Deprecated: old}, nil
		case legacy != current:
			return Resolution{}, fmt.Errorf(
				"%s and %s are both set and hold different values; %s is the current "+
					"name, so remove %s once you have checked which account you meant",
				old, name, name, old)
		}
	}
	return Resolution{Value: current}, nil
}

// MissingCredentials lists the names among these that no variable supplies,
// counting a retired name as supplying the one that replaced it.
func MissingCredentials(names ...secret.Name) []string {
	var missing []string
	for _, n := range names {
		res, err := Resolve(n)
		if err != nil || res.Value == "" {
			missing = append(missing, string(n))
		}
	}
	return missing
}

// Source is one configured source: which one it is, how to sign in to it, and
// which manual account its holdings are recorded in.
type Source struct {
	// ID is the source's own name, matching [secret.Provider.ID].
	ID string

	Login Login

	// AssetID identifies the MoneyForward manual account this source's holdings
	// are written into. One per source, so a run that cannot read one leaves the
	// other's account untouched.
	AssetID string

	// AcquisitionYen is what was paid, in yen, where the source has a single
	// figure for it that never changes — a single-premium contract. Zero
	// everywhere else, and zero when it was not supplied.
	AcquisitionYen int64
}

// Config is everything one run was given.
type Config struct {
	// MoneyForward is the ledger's own sign-in, and the only one every run
	// needs. The sources' are in [Config.Sources].
	MoneyForward Login

	// Sources are the ones this run was configured with, in [secret.Providers]
	// order. A source whose variables are absent is not here at all.
	//
	// Absent means not read, which is different from read and found empty: an
	// unread category's entries are left alone, and an empty one's are deleted.
	Sources []Source

	// GmailCredentialsJSON is the authorized_user blob, empty when the run is
	// expected to fall back to the local file.
	GmailCredentialsJSON string

	// AllowEmptyingCategories permits deleting every entry in a category.
	AllowEmptyingCategories bool

	// CI says this is a hosted runner.
	CI bool

	// Deprecated names the retired variables this run was configured with, so
	// the caller can say so once. Nothing changes because of it — the values
	// were read — but a name that still works and will not forever is worth
	// mentioning while there is time to change it.
	Deprecated []secret.Name
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
		MoneyForward: Login{
			Username: get(secret.MoneyForwardEmail),
			Password: get(secret.MoneyForwardPass),
		},
		GmailCredentialsJSON: os.Getenv(GmailCredentials),
		CI:                   os.Getenv(CI) != "",
	}

	// Refused rather than defaulted. Someone who wrote "yes" meant to lift the
	// guard, and treating that as off would fail the run for the reason they were
	// trying to get past, saying nothing about the typo.
	if v := os.Getenv(AllowEmptyingCategories); v != "" {
		allow, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("%s is %q; it takes true or false", AllowEmptyingCategories, v)
		}
		c.AllowEmptyingCategories = allow
	}

	if missing := MissingCredentials(secret.Required...); len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s",
			strings.Join(missing, ", "))
	}
	if err := c.loadSources(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Source returns the configuration for one source, and whether this run has it.
func (c Config) Source(id string) (Source, bool) {
	for _, s := range c.Sources {
		if s.ID == id {
			return s, true
		}
	}
	return Source{}, false
}

// loadSources reads whichever sources this run was configured with.
//
// All of a source's variables or none. Half a set is refused rather than
// treated as absent, because the two look identical from here and only one of
// them is what anybody meant: a mistyped variable name would otherwise stop a
// source being read, and the run would succeed with that account quietly going
// stale — which is worse than failing, because failing sends mail.
//
// None at all is refused too. A run with no source is not a smaller job.
func (c *Config) loadSources() error {
	for _, provider := range secret.Providers {
		values, present, missing, err := resolveAll(provider.Names())
		if err != nil {
			return err
		}
		// A source's optional extra counts towards "somebody was configuring
		// this". Without it, setting only MANULIFE_ACQUISITION_YEN — or setting
		// it and mistyping the other three — reads as a source nobody wanted,
		// and a variable that was deliberately set is discarded in silence.
		extra := provider.AcquisitionYen != "" && os.Getenv(string(provider.AcquisitionYen)) != ""
		switch {
		case len(present) == 0 && !extra:
			continue
		case len(missing) > 0:
			if len(present) == 0 {
				present = []string{string(provider.AcquisitionYen)}
			}
			return fmt.Errorf("the %s source is partly configured: %s set, %s missing. "+
				"Set all of them or none — a source with some of its credentials is not "+
				"a source that gets skipped, it is one that fails",
				provider.ID, strings.Join(present, ", "), strings.Join(missing, ", "))
		}

		for _, res := range values {
			if res.Deprecated != "" {
				c.Deprecated = append(c.Deprecated, res.Deprecated)
			}
		}
		source := Source{
			ID: provider.ID,
			Login: Login{
				Username: values[provider.Username].Value,
				Password: values[provider.Password].Value,
			},
			AssetID: values[provider.AssetID].Value,
		}

		// Optional even within a configured source: without it the ledger takes
		// the cost to equal the value and reports a profit of exactly zero,
		// which is wrong — but not a reason to refuse to record the valuation,
		// which is right.
		if provider.AcquisitionYen != "" {
			if v := os.Getenv(string(provider.AcquisitionYen)); v != "" {
				yen, perr := strconv.ParseInt(v, 10, 64)
				if perr != nil || yen < 0 {
					// Negative is refused rather than recorded. It would be a
					// cost basis below zero, which the ledger turns into a
					// profit larger than the holding — a plausible number, which
					// is the shape of every mistake this program guards against.
					return fmt.Errorf("%s is %q; it takes a whole number of yen, "+
						"digits only", provider.AcquisitionYen, v)
				}
				source.AcquisitionYen = yen
			}
		}
		c.Sources = append(c.Sources, source)
	}

	// Two sources writing into one account would each reconcile against the
	// other's rows, and reconciliation deletes what it does not recognise. The
	// coverage check catches it eventually, in terms nobody can act on; this
	// says what is actually wrong.
	seen := map[string]string{}
	for _, source := range c.Sources {
		if other, dup := seen[source.AssetID]; dup {
			return fmt.Errorf("the %s and %s sources are set to write into the same "+
				"MoneyForward account; each source needs its own, or they delete each "+
				"other's entries", other, source.ID)
		}
		seen[source.AssetID] = source.ID
	}

	if len(c.Sources) == 0 {
		return fmt.Errorf("no source is configured; set one source's variables in full. "+
			"The sources are: %s", strings.Join(providerSummary(), "; "))
	}
	return nil
}

// resolveAll reads every name in a set, counting a retired name as supplying
// the one that replaced it.
//
// Through [Resolve] rather than the environment directly, and that matters
// here: an existing caller supplies MONEYFORWARD_ASSET_ID and not the name that
// replaced it, and a raw lookup would report the PayPay 証券 source as partly
// configured — which is a hard failure for the exact people the rename was
// arranged not to break.
func resolveAll(names []secret.Name) (map[secret.Name]Resolution, []string, []string, error) {
	values := make(map[secret.Name]Resolution, len(names))
	var present, missing []string
	for _, n := range names {
		res, err := Resolve(n)
		if err != nil {
			return nil, nil, nil, err
		}
		values[n] = res
		if res.Value == "" {
			missing = append(missing, string(n))
			continue
		}
		present = append(present, string(n))
	}
	return values, present, missing, nil
}

// providerSummary lists each source and what it needs, for the message a run
// with nothing configured gets.
func providerSummary() []string {
	out := make([]string, 0, len(secret.Providers))
	for _, p := range secret.Providers {
		names := make([]string, 0, 3)
		for _, n := range p.Names() {
			names = append(names, string(n))
		}
		out = append(out, p.ID+" ("+strings.Join(names, ", ")+")")
	}
	return out
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
