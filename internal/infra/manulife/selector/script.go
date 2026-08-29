package selector

import (
	"embed"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/pagescript"
)

// scripts holds the page-side JavaScript for this site.
//
// In files rather than Go string constants so the selectors above can be passed
// in as arguments instead of spliced into source text, and so the extraction
// can be read as what it is — a walk over a DOM — by someone looking at the
// page in DevTools beside it.
//
//go:embed js/*.js
var scriptFS embed.FS

// siteScripts are this site's extraction routines. Its own set, so a name here
// cannot collide with another site's or with the browser layer's generic
// probes.
var siteScripts = pagescript.Load(scriptFS, "js")

// Read once: a missing file is a build mistake, so it surfaces at init rather
// than on the first scrape.
const (
	extractContractsScript = "extract_contracts.js"
	extractPolicyScript    = "extract_policy.js"
	markContractScript     = "mark_contract.js"
)

// MarkContract renders the call that finds one contract's card by its number,
// marks it and clicks it.
//
// The number crosses into the page as a JSON argument rather than spliced into
// the source; see [pagescript.Apply].
func MarkContract(number string) (string, error) {
	return siteScripts.Call(markContractScript, map[string]string{
		"card":  ContractCard,
		"table": ContractCardTable,
		"mark":  ContractMarkAttr,
	}, number)
}

// ExtractContracts renders the call that reads the contract list.
func ExtractContracts() (string, error) {
	return siteScripts.Call(extractContractsScript, map[string]string{
		"card":  ContractCard,
		"title": ContractCardTitle,
		"table": ContractCardTable,
	})
}

// ExtractPolicy renders the call that reads one contract's detail page.
func ExtractPolicy() (string, error) {
	return siteScripts.Call(extractPolicyScript, map[string]string{
		"summary":     PolicySummary,
		"summaryRow":  SummaryRow,
		"valueMarker": SummaryValueMarker,
		"valueText":   ValueText,
	})
}
