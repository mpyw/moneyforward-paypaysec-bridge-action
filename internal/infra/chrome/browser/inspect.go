package browser

import (
	"fmt"

	"github.com/chromedp/chromedp"
)

// Element is an interactive element discovered on a page, described well enough
// to turn into a selector constant.
type Element struct {
	// Selector is a best-effort CSS selector for the element, preferring an id,
	// then a distinctive attribute, then a nth-of-type path.
	Selector string `json:"selector"`

	// Tag, Role and Text describe it well enough to recognise which control it
	// is without opening the page.
	Tag  string `json:"tag"`
	Role string `json:"role"`
	Text string `json:"text"`

	// Visible is false for elements present but not rendered — an inactive tab
	// panel's controls, for instance.
	Visible bool `json:"visible"`
}

// FindInteractive lists the tab-like and button-like controls on the current
// page.
//
// This is discovery tooling for the debug command: when a selector constant is
// still a guess, the fastest way to replace it with a real one is to ask the
// page what it actually has, rather than reading a 20,000-line HTML dump.
func (p Page) FindInteractive() ([]Element, error) {
	ctx := p.ctx
	js, err := pageScripts.Call("find_interactive.js")
	if err != nil {
		return nil, err
	}
	var els []Element
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &els)); err != nil {
		return nil, fmt.Errorf("enumerate interactive elements: %w", err)
	}
	return els, nil
}
