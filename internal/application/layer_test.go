// Package application groups the half of this program that decides things: the
// domain, the use cases, and the interfaces they are served through.
//
// It exists to make one rule visible and checkable — nothing under here may
// import internal/infra or internal/cli. Everything that touches a browser, a
// site, a mailbox or a terminal lives on the other side of that line and reaches
// this one by implementing an interface in application/port.
package application

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/mpyw/moneyforward-paypaysec-bridge-action"

// forbidden is what the application layer may not depend on, and why.
var forbidden = map[string]string{
	modulePath + "/internal/infra": "infra is where the side effects are; the " +
		"application layer is reached through an interface, not the other way round",
	modulePath + "/internal/cli": "cli is a delivery mechanism, and a use case " +
		"must not know which one invoked it",
}

// TestApplicationDoesNotDependOnInfrastructure is the layering rule as a test.
//
// A rule stated only in a doc comment is one that gets broken by an import
// someone added without thinking about it. That is how the sync use case came to
// declare its own ports in terms of a scraper's struct, and how PayPay 証券's
// target table came to carry MoneyForward's numbering: both compiled, both
// passed, and neither looked wrong at the line where it was written.
func TestApplicationDoesNotDependOnInfrastructure(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		pkg, perr := build.ImportDir(path, 0)
		if perr != nil {
			// A directory with no Go files is not a package; nothing to check.
			return nil
		}
		checked++
		rel, _ := filepath.Rel(root, path)

		// Test imports as well: they compile into the same package, and a test
		// reaching for infra would make the layer depend on it just as surely.
		imports := append(append([]string{}, pkg.Imports...), pkg.TestImports...)
		for _, imported := range imports {
			for prefix, why := range forbidden {
				if imported == prefix || strings.HasPrefix(imported, prefix+"/") {
					t.Errorf("application/%s imports %s\n    %s", rel, imported, why)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Guards the walk itself: a rule that silently checks nothing is worse than
	// no rule, because it reads as a passing check.
	if checked < 5 {
		t.Fatalf("only %d packages were examined; the walk is not finding them", checked)
	}
}
