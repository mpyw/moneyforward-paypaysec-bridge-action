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
// The body carries a phone number and a registration number alongside the code,
// which is why the pattern is anchored to the 認証コード label rather than
// hunting for any run of six digits.
func TestOTPCodePattern(t *testing.T) {
	const body = `平素よりPayPay証券をご愛顧いただき誠にありがとうございます。

▼以下の認証コードを入力して、お手続きください。

認証コード:AB-112233

【注意事項】
・上記の認証コードは本メール受信後、10分間有効です。
■TEL：03-6833-3000（通話料有料）
金融商品取引業者 関東財務局長（金商）第2883号
MC_0002-1`

	m := OTPMail.Pattern.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("OTPMail.Pattern found no code in the message body")
	}
	if m[1] != "112233" {
		t.Errorf("extracted %q, want %q — the two-letter prefix must not be part of it", m[1], "112233")
	}
}

func TestOTPCodePatternVariants(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"full width colon", "認証コード：AB-112233", "112233"},
		{"no prefix", "認証コード:112233", "112233"},
		{"space after colon", "認証コード: AB-112233", "112233"},
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

func TestOTPCodePatternIgnoresOtherNumbers(t *testing.T) {
	// No 認証コード label: nothing should be extracted, however many digits are
	// lying around.
	const body = "【PayPay証券】ログイン通知\n■TEL：03-6833-3000\n第2883号 123456"
	if m := OTPMail.Pattern.FindStringSubmatch(body); m != nil {
		t.Errorf("OTPMail.Pattern matched %q in a message with no code", m[0])
	}
}

// TestOTPCodePatternRefusesTheLoginNotice is what the subject filter used to be
// for, now that the query is scoped to the sender alone.
//
// 【PayPay証券】ログイン通知 arrives from the same address seconds after the code
// and is inside the same freshness window, so it reaches the pattern. It
// carries digits — a reference number, a phone number — but no labelled code.
func TestOTPCodePatternRefusesTheLoginNotice(t *testing.T) {
	const body = `PayPay証券をご利用いただきありがとうございます。
2026年08月02日 03時15分30秒にログインがありました。

お心当たりのない場合はお問い合わせください。
お問い合わせ番号 123456789
電話番号 0120123456
`

	if m := OTPMail.Pattern.FindStringSubmatch(body); len(m) >= 2 {
		t.Errorf("the login notice yielded %q as a code — the query no longer "+
			"filters it out, so this pattern is what keeps it from reaching the form", m[1])
	}
}

// TestOTPMailQueryDoesNotFilterOnSubject mirrors the MoneyForward check: the
// subject is a localized string, and MoneyForward's turned out to depend on
// where the login came from.
func TestOTPMailQueryDoesNotFilterOnSubject(t *testing.T) {
	if strings.Contains(OTPMail.Query, "subject:") {
		t.Errorf("Query = %q — discrimination belongs to the pattern", OTPMail.Query)
	}
	if !strings.Contains(OTPMail.Query, "from:") {
		t.Errorf("Query = %q, want it scoped to the sender", OTPMail.Query)
	}
}
