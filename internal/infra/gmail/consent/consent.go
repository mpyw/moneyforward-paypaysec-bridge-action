package consent

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/credential"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/gmail"
)

const (
	// consentTimeout bounds how long the loopback server waits for the browser.
	consentTimeout = 5 * time.Minute
)

// desktopClient is the shape of the console's download.
type desktopClient struct {
	Installed struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
}

// authorizedUser is the credential format Google's libraries accept, and what
// `gcloud auth application-default login` writes.
type authorizedUser struct {
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// Flow is the OAuth consent flow for a desktop client.
//
// Deliberately not `gcloud auth application-default login`: gcloud issues its
// credentials with cloud-platform, so one leaked out of CI would authorize
// operating the whole Google Cloud project rather than reading mail. This asks for
// [gmail.Scope] and nothing else.
type Flow struct {
	// ClientFile is the OAuth client downloaded from the Google Cloud console —
	// the "Desktop app" type, whose redirect is a loopback address.
	ClientFile string

	// Announce, if set, is shown the URL to visit. Defaults to stderr.
	Announce func(string)
}

// Obtain runs the flow and returns what Google granted.
func (f Flow) Obtain(ctx context.Context) (credential.Gmail, error) {
	var none credential.Gmail

	raw, err := os.ReadFile(f.ClientFile)
	if err != nil {
		return none, fmt.Errorf("read OAuth client %s: %w", f.ClientFile, err)
	}
	var dc desktopClient
	if err := json.Unmarshal(raw, &dc); err != nil {
		return none, fmt.Errorf("parse %s: %w", f.ClientFile, err)
	}
	if dc.Installed.ClientID == "" {
		return none, fmt.Errorf("%s has no \"installed\" section — it must be an OAuth "+
			"client of type Desktop app", f.ClientFile)
	}

	// Bind first, so the redirect URI can name the port the browser will hit.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return none, fmt.Errorf("open loopback listener: %w", err)
	}
	defer func() { _ = listener.Close() }()
	redirect := fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	conf := &oauth2.Config{
		ClientID:     dc.Installed.ClientID,
		ClientSecret: dc.Installed.ClientSecret,
		Endpoint:     googleoauth.Endpoint,
		RedirectURL:  redirect,
		Scopes:       []string{gmail.Scope},
	}

	state, err := randomState()
	if err != nil {
		return none, err
	}
	// AccessTypeOffline asks for a refresh token; ApprovalForce makes Google
	// issue a new one even if this account has consented before, which it
	// otherwise skips — leaving the flow "successful" but useless.
	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	code, err := f.awaitCode(ctx, listener, state, authURL)
	if err != nil {
		return none, err
	}

	token, err := conf.Exchange(ctx, code)
	if err != nil {
		return none, fmt.Errorf("exchange authorization code: %w", err)
	}
	return credential.Gmail{
		ClientID:     conf.ClientID,
		ClientSecret: conf.ClientSecret,
		RefreshToken: token.RefreshToken,
	}, nil
}

// announce shows the user something, through the caller's hook when there is
// one so a test can read it back.
func (f Flow) announce(msg string) {
	if f.Announce != nil {
		f.Announce(msg)
		return
	}
	_, _ = fmt.Fprint(os.Stderr, msg)
}

// File stores a credential as the authorized_user JSON the Google libraries
// read.
type File struct{ Path string }

// Store writes the credential, readable only by its owner.
func (f File) Store(_ context.Context, cred credential.Gmail) error {
	blob, err := json.MarshalIndent(authorizedUser{
		Type:         "authorized_user",
		ClientID:     cred.ClientID,
		ClientSecret: cred.ClientSecret,
		RefreshToken: cred.RefreshToken,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credential: %w", err)
	}
	// 0600: a long-lived key to a mailbox.
	if err := os.WriteFile(f.Path, append(blob, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", f.Path, err)
	}
	return nil
}

func (f Flow) awaitCode(ctx context.Context, listener net.Listener, state, authURL string) (string, error) {
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)

	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if got := q.Get("state"); got != state {
				http.Error(w, "state mismatch", http.StatusBadRequest)
				done <- result{err: fmt.Errorf("state mismatch: the redirect did not come from the request we made")}
				return
			}
			if e := q.Get("error"); e != "" {
				http.Error(w, "authorization denied: "+e, http.StatusBadRequest)
				done <- result{err: fmt.Errorf("authorization denied: %s", e)}
				return
			}
			code := q.Get("code")
			if code == "" {
				http.Error(w, "no code in redirect", http.StatusBadRequest)
				done <- result{err: fmt.Errorf("redirect carried no authorization code")}
				return
			}
			_, _ = fmt.Fprintln(w, "Authorized. You can close this tab and return to the terminal.")
			done <- result{code: code}
		}),
	}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	f.announce(fmt.Sprintf(
		"Opening the consent page. If it does not open, visit:\n\n%s\n\n", authURL))
	openBrowser(authURL)

	ctx, cancel := context.WithTimeout(ctx, consentTimeout)
	defer cancel()
	select {
	case res := <-done:
		return res.code, res.err
	case <-ctx.Done():
		return "", fmt.Errorf("timed out waiting for the consent redirect: %w", ctx.Err())
	}
}

// randomState guards against a redirect this process did not initiate.
func randomState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// openBrowser is best effort; the URL is printed either way.
func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, url).Start()
}
