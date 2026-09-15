// Package manualasset reads and writes the 資産 rows of a MoneyForward manual
// account.
//
// Separate from the parent package, which drives a browser to sign in. The two
// share nothing but the site: this one speaks plain HTTP with the resulting
// session, and its form field names, CSRF patterns and endpoint paths have no
// business being reachable from the sign-in code.
//
// Everything here was CONFIRMED against the live account page on 2026-08-01.
// The account page renders fine in a browser but keeps a headless renderer busy
// long enough that querying its DOM times out, and none of that work is needed
// to submit what are plain form posts.
package manualasset

import (
	"strconv"
)

// MaxEntryNameLength is the limit MoneyForward enforces on an entry's name,
// established by exceeding it: 名称は20文字以内でお願いします.
//
// The site announces that as a 200 with the page re-rendered, so a name over
// the limit is dropped with no error anywhere — which is how two of five
// holdings went missing from a run that reported success.
const MaxEntryNameLength = 20

// Entry is one row in the manual portfolio.
//
// Existing rows carry two identifiers, and they are not interchangeable: an
// update addresses the numeric one, a delete the hashed one. Keeping both is
// simpler than deriving either.
type Entry struct {
	// ID is the numeric identifier an update needs. Empty for a row not yet
	// created.
	ID string

	// Hash is the identifier a delete needs. Empty for a row not yet created.
	Hash string

	// Token is the CSRF token from this row's own edit form. Tokens here are
	// per-form, not per-session: reusing the create form's token on an update
	// gets the request treated as forged, which Rails answers by nullifying the
	// session and redirecting to sign-in — indistinguishable from an expired
	// login.
	Token string

	Name string

	// Yen is the current valuation.
	Yen int64

	// AcquisitionYen is what the holding cost, and whether it is known.
	//
	// MoneyForward computes 評価損益 from this, and a blank one is not treated
	// as unknown: it takes the cost to equal the current value and reports a
	// profit of exactly zero. Leaving it out is therefore not neutral.
	AcquisitionYen int64
	HasAcquisition bool

	Subclass AssetSubclass
}

// Amounts is every yen figure this entry could put in a message.
//
// On the type that holds them rather than assembled by a caller, so that
// "which of these fields are money" stays one answer. The scheduled job
// registers them with the Actions log masker.
func (e Entry) Amounts() []int64 {
	return []int64{e.Yen, e.AcquisitionYen}
}

// entriedPrice renders the acquisition cost for the form, empty when unknown.
//
//declscope:package // the writer serialises the entry through this
func (e Entry) entriedPrice() string {
	if !e.HasAcquisition {
		return ""
	}
	return strconv.FormatInt(e.AcquisitionYen, 10)
}
