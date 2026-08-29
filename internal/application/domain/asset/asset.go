// Package asset is the unit that crosses between the broker and the ledger.
//
// Its own package, and in the domain, because it is what the use case's ports
// are written in terms of. They used to be written in terms of the scraper's
// own struct, which made the application layer depend on the site it happened
// to be scraping — see the layering test in internal/application.
package asset

import "fmt"

// Kind is what sort of instrument a holding is.
//
// Named here rather than carried as either site's identifier. PayPay 証券's
// target table used to hold MoneyForward's asset_subclass_id directly, so how
// one site files an instrument reached into the package that scrapes the other.
// Each side translates at its own edge now.
type Kind int

const (
	// KindUnknown is the zero value, and never valid to record.
	KindUnknown Kind = iota
	DomesticStock
	USStock
	OtherStock
	MutualFund

	// SavingsInsurance is an insurance contract held for the value it
	// accumulates.
	//
	// Named for the contract rather than for what backs it. The position this
	// exists for is a foreign-currency single-premium whole life policy whose
	// value tracks US treasuries, so 外債 would describe the exposure — but the
	// thing owned is a policy, the figure recorded is its surrender value, and
	// the surrender charge that figure is net of belongs to the contract and not
	// to any bond.
	SavingsInsurance
)

// String names the kind in the words both sites use for it.
func (k Kind) String() string {
	switch k {
	case DomesticStock:
		return "国内株"
	case USStock:
		return "米国株"
	case OtherStock:
		return "その他株式"
	case MutualFund:
		return "投資信託"
	case SavingsInsurance:
		return "積立型保険"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// Valid reports whether the kind is one this program knows how to file.
//
// Checked rather than assumed: an unrecognised kind reaching the ledger would be
// filed under whatever the zero value maps to, which is a holding recorded under
// the wrong instrument type with no error anywhere.
func (k Kind) Valid() bool {
	switch k {
	case DomesticStock, USStock, OtherStock, MutualFund, SavingsInsurance:
		return true
	default:
		return false
	}
}

// Kinds is every kind this program knows how to file.
//
// A list rather than a range over the constants, so that adding one to the
// block above without adding it here is a gap something can notice: the
// MoneyForward side uses this to report which of its 資産クラス options nothing
// maps to, and a kind missing from it is simply invisible there.
func Kinds() []Kind {
	return []Kind{DomesticStock, USStock, OtherStock, MutualFund, SavingsInsurance}
}

// Asset is one 銘柄 as it should be recorded.
type Asset struct {
	// Name is the name to record it under, "[米国株] テスト電機".
	Name string

	// Yen is the current valuation.
	Yen int64

	// AcquisitionYen is what it cost, and whether that is known.
	//
	// The ledger computes 評価損益 from this, and a blank one is not treated as
	// unknown: it takes the cost to equal the current value and reports a profit
	// of exactly zero. Leaving it out is therefore not neutral.
	AcquisitionYen int64
	HasAcquisition bool

	// Kind is how it should be filed.
	Kind Kind

	// Source says where the figure came from, e.g. "usa" — for whoever is
	// checking a number by hand, which is the first thing they will want.
	//
	// A short string rather than the scraper's own structs: those were carried
	// across this boundary for exactly this purpose and never read once.
	Source string
}

// Amounts is every yen figure this asset could put in a message.
//
// On the type that holds them, so "which of these fields are money" has one
// answer. The scheduled job registers them with the Actions log masker.
func (a Asset) Amounts() []int64 {
	return []int64{a.Yen, a.AcquisitionYen}
}

// Holdings is one pass over the source: what was found, and where it looked.
//
// Categories is the second half because a category that read as empty and one
// that was never read produce the same thing — no assets. The difference decides
// whether the ledger's entries under it are stale or unverified, so dropping it
// here leaves the reconciliation downstream with nothing to decide on but the
// number of deletes.
type Holdings struct {
	// Assets is one entry per 銘柄 found.
	Assets []Asset

	// Categories is every category the pass covered, including the ones that
	// turned out to hold nothing.
	Categories []string
}
