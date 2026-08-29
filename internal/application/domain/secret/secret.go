// Package secret names the credentials a run is given.
//
// One place, in the domain, because two once disagreed: the job checked a set
// of environment variables and a setup command prompted for a set, and nothing
// but care kept them the same. The setup command is gone; the names are not,
// because they are also the action's input contract — and `config_test.go` and
// `actionyml_test.go` hold all three statements of it together.
//
// The shape of the file follows the shape of a run: what every run needs, then
// one block per source it may read, then the lists that say which is which.
package secret

// Name is one credential's identifier, as the environment and the repository
// both know it.
type Name string

// The ledger. Every run signs in here, whatever it is reading.
const (
	MoneyForwardEmail Name = "MONEYFORWARD_EMAIL"
	MoneyForwardPass  Name = "MONEYFORWARD_PASSWORD"
)

// The PayPay 証券 source.
//
// The account id puts the ledger first and the source second —
// MONEYFORWARD_<source>_ASSET_ID — because that is what distinguishes one of
// these from another: they are all MoneyForward accounts, and what differs is
// whose holdings go in. It also keeps MONEYFORWARD_* one family alongside the
// ledger's own sign-in.
const (
	PayPaySecUsername Name = "PAYPAYSEC_USERNAME"
	PayPaySecPassword Name = "PAYPAYSEC_PASSWORD"
	PayPaySecAssetID  Name = "MONEYFORWARD_PAYPAYSEC_ASSET_ID"
)

// The マニュライフ生命 source.
const (
	ManulifeUsername Name = "MANULIFE_USERNAME"
	ManulifePassword Name = "MANULIFE_PASSWORD"
	ManulifeAssetID  Name = "MONEYFORWARD_MANULIFE_ASSET_ID"

	// ManulifeAcquisitionYen is what was actually paid, in yen, for the
	// single-premium contract. Optional even when the rest of the source is set.
	//
	// Supplied rather than read, because the site does not state it: the
	// contract is denominated in a foreign currency and every figure on the page
	// is either that currency or a yen conversion struck at today's rate.
	// Converting the premium at today's rate would not be the cost — it would be
	// a number that moves with the exchange rate, and a holding can be down in
	// dollars while up in yen, which is exactly the case this exists for.
	//
	// It works only because the premium was paid once. A contract paid in
	// instalments has no single historical yen figure, and the name would be a
	// lie for one — as it would for a second contract, since it names no
	// contract at all. Both are reasons to revisit rather than to generalise
	// now: there is one contract, paid once.
	//
	// Left out, the ledger takes the cost to equal the value and reports a
	// profit of exactly zero. That is not neutral.
	//
	// It is a credential in the sense that matters here: the amount is the
	// contents of an account, so it is masked like a balance and never written
	// down in this repository.
	ManulifeAcquisitionYen Name = "MANULIFE_ACQUISITION_YEN"
)

// Retired names, still accepted.
//
// A rename that breaks every existing caller costs a major version, a module
// path change, and the whole retraction dance this project has been through
// once. Accepting both names costs a lookup.
const (
	// LegacyAssetID is what [PayPaySecAssetID] was called while there was only
	// one manual account — a name that worked exactly as long as that was true.
	LegacyAssetID Name = "MONEYFORWARD_ASSET_ID"
)

// Required is what a run needs whatever it is reading: the ledger's sign-in.
//
// Everything else belongs to a source, and every source is optional — see
// [Providers]. This list held PayPay 証券's credentials for as long as it was
// the only source there was, which made one source look like part of the
// program rather than one thing the program can read.
var Required = []Name{
	MoneyForwardEmail,
	MoneyForwardPass,
}

// Provider is a source and the credentials it needs as a set.
//
// A source nobody configured is not read, and that is different from a source
// that read nothing: an unread category's entries are left alone, where an
// empty one's are deleted. So "is this configured" has to have one answer, and
// a half-filled set has to be an error — a typo in one variable name would
// otherwise stop a source being read and say nothing about it.
// Each field says what a variable is for, rather than leaving that to be
// guessed from its name. The loader used to infer it from the suffix —
// _USERNAME, _PASSWORD, _ASSET_ID — which reads fine until a source needs
// something that is none of those, and then that variable is silently dropped.
type Provider struct {
	// ID is the source's own name, as the adapters and the log lines use it.
	ID string

	// The set. All present or all absent.
	Username Name
	Password Name
	AssetID  Name

	// AcquisitionYen is what was paid, where the source has a single figure for
	// it that never changes. Optional even when the set above is present, and
	// empty for a source that has no such figure.
	AcquisitionYen Name
}

// Names is every variable this source must have in full.
//
// AcquisitionYen is not among them: it is optional within a configured source,
// and including it here would make a source that omits it read as half
// configured — which is an error.
func (p Provider) Names() []Name {
	return []Name{p.Username, p.Password, p.AssetID}
}

// Providers are the sources a run may have, in the order they are read.
//
// A run with none of them configured is refused at load: that is a
// misconfiguration rather than a smaller job. What is checked is "at least
// one", never "this one".
var Providers = []Provider{
	{
		ID:       "paypaysec",
		Username: PayPaySecUsername,
		Password: PayPaySecPassword,
		AssetID:  PayPaySecAssetID,
	},
	{
		ID:             "manulife",
		Username:       ManulifeUsername,
		Password:       ManulifePassword,
		AssetID:        ManulifeAssetID,
		AcquisitionYen: ManulifeAcquisitionYen,
	},
}

// Legacy maps a retired name to the one that replaced it.
//
// Data rather than a branch in the loader, so that "which names are still
// accepted" has one answer and a test can hold the rest of the program to it —
// including the completeness check above, where a retired name has to count as
// supplying its replacement or an existing caller's source reads as half
// configured.
//
// A value under both names is an error rather than a preference. Choosing one
// would be guessing which account a run is meant to write into, and the wrong
// guess reconciles one source's holdings against another's rows — which deletes
// them.
var Legacy = map[Name]Name{
	LegacyAssetID: PayPaySecAssetID,
}

// RequiredNames is [Required] as plain names, for checking an environment.
func RequiredNames() []string {
	out := make([]string, 0, len(Required))
	for _, n := range Required {
		out = append(out, string(n))
	}
	return out
}
