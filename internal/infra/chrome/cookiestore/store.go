package cookiestore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// SessionCookie is one cookie as this project stores it.
//
// Deliberately not cdproto's network.Cookie. That type carries CDP enums with no
// valid zero value, so a record missing one fails to decode and takes the whole
// session with it — the on-disk format should not be hostage to a wire protocol's
// stricter corners. These fields are also the only ones a restore needs.
type SessionCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
	SameSite string `json:"sameSite,omitempty"`
}

// hostOnly reports whether the cookie belongs to exactly one host.
//
// Chrome marks a domain cookie with a leading dot and a host-only one without.
// The distinction decides how a restore has to address it.
func (c SessionCookie) hostOnly() bool { return !strings.HasPrefix(c.Domain, ".") }

// Host is the cookie's domain without the domain-cookie marker.
func (c SessionCookie) Host() string { return strings.TrimPrefix(c.Domain, ".") }

// Origin reconstructs the origin a host-only cookie belongs to.
func (c SessionCookie) Origin() string {
	scheme := "http://"
	if c.Secure {
		scheme = "https://"
	}
	path := c.Path
	if path == "" {
		path = "/"
	}
	return scheme + c.Host() + path
}

// CookieSet is a captured session.
//
// A type rather than a package-level clientFor/decodeCookies/fromChrome trio:
// those names say nothing about what they convert, and at package scope they
// were reachable from every file here.
type CookieSet []SessionCookie

// cookieSetFromChrome converts what the browser reported into what is stored.
func cookieSetFromChrome(cookies []*network.Cookie) CookieSet {
	out := make(CookieSet, 0, len(cookies))
	for _, c := range cookies {
		out = append(out, SessionCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite.String(),
		})
	}
	return out
}

// Store persists a browser session to a file.
//
// A type rather than SaveCookies/LoadCookies each taking a st.Path: the st.Path is the
// one thing both need, and the one thing a caller must not get inconsistent
// between them.
type Store struct {
	// Path is the file the session is written to and read from.
	//
	// SECURITY: it holds live brokerage session cookies. Keep it under a
	// gitignored directory and delete it when finished.
	Path string
}

// Save writes the browser's cookies to the store.
//
// A persisted Chrome profile is not enough on its own: PayPay 証券's auth cookie
// is a session cookie, which lives in memory and is gone the moment the browser
// exits. Reading the cookies out of the live browser and writing them ourselves
// is what lets a login be reused by a later process — which is the difference
// between iterating on the scrape freely and needing a human and a one-time code
// for every attempt.
//
// SECURITY: the file is a bearer credential for a brokerage account. It is
// written 0600 under the gitignored debug directory, and should be deleted when
// the work is done.
func (st Store) Save(ctx context.Context) (int, error) {
	// Storage.getCookies, not Network.getCookies: the latter returns only the
	// cookies visible to the current page's own URLs. MoneyForward's session
	// spans moneyforward.com and id.moneyforward.com, so capturing from
	// whichever page happens to be open silently leaves half the session behind
	// — and the restore then looks successful while the site renders
	// signed-out.
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		c, err := storage.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		cookies = c
		return nil
	})); err != nil {
		return 0, fmt.Errorf("read cookies: %w", err)
	}

	stored := cookieSetFromChrome(cookies)
	blob, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("encode cookies: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(st.Path), 0o700); err != nil {
		return 0, fmt.Errorf("create cookie dir: %w", err)
	}
	if err := os.WriteFile(st.Path, blob, 0o600); err != nil {
		return 0, fmt.Errorf("write cookies: %w", err)
	}
	return len(cookies), nil
}

// decodeCookieSet reads the stored form.
func decodeCookieSet(blob []byte, source string) (CookieSet, error) {
	var cookies CookieSet
	if err := json.Unmarshal(blob, &cookies); err != nil {
		return nil, fmt.Errorf("decode %s: %w", source, err)
	}
	return cookies, nil
}

// Params renders the set for Storage.setCookies.
//
// Host-only cookies are addressed by URL, not by domain. Setting Domain on one
// turns it into a domain cookie, and for a __Host- prefixed name that is not
// merely different but invalid — the browser rejects it outright. The restore
// then "succeeds" while quietly dropping exactly the cookie carrying the
// session, which is how this was found: pages loaded fine and rendered
// signed-out.
func (s CookieSet) Params() []*network.CookieParam {
	params := make([]*network.CookieParam, 0, len(s))
	for _, c := range s {
		param := &network.CookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: network.CookieSameSite(c.SameSite),
			// Expires is deliberately left unset: a session cookie has no
			// expiry, and forcing one would either drop it immediately or
			// outlive what the site intended.
		}
		if c.hostOnly() {
			param.URL = c.Origin()
		} else {
			param.Domain = c.Domain
		}
		params = append(params, param)
	}
	return params
}

// Load restores cookies previously written by [Store.Save].
//
// A missing file is not an error: the common case is simply "no session saved
// yet", and the caller finds out soon enough by landing on a login page.
func (st Store) Load(ctx context.Context) (int, error) {
	blob, err := os.ReadFile(st.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read cookies: %w", err)
	}

	cookies, err := decodeCookieSet(blob, st.Path)
	if err != nil {
		return 0, err
	}
	if len(cookies) == 0 {
		return 0, nil
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return storage.SetCookies(cookies.Params()).Do(ctx)
	})); err != nil {
		return 0, fmt.Errorf("restore cookies: %w", err)
	}
	return len(cookies), nil
}
