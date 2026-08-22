// Package gmail reads OTP mail from one mailbox, read-only.
//
// Authentication is a user OAuth credential, not a service account. Domain-wide
// delegation is a Google Workspace feature, so a service account — including one
// reached from GitHub Actions via Workload Identity Federation — cannot read a
// consumer @gmail.com inbox at all. A long-lived user refresh token is the only
// way in, which is why [NewFromJSON] takes the authorized_user JSON that
// `gcloud auth application-default login` produces.
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/auth/credentials"

	credential "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/credential"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Scope is the only permission this project needs. Read-only on purpose: the
// credential ends up in CI, and nothing here has any reason to modify mail.
const Scope = gmailapi.GmailReadonlyScope

// mailbox is the authenticated user's own mailbox.
const mailbox = "me"

// Client reads messages from one mailbox.
type Client struct {
	svc *gmailapi.Service
}

// NewFromCredential builds a client from a credential this program obtained
// itself, without going back through the JSON it is stored as.
func NewFromCredential(ctx context.Context, cred credential.Gmail) (*Client, error) {
	blob, err := json.Marshal(map[string]string{
		"type":          "authorized_user",
		"client_id":     cred.ClientID,
		"client_secret": cred.ClientSecret,
		"refresh_token": cred.RefreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("gmail: encode credential: %w", err)
	}
	return NewFromJSON(ctx, blob)
}

// NewFromJSON builds a client from an authorized_user credentials blob — the CI
// path, where the same JSON is held as a secret.
func NewFromJSON(ctx context.Context, credentialsJSON []byte) (*Client, error) {
	// Named as authorized_user rather than detected. This program has exactly one
	// kind of credential — a personal mailbox's refresh token, because domain-wide
	// delegation is Workspace-only — and detection would accept any of the other
	// kinds from whatever set GMAIL_CREDENTIALS. An external_account blob, in
	// particular, carries the URLs it is fetched from. Saying the type here makes
	// anything else a parse error.
	creds, err := credentials.NewCredentialsFromJSON(credentials.AuthorizedUser, credentialsJSON, &credentials.DetectOptions{
		Scopes: []string{Scope},
	})
	if err != nil {
		return nil, fmt.Errorf("gmail: parse credentials: %w", err)
	}
	svc, err := gmailapi.NewService(ctx, option.WithAuthCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("gmail: build service: %w", err)
	}
	return &Client{svc: svc}, nil
}

// Profile returns the address of the mailbox the credentials actually open.
//
// Worth calling before anything else: it is the cheapest way to find out both
// that the credential works and that it belongs to the account you expected.
func (c *Client) Profile(ctx context.Context) (string, error) {
	p, err := c.svc.Users.GetProfile(mailbox).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gmail: get profile: %w", explainDeadCredential(err))
	}
	return p.EmailAddress, nil
}

// Message is the subset of a Gmail message this project needs.
type Message struct {
	ID string

	// Received is the server's own receive time (internalDate). Freshness is
	// judged on this rather than on the Date header, which is written by the
	// sender and can be wrong or skewed.
	Received time.Time

	From    string
	Subject string
	Body    string
}

// Search returns up to max messages matching a Gmail query, newest first.
//
// The query is Gmail's own search syntax, e.g.
// `from:noreply@example.com newer_than:1h`. Narrowing it there rather than
// filtering afterwards keeps the number of message fetches down.
func (c *Client) Search(ctx context.Context, query string, max int64) ([]Message, error) {
	list, err := c.svc.Users.Messages.List(mailbox).
		Q(query).
		MaxResults(max).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("gmail: list messages: %w", explainDeadCredential(err))
	}

	out := make([]Message, 0, len(list.Messages))
	for _, ref := range list.Messages {
		full, err := c.svc.Users.Messages.Get(mailbox, ref.Id).
			Format("full").
			Context(ctx).
			Do()
		if err != nil {
			return nil, fmt.Errorf("gmail: get message %s: %w", ref.Id, explainDeadCredential(err))
		}
		payload := messagePayload{message: full}
		out = append(out, Message{
			ID:       full.Id,
			Received: time.UnixMilli(full.InternalDate),
			From:     payload.header("From"),
			Subject:  payload.header("Subject"),
			Body:     payload.text(),
		})
	}
	return out, nil
}

// messagePayload reads the parts of one message.
//
// A type rather than a handful of package-level helpers: "body", "header" and
// "collect" say nothing about what they operate on, and at package scope they
// are in reach of every file here.
type messagePayload struct {
	message *gmailapi.Message
}

// header returns a header's value, matched case-insensitively.
func (p messagePayload) header(name string) string {
	if p.message.Payload == nil {
		return ""
	}
	for _, h := range p.message.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// text returns the message body, preferring text/plain.
//
// Both kinds are gathered rather than stopping at the first match: a multipart
// message can carry the code in either, and OTP mail is frequently HTML-only.
func (p messagePayload) text() string {
	var plain, html strings.Builder
	p.appendText(p.message.Payload, &plain, &html)
	if plain.Len() > 0 {
		return plain.String()
	}
	return html.String()
}

// appendText walks the part tree, accumulating each kind separately.
func (p messagePayload) appendText(part *gmailapi.MessagePart, plain, html *strings.Builder) {
	if part == nil {
		return
	}
	if part.Body != nil && part.Body.Data != "" {
		decoded, err := decodeBase64URL(part.Body.Data)
		if err != nil {
			// Never silently: an undecodable body reads downstream as "the mail
			// had no code in it", which sends the caller looking in the wrong
			// place entirely.
			log.Printf("WARN: gmail: decode %s part: %v", part.MimeType, err)
		} else {
			switch {
			case strings.HasPrefix(part.MimeType, "text/plain"):
				plain.Write(decoded)
			case strings.HasPrefix(part.MimeType, "text/html"):
				html.Write(decoded)
			}
		}
	}
	for _, sub := range part.Parts {
		p.appendText(sub, plain, html)
	}
}

// decodeBase64URL decodes Gmail's web-safe base64 payload.
//
// The API documents base64url, but whether the padding is present varies, and a
// strict decoder rejects the variant it was not built for. Both are tried rather
// than assuming one — assuming produced empty bodies for every message.
func decodeBase64URL(data string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(data, "=")); err == nil {
		return decoded, nil
	}
	decoded, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("not valid base64url (%d bytes): %w", len(data), err)
	}
	return decoded, nil
}
