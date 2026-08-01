package selector

import (
	"strings"
	"testing"
)

// TestOTPCodePattern checks extraction against the shape of a real message.
//
// The body is full of digits — a timestamp on nearly every line — so the code is
// matched as a line holding nothing else, which is structural, rather than by
// the sentence above it, which is just wording.
func TestOTPCodePattern(t *testing.T) {
	const body = `マネーフォワード IDをご利用いただきありがとうございます。
リスクの高いログイン試行を2026年08月01日 14時21分59秒に検知したため、このメールをお送りしています。

ご自身でのログイン試行である場合は、こちらのコードを入力してログインを継続してください。
112233

この確認コードの有効期限は10分（2026年08月01日 14時31分59秒まで）です。
期限を過ぎた場合は、元の画面に戻っていただき再送信を行ってください。
`

	m := OTPMail.Pattern.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("OTPMail.Pattern found no code in the message body")
	}
	if m[1] != "112233" {
		t.Errorf("extracted %q, want %q", m[1], "112233")
	}
}

func TestOTPCodePatternToleratesLineEndings(t *testing.T) {
	tests := map[string]string{
		"crlf":             "コード\r\n445566\r\nつづき\r\n",
		"trailing spaces":  "コード\n445566  \nつづき\n",
		"leading spaces":   "コード\n  445566\nつづき\n",
		"no trailing line": "コード\n445566",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			m := OTPMail.Pattern.FindStringSubmatch(body)
			if len(m) < 2 || m[1] != "445566" {
				t.Errorf("OTPMail.Pattern on %q = %v, want 445566", body, m)
			}
		})
	}
}

// TestOTPCodePatternIgnoresInlineDigits is the point of the line anchor: the
// timestamps in this mail carry plenty of digits, and none of them is the code.
func TestOTPCodePatternIgnoresInlineDigits(t *testing.T) {
	const body = `リスクの高いログイン試行を2026年08月01日 14時21分59秒に検知しました。
有効期限は10分（2026年08月01日 14時31分59秒まで）です。
お問い合わせ: 0120123456 まで
`
	if m := OTPMail.Pattern.FindStringSubmatch(body); m != nil {
		t.Errorf("OTPMail.Pattern matched %q in a message with no code on its own line", m[0])
	}
}

// TestOTPCodePatternReadsTheEnglishMessage is the failure of 2026-08-02,
// against the real message this time.
//
// MoneyForward localizes this mail by where the login came from: a GitHub
// runner in the US gets English, a laptop in Tokyo gets Japanese. The old query
// filtered on `subject:追加認証`, so it matched every message tested by hand and
// none of the ones the scheduled job received — which showed up as a poll
// running its whole window with the mail sitting in every result set.
//
// Wording below is the real English body; the code is invented. The shape is
// what matters and it is the same in both languages: the code alone on a line,
// and no other run of six digits — the timestamps are punctuated.
//
// The HTML part wraps it in <span id="verification_code">, which is present in
// both languages and would be a better anchor still. It is not used: the client
// prefers text/plain, every message here has one, and this pattern is exact on
// it across every real message in the mailbox.
func TestOTPCodePatternReadsTheEnglishMessage(t *testing.T) {
	const body = `Thank you for using your Money Forward ID.
We detected risky access to your account at August 02, 2026 03:16:30 (JST).

If it's your access, please enter this code to the website to continue sign-in.
778899

This code is valid for 10 mins (August 02, 2026 03:26:30).
If the deadline has passed, please start again from the beginning.

If you do not recognize this email, please consider to change your password.
`

	m := OTPMail.Pattern.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatal("OTPMail.Pattern found no code in the English message body")
	}
	if m[1] != "778899" {
		t.Errorf("extracted %q, want %q", m[1], "778899")
	}
}

// TestOTPCodePatternRefusesTheSignInNotice is what the subject filter used to
// be for.
//
// The same address sends a new-device notice within seconds of the code, so it
// is inside the freshness window and reaches the pattern. Nothing else stops
// it: if this ever matched, the source would take the newest qualifying message
// and feed a number out of a notification into the OTP form.
func TestOTPCodePatternRefusesTheSignInNotice(t *testing.T) {
	bodies := map[string]string{
		"ja": `マネーフォワード IDをご利用いただきありがとうございます。
2026年08月02日 03時16分30秒に、新しい環境からのログインがありました。

ご利用の端末: Chrome (Linux)
心当たりがない場合は、パスワードを変更してください。
お問い合わせ番号 123456789
`,
		"en": `Thank you for using Money Forward ID.
There was a sign-in from a new environment at 2026/08/02 03:16:30.

Device: Chrome (Linux)
If this was not you, please change your password.
Reference number 123456789
`,
	}
	for lang, body := range bodies {
		t.Run(lang, func(t *testing.T) {
			if m := OTPMail.Pattern.FindStringSubmatch(body); len(m) >= 2 {
				t.Errorf("the sign-in notice yielded %q as a code", m[1])
			}
		})
	}
}

// TestOTPMailQueryDoesNotFilterOnSubject keeps the fix from being undone by
// someone tightening the query back up.
func TestOTPMailQueryDoesNotFilterOnSubject(t *testing.T) {
	if strings.Contains(OTPMail.Query, "subject:") {
		t.Errorf("Query = %q — the subject is localized, so filtering on it "+
			"matches only the language the author happened to receive", OTPMail.Query)
	}
	if !strings.Contains(OTPMail.Query, "from:") {
		t.Errorf("Query = %q, want it scoped to the sender", OTPMail.Query)
	}
}
