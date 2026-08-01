package cookiestore

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// mustParseURL fails the test rather than returning an error nobody would act on.
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return u
}

// containsString reports membership without pulling in a dependency.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// storedCookie is shorthand for a stored storedCookie record.
func storedCookie(name, domain, path string, secure bool) SessionCookie {
	return SessionCookie{Name: name, Value: "v-" + name, Domain: domain, Path: path, Secure: secure}
}

// TestStoreHTTPClientCarriesEveryOrigin covers the failure that made every
// MoneyForward page render signed-out: a session spanning two sub-domains, of
// which only one was ever seeded.
func TestStoreHTTPClientCarriesEveryOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	blob, err := json.Marshal([]SessionCookie{
		storedCookie("app_session", "moneyforward.com", "/", true),
		storedCookie("id_session", "id.moneyforward.com", "/", true),
		storedCookie("wide", ".moneyforward.com", "/", true),
	})
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	client, err := Store{Path: path}.HTTPClient()
	if err != nil {
		t.Fatalf("HTTPClient() error = %v", err)
	}

	for _, tc := range []struct{ url, want string }{
		{"https://moneyforward.com/accounts", "app_session"},
		{"https://id.moneyforward.com/me", "id_session"},
	} {
		u := mustParseURL(t, tc.url)
		var names []string
		for _, c := range client.Jar.Cookies(u) {
			names = append(names, c.Name)
		}
		if !containsString(names, tc.want) {
			t.Errorf("%s carries %v, missing %q", tc.url, names, tc.want)
		}
	}
}

func TestStoreHTTPClientReportsAMissingFile(t *testing.T) {
	_, err := Store{Path: filepath.Join(t.TempDir(), "absent.json")}.HTTPClient()
	if err == nil {
		t.Fatal("HTTPClient() succeeded with no file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want it to wrap os.ErrNotExist", err)
	}
}

// TestSessionCookieOrigin covers how a host-only storedCookie is addressed on restore.
// Getting this wrong drops __Host- prefixed cookies silently.
func TestSessionCookieOrigin(t *testing.T) {
	tests := []struct {
		name         string
		storedCookie SessionCookie
		want         string
	}{
		{"host-only secure", storedCookie("a", "moneyforward.com", "/", true), "https://moneyforward.com/"},
		{"insecure", storedCookie("a", "example.test", "/", false), "http://example.test/"},
		{"leading dot is stripped", storedCookie("a", ".example.test", "/x", true), "https://example.test/x"},
		{"empty path becomes root", storedCookie("a", "example.test", "", true), "https://example.test/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.storedCookie.Origin(); got != tt.want {
				t.Errorf("Origin() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionCookieHostOnly(t *testing.T) {
	if !storedCookie("a", "moneyforward.com", "/", true).hostOnly() {
		t.Error("a domain with no leading dot is host-only")
	}
	if storedCookie("a", ".moneyforward.com", "/", true).hostOnly() {
		t.Error("a leading dot marks a domain storedCookie, not a host-only one")
	}
	if got := storedCookie("a", ".moneyforward.com", "/", true).Host(); got != "moneyforward.com" {
		t.Errorf("Host() = %q", got)
	}
}
