package browser

import (
	"embed"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/pagescript"
)

// scriptFS holds this package's page-side JavaScript.
//
// These live as .js files rather than Go string constants so they get editor
// support and stay readable, and — more importantly — so nothing has to be
// escaped twice. Building a script by concatenating Go constants into a quoted
// string is how selectors end up mangled.
//
//go:embed scripts/*.js
var scriptFS embed.FS

// pageScripts are the generic probes this package runs. Site-specific
// extraction lives in its own set, under the package that owns those selectors.
var pageScripts = pagescript.Load(scriptFS, "scripts")
