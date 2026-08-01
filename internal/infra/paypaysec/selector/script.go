package selector

import (
	"embed"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/pagescript"
)

// scripts holds the page-side JavaScript for this site.
//
// Keeping the extraction in a .js file rather than a Go string constant is what
// lets the selectors below be passed in as an argument: the previous version
// spliced them into the source with backtick concatenation, which is fragile
// and unreadable for no benefit.
//
//go:embed js/*.js
var scriptFS embed.FS

// siteScripts are this site's extraction routines. Its own set, so a name here
// cannot collide with the browser layer's generic probes.
var siteScripts = pagescript.Load(scriptFS, "js")

// extractBalanceJS is read once: a missing file is a build mistake, so it should
// surface at init rather than on the first scrape.
const extractBalanceScript = "extract_balance.js"

// selectTabJS is read once, for the same reason as extractBalanceJS.
const selectTabScript = "select_tab.js"

// SelectTab renders the tab-activation call for one label.
func SelectTab(label string) (string, error) {
	return siteScripts.Call(selectTabScript, map[string]string{
		"container":   TabMenu,
		"label":       label,
		"activeClass": TabActiveClass,
	})
}

// activeTabJS is read once, for the same reason as extractBalanceJS.
const activeTabScript = "active_tab.js"

// ActiveTab renders the call that reports the currently active tab.
func ActiveTab() (string, error) {
	return siteScripts.Call(activeTabScript, map[string]string{
		"container":   TabMenu,
		"activeClass": TabActiveClass,
	})
}

// pageStateJS is read once, for the same reason as extractBalanceJS.
const pageStateScript = "page_state.js"

// PageState renders the load-state probe, watching the element that carries the
// figure the caller is about to read.
//
// Which element that is differs by page, and this used to be hard-coded to
// [ValueTotal]. On a 銘柄's own page that element does not exist — its figure is
// [HoldingValue] — so the probe reported "not present" for the full timeout,
// every detail page burned twenty seconds finding nothing, and the read that
// followed had no settle guarantee at all. Which is where it was needed: a
// placeholder gain there becomes an acquisition cost equal to the valuation,
// and 評価損益 of exactly zero.
func PageState(value string) (string, error) {
	return siteScripts.Call(pageStateScript, map[string]string{
		"loading": LoadingOverlay,
		"total":   value,
	})
}

// extractHoldingJS is read once, for the same reason as extractBalanceJS.
const extractHoldingScript = "extract_holding.js"

// ExtractHolding renders the per-銘柄 detail extraction call.
func ExtractHolding() (string, error) {
	return siteScripts.Call(extractHoldingScript, map[string]string{
		"value":       HoldingValue,
		"acquisition": HoldingAcquisition,
		"gain":        HoldingGain,
	})
}

// ExtractBalance renders the extraction call, passing the current cell
// selectors across as JSON.
func ExtractBalance() (string, error) {
	return siteScripts.Call(extractBalanceScript, map[string]string{
		"total":       ValueTotal,
		"acquisition": Acquisition,
		"gain":        GrossProfit,
		"heading":     HoldingsHeadingTag,
		"headingText": HoldingsHeading,
		"container":   HoldingsContainer,
		"row":         HoldingRow,
		"name":        HoldingName,
		"invest":      BrandInvest,
		"gain_cell":   BrandGain,
	})
}
