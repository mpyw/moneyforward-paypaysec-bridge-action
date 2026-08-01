package browser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestNewLaunchesChrome actually starts a browser and loads a page.
//
// The one thing no other test here covers, and the thing that had this project
// failing fifty scheduled runs in a row: the launch flags. GitHub-hosted Linux
// runners restrict unprivileged user namespaces, so a Chrome started without
// --no-sandbox aborts at the zygote before any page loads, and the failure
// surfaced far downstream as a timeout waiting for a selector.
//
// Nothing is fetched over the network — a data: URL is enough to prove Chrome
// started, attached, and rendered — so this is safe to run on every push.
func TestNewLaunchesChrome(t *testing.T) {
	if !chromeInstalled() {
		t.Skip("no Chrome on PATH")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// DefaultsFor(true), not hand-picked options: the point is to exercise the
	// combination the scheduled job will actually use.
	bctx, closeBrowser, err := New(ctx, DefaultsFor(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeBrowser()

	const page = `data:text/html,<title>launch check</title><body><p id=marker>ok</p>`

	var title, marker string
	if err := chromedp.Run(bctx,
		chromedp.Navigate(page),
		chromedp.WaitReady("#marker", chromedp.ByQuery),
		chromedp.Title(&title),
		chromedp.TextContent("#marker", &marker, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("driving the browser failed: %v", err)
	}

	if title != "launch check" {
		t.Errorf("Title() = %q, want the page's own title", title)
	}
	if marker != "ok" {
		t.Errorf("#marker = %q, want %q — Chrome started but did not render", marker, "ok")
	}
}

// TestPageInfoReportsWhereTheBrowserIs covers the reporting every debug command
// leans on when a page is not what was expected.
func TestPageInfoReportsWhereTheBrowserIs(t *testing.T) {
	if !chromeInstalled() {
		t.Skip("no Chrome on PATH")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	bctx, closeBrowser, err := New(ctx, DefaultsFor(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeBrowser()

	if err := PageOf(bctx).Open(`data:text/html,<title>somewhere</title><body>x`); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	info, err := PageOf(bctx).Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Title != "somewhere" {
		t.Errorf("Info().Title = %q", info.Title)
	}
	if info.URL == "" {
		t.Error("Info().URL is empty; a report with no URL cannot explain a redirect")
	}
}

// chromeInstalled reports whether there is a browser to drive.
//
// Skipping rather than failing: this package is also built on machines with no
// Chrome, and a hard failure there would say nothing about the code.
func chromeInstalled() bool {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// TestDumpRecordsWhereThePageWas covers the artefact every debug command leans
// on when a selector stops resolving. Until now only its filename sanitiser was
// tested, and the header — the part that says which page this actually was — was
// exercised nowhere except a live test against MoneyForward.
func TestDumpRecordsWhereThePageWas(t *testing.T) {
	if !chromeInstalled() {
		t.Skip("no Chrome on PATH")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	bctx, closeBrowser, err := New(ctx, DefaultsFor(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeBrowser()

	if err := PageOf(bctx).Open(`data:text/html,<title>dump me</title><body><p id=marker>needle`); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	dir := t.TempDir()
	path, err := PageOf(bctx).Dump(dir, "await/challenge")
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}

	if base := filepath.Base(path); !strings.HasSuffix(base, "-await-challenge.html") {
		t.Errorf("dump written to %q, want the label sanitised into the name", base)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	dump := string(body)

	info, err := PageOf(bctx).Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if !strings.Contains(dump, info.URL) {
		t.Error("the dump does not record the page URL, which is what identifies it afterwards")
	}
	if !strings.Contains(dump, "dump me") {
		t.Error("the dump does not record the page title")
	}
	if !strings.Contains(dump, "needle") {
		t.Error("the dump does not contain the page markup, which is the point of writing one")
	}

	// The file holds whatever was on screen, which in real use is an
	// authenticated page.
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat dump: %v", err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Errorf("dump written %o, want 0600 — it may contain personal data", perm)
	}
}

// TestWithLocationNamesThePage is the annotation working against a real browser.
//
// The unit tests above cover the URL trimming and the nil case; this covers the
// part that matters on the scheduled job — that a selector timeout comes back
// saying which page the selector was missing from.
func TestWithLocationNamesThePage(t *testing.T) {
	if !chromeInstalled() {
		t.Skip("no Chrome on PATH")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	bctx, closeBrowser, err := New(ctx, DefaultsFor(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeBrowser()

	if err := PageOf(bctx).Open(`data:text/html,<title>rejected</title><body>bad credentials`); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	// The failure this is meant to explain: a selector that never shows up.
	_, waitErr := PageOf(bctx).WaitForAny(2*time.Second, map[string]string{"dashboard": "#never"})
	if waitErr == nil {
		t.Fatal("WaitForAny() found #never, which does not exist")
	}

	annotated := PageOf(bctx).WithLocation(waitErr)
	msg := annotated.Error()
	if !strings.Contains(msg, "rejected") {
		t.Errorf("%q does not name the page title, which is what identifies where the wait failed", msg)
	}
	if !strings.Contains(msg, "#never") {
		t.Errorf("%q lost the original error", msg)
	}
	if !errors.Is(annotated, waitErr) {
		t.Error("the original error is no longer unwrappable, so steperr and errors.Is stop working")
	}
}
