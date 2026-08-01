package gmail

import (
	"encoding/base64"
	"strings"
	"testing"

	gmailapi "google.golang.org/api/gmail/v1"
)

// This package had no tests, and the part that reads a message body is where a
// mistake is quietest: an undecodable or unfound body is indistinguishable
// downstream from "the mail did not contain a code", which sends whoever is
// debugging it to the OTP regexp, or to the mailbox, or to the login form —
// anywhere but here. That already happened once, when a padding assumption
// decoded every body to nothing.

// part builds one MIME part with a base64url-encoded body, padded or not.
func part(mime, body string, padded bool) *gmailapi.MessagePart {
	enc := base64.RawURLEncoding.EncodeToString([]byte(body))
	if padded {
		enc = base64.URLEncoding.EncodeToString([]byte(body))
	}
	return &gmailapi.MessagePart{MimeType: mime, Body: &gmailapi.MessagePartBody{Data: enc}}
}

func TestDecodeBase64URLAcceptsBothPaddings(t *testing.T) {
	// Chosen so the encoding needs padding: 4 bytes -> 6 base64 chars + "==".
	const body = "認証コード:AB-123456"

	for name, encoded := range map[string]string{
		"padded":   base64.URLEncoding.EncodeToString([]byte(body)),
		"unpadded": base64.RawURLEncoding.EncodeToString([]byte(body)),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeBase64URL(encoded)
			if err != nil {
				t.Fatalf("decodeBase64URL() error = %v", err)
			}
			if string(got) != body {
				t.Errorf("decodeBase64URL() = %q, want %q", got, body)
			}
		})
	}
}

// TestDecodeBase64URLUsesTheWebSafeAlphabet is the reason it is not plain
// base64: Gmail sends - and _ where standard base64 sends + and /.
func TestDecodeBase64URLUsesTheWebSafeAlphabet(t *testing.T) {
	raw := []byte{0xfb, 0xef, 0xbe}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if !strings.ContainsAny(encoded, "-_") {
		t.Fatalf("%q does not exercise the web-safe characters", encoded)
	}
	got, err := decodeBase64URL(encoded)
	if err != nil {
		t.Fatalf("decodeBase64URL(%q) error = %v", encoded, err)
	}
	if string(got) != string(raw) {
		t.Errorf("decodeBase64URL() = %x, want %x", got, raw)
	}
}

func TestDecodeBase64URLReportsGarbage(t *testing.T) {
	if _, err := decodeBase64URL("not base64 !!!"); err == nil {
		t.Fatal("decodeBase64URL() accepted a body that is not base64")
	}
}

func TestMessagePayloadHeaderIsCaseInsensitive(t *testing.T) {
	p := messagePayload{message: &gmailapi.Message{Payload: &gmailapi.MessagePart{
		Headers: []*gmailapi.MessagePartHeader{
			{Name: "from", Value: "no-reply@cs.paypay-sec.co.jp"},
			{Name: "Subject", Value: "認証コードのお知らせ"},
		},
	}}}

	// Gmail's casing is not guaranteed, and matching exactly would drop a header
	// silently rather than fail.
	if got := p.header("From"); got != "no-reply@cs.paypay-sec.co.jp" {
		t.Errorf("header(\"From\") = %q", got)
	}
	if got := p.header("subject"); got != "認証コードのお知らせ" {
		t.Errorf("header(\"subject\") = %q", got)
	}
	if got := p.header("Reply-To"); got != "" {
		t.Errorf("header() of an absent header = %q, want empty", got)
	}
}

func TestMessagePayloadHeaderOnAnEmptyMessage(t *testing.T) {
	if got := (messagePayload{message: &gmailapi.Message{}}).header("From"); got != "" {
		t.Errorf("header() with no payload = %q", got)
	}
}

// TestMessagePayloadTextPrefersPlain covers the ordinary multipart/alternative
// shape: the same code in both parts, and the one without markup wins.
func TestMessagePayloadTextPrefersPlain(t *testing.T) {
	p := messagePayload{message: &gmailapi.Message{Payload: &gmailapi.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmailapi.MessagePart{
			part("text/plain", "認証コード:AB-123456", false),
			part("text/html", "<p>認証コード:AB-999999</p>", true),
		},
	}}}

	if got := p.text(); got != "認証コード:AB-123456" {
		t.Errorf("text() = %q, want the plain part", got)
	}
}

// TestMessagePayloadTextFallsBackToHTML is the case that matters in practice:
// MoneyForward's OTP mail has no plain part at all.
func TestMessagePayloadTextFallsBackToHTML(t *testing.T) {
	p := messagePayload{message: &gmailapi.Message{Payload: &gmailapi.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmailapi.MessagePart{
			part("text/html", "<p>123456</p>", false),
		},
	}}}

	if got := p.text(); got != "<p>123456</p>" {
		t.Errorf("text() = %q, want the HTML part", got)
	}
}

// TestMessagePayloadTextWalksNestedParts guards against reading only the top
// level. multipart/mixed wrapping multipart/alternative is routine, and stopping
// at depth one finds nothing while looking like an empty mail.
func TestMessagePayloadTextWalksNestedParts(t *testing.T) {
	p := messagePayload{message: &gmailapi.Message{Payload: &gmailapi.MessagePart{
		MimeType: "multipart/mixed",
		Parts: []*gmailapi.MessagePart{
			{
				MimeType: "multipart/alternative",
				Parts: []*gmailapi.MessagePart{
					part("text/plain", "認証コード:AB-112233", false),
				},
			},
			part("application/pdf", "not text", false),
		},
	}}}

	if got := p.text(); got != "認証コード:AB-112233" {
		t.Errorf("text() = %q, want the nested plain part", got)
	}
}

// TestMessagePayloadTextIgnoresOtherMimeTypes keeps an attachment's bytes out of
// the body the code is searched in.
func TestMessagePayloadTextIgnoresOtherMimeTypes(t *testing.T) {
	p := messagePayload{message: &gmailapi.Message{Payload: &gmailapi.MessagePart{
		MimeType: "multipart/mixed",
		Parts: []*gmailapi.MessagePart{
			part("application/octet-stream", "999999", false),
			part("text/plain", "123456", false),
		},
	}}}

	if got := p.text(); got != "123456" {
		t.Errorf("text() = %q, want only the text part", got)
	}
}

// TestMessagePayloadTextOnASinglePartMessage covers a mail with no Parts at all,
// where the body hangs directly off the payload.
func TestMessagePayloadTextOnASinglePartMessage(t *testing.T) {
	p := messagePayload{message: &gmailapi.Message{Payload: part("text/plain", "123456", false)}}
	if got := p.text(); got != "123456" {
		t.Errorf("text() = %q", got)
	}
}

// TestMessagePayloadTextSurvivesAnUndecodableBody checks the whole message is
// not lost to one bad part — and that a decodable sibling still comes through.
func TestMessagePayloadTextSurvivesAnUndecodableBody(t *testing.T) {
	p := messagePayload{message: &gmailapi.Message{Payload: &gmailapi.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmailapi.MessagePart{
			{MimeType: "text/plain", Body: &gmailapi.MessagePartBody{Data: "!!! not base64 !!!"}},
			part("text/html", "<p>123456</p>", false),
		},
	}}}

	if got := p.text(); got != "<p>123456</p>" {
		t.Errorf("text() = %q, want the part that could be read", got)
	}
}

func TestMessagePayloadTextOnAnEmptyMessage(t *testing.T) {
	if got := (messagePayload{message: &gmailapi.Message{}}).text(); got != "" {
		t.Errorf("text() with no payload = %q", got)
	}
}
