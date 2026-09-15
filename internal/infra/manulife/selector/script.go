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

// MarkContractScript renders the call that finds one contract's card by its number,
// marks it and clicks it.
//
// The number crosses into the page as a JSON argument rather than spliced into
// the source; see [pagescript.Apply].
func MarkContractScript(number string) (string, error) {
	return siteScripts.Call("mark_contract.js", map[string]string{
		"card":  ContractCard,
		"table": ContractCardTable,
		"mark":  ContractMarkAttr,
	}, number)
}

// ExtractContractsScript renders the call that reads the contract list.
func ExtractContractsScript() (string, error) {
	return siteScripts.Call("extract_contracts.js", map[string]string{
		"card":  ContractCard,
		"title": ContractCardTitle,
		"table": ContractCardTable,
	})
}

// ExtractPolicyScript renders the call that reads one contract's detail page.
func ExtractPolicyScript() (string, error) {
	return siteScripts.Call("extract_policy.js", map[string]string{
		"summary":     PolicySummary,
		"summaryRow":  SummaryRow,
		"valueMarker": SummaryValueMarker,
		"valueText":   ValueText,
	})
}
