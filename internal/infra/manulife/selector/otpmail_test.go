package selector

import (
	"strings"
	"testing"
)

// TestOTPCodePattern checks the extraction against the shape of a real message.
//
// Exercised through OTPMail rather than the pattern variable directly, so the
// test covers what is actually wired into the shared OTP source.
//
// The message is HTML, and the code sits between a label and a <br/> with
// whitespace on both sides — so the pattern has to tolerate that without
// reaching past it to the call-centre numbers further down.
func TestOTPCodePattern(t *testing.T) {
	// The spaces around the code are U+3000, exactly as the real message has
	// them. Written as an escape rather than pasted, so that what this test
	// asserts is visible to whoever reads it — a pasted ideographic space looks
	// like an ordinary one and a reviewer cannot tell them apart.
	const ideographicSpace = "\u3000"
	const body = `<!DOCTYPE html><html lang="ja"><body>
マイページをご利用いただき、ありがとうございます。<br/><br/>
ログイン用ワンタイムパスワードを元の画面に戻りご入力ください。<br/><br/>
ワンタイムパスワード：` + ideographicSpace + ideographicSpace + `112233` +
		ideographicSpace + ideographicSpace + `<br/><br/>
※発行から10分間のみ有効です<br/><br/>
＜コールセンター＞<br />
0120-100-100 <br />
受付時間 平日 9:00 ～17:00<br />
</body></html>`

	m := OTPMail.Pattern.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("OTPMail.Pattern found no code in the message body")
	}
	if m[1] != "112233" {
		t.Errorf("extracted %q, want %q", m[1], "112233")
	}
}

func TestOTPCodePatternVariants(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"full width colon", "ワンタイムパスワード： 112233", "112233"},
		// The one the live mail actually uses. Go's \s is ASCII, so a pattern
		// written with \s* matches every case here except this one — and this
		// one is the only case that occurs.
		{"ideographic space", "ワンタイムパスワード：\u3000\u3000112233", "112233"},
		{"non breaking space", "ワンタイムパスワード：\u00a0112233", "112233"},
		{"half width colon", "ワンタイムパスワード:112233", "112233"},
		{"no space", "ワンタイムパスワード：112233", "112233"},
		{"newline after colon", "ワンタイムパスワード：\n112233", "112233"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := OTPMail.Pattern.FindStringSubmatch(tt.body)
			if len(m) < 2 || m[1] != tt.want {
				t.Errorf("OTPMail.Pattern on %q = %v, want %q", tt.body, m, tt.want)
			}
		})
	}
}

// TestOTPCodePatternRefusesTheLoginNotice is the whole reason the pattern is
// anchored to a label.
//
// 【マニュライフ生命マイページ】ログインのお知らせ arrives from the same address
// thirteen seconds after the code — CONFIRMED 2026-08-29 — so it is inside the
// freshness window and reaches the pattern. It carries a postcode and an
// address, and no labelled code.
func TestOTPCodePatternRefusesTheLoginNotice(t *testing.T) {
	const body = `テスト 太郎 様 いつもマイページをご利用いただき、ありがとうございます。
マイページにログインされましたので、確認のためメールをお送りしております。
発行元：マニュライフ生命保険株式会社
〒 123-4567 東京都新宿区西新宿0-00-0 テストタワー00階
`

	if m := OTPMail.Pattern.FindStringSubmatch(body); len(m) >= 2 {
		t.Errorf("the login notice yielded %q as a code — the pattern is what "+
			"keeps it from reaching the form", m[1])
	}
}

// TestOTPCodePatternIgnoresPhoneNumbers guards the other half of the same
// message: the code mail itself lists two call-centre numbers.
func TestOTPCodePatternIgnoresPhoneNumbers(t *testing.T) {
	const body = "＜コールセンター＞<br />0120-100-100<br />0120-999-999<br />"
	if m := OTPMail.Pattern.FindStringSubmatch(body); m != nil {
		t.Errorf("matched %q in a message with no code", m[0])
	}
}

// TestOTPMailQueryScopedToSender mirrors the checks on the other two services.
//
// The subject is a localized string, and MoneyForward's turned out to depend on
// where the login came from. The sender is also the one part of this service's
// mail that is easy to get wrong in the other direction: it is on manulife.com,
// while the site is served from manulife.co.jp.
func TestOTPMailQueryScopedToSender(t *testing.T) {
	if strings.Contains(OTPMail.Query, "subject:") {
		t.Errorf("Query = %q — discrimination belongs to the pattern", OTPMail.Query)
	}
	if !strings.Contains(OTPMail.Query, "from:") {
		t.Errorf("Query = %q, want it scoped to the sender", OTPMail.Query)
	}
	if !strings.Contains(OTPMail.Query, "@manulife.com") {
		t.Errorf("Query = %q — the code comes from manulife.com, not the "+
			"manulife.co.jp the site is served from", OTPMail.Query)
	}
}
