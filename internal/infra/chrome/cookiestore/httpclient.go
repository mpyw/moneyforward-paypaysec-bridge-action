package cookiestore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// HTTPSessionTimeout bounds a request made with a borrowed browser session.
const HTTPSessionTimeout = 60 * time.Second

// HTTPClientFor builds an http.Client carrying the running browser's session,
// without going through a file.
func HTTPClientFor(ctx context.Context) (*http.Client, error) {
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		c, err := storage.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		cookies = c
		return nil
	})); err != nil {
		return nil, fmt.Errorf("read cookies: %w", err)
	}
	return cookieSetFromChrome(cookies).HTTPClient()
}

// HTTPClient builds an http.Client carrying the session in the store.
//
// Some pages are far cheaper to read over plain HTTP than through the browser:
// MoneyForward's manual-account page renders fine but keeps a headless renderer
// busy long enough that querying it times out, and none of that work is needed
// to read the markup. The session is the only thing the browser was providing.
func (st Store) HTTPClient() (*http.Client, error) {
	blob, err := os.ReadFile(st.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", st.Path, err)
	}
	cookies, err := decodeCookieSet(blob, st.Path)
	if err != nil {
		return nil, err
	}
	return cookies.HTTPClient()
}

// HTTPClient builds a client carrying this session.
//
// Cookies are grouped by origin because the jar stores them per-URL, and a
// session spanning several sub-domains has to be seeded for each of them —
// seeding only the page that happened to be open is how half a MoneyForward
// session went missing.
func (s CookieSet) HTTPClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	// Group by origin: the jar stores per-URL, and a session spanning several
	// sub-domains has to be seeded for each of them.
	byOrigin := map[string][]*http.Cookie{}
	for _, c := range s {
		origin := c.Origin()
		byOrigin[origin] = append(byOrigin[origin], &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		})
	}
	for origin, cs := range byOrigin {
		u, perr := url.Parse(origin)
		if perr != nil {
			continue
		}
		jar.SetCookies(u, cs)
	}

	return &http.Client{Jar: jar, Timeout: HTTPSessionTimeout}, nil
}
