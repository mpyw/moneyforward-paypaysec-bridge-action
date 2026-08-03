package moneyforward

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/cookiestore"
)

func fetchCommand() *cli.Command {
	var url string
	return &cli.Command{
		Name:  "fetch",
		Usage: "download one page over HTTP using the saved session, and save it",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "url",
				Usage:       "page to fetch; defaults to the manual account from MONEYFORWARD_ASSET_ID",
				Destination: &url,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runFetch(ctx, session.From(cmd), url)
		},
	}
}

// runFetch downloads a page with the saved session and writes it to the debug
// directory, without starting a browser.
func runFetch(ctx context.Context, opts *session.Options, url string) error {
	if url == "" {
		var err error
		if url, err = accountURL(opts); err != nil {
			return err
		}
	}

	client, err := cookiestore.Store{Path: opts.CookieFile()}.HTTPClient()
	if err != nil {
		return fmt.Errorf("%w\n(run `mfpp debug mf login` first)", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if err := os.MkdirAll(opts.DebugDir(), 0o700); err != nil {
		return err
	}
	path := filepath.Join(opts.DebugDir(), "mf-fetch.html")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stderr, "%s -> %s (%d bytes)\n", url, resp.Status, len(body))
	_, _ = fmt.Fprintf(os.Stderr, "saved: %s\n", path)
	if resp.Request != nil && resp.Request.URL.String() != url {
		_, _ = fmt.Fprintf(os.Stderr, "redirected to: %s\n", resp.Request.URL)
	}
	return nil
}
