package selector

import "testing"

// TestTrimLabelHandlesEveryPunctuationThisSiteUses is a regression test for a
// site that writes one label three ways.
//
// CONFIRMED 2026-08-29: the contract list renders 種類-証券番号 with a full-width
// colon and 契約状況 with a space then a half-width one, while the detail page
// renders 種類-証券番号 with a half-width colon. Comparing literally works on
// whichever page was read first and finds nothing on the other, and finding
// nothing is not a loud failure here — it is a contract that appears not to
// exist.
func TestTrimLabelHandlesEveryPunctuationThisSiteUses(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"full width colon, from the list card", "種類-証券番号：", LabelPolicyNumber},
		{"half width colon, from the detail page", "種類-証券番号:", LabelPolicyNumber},
		{"space then half width colon", "契約状況 :", LabelStatus},
		{"ideographic space", "契約状況　:", LabelStatus},
		{"already bare", LabelProductName, LabelProductName},
		{"surrounding whitespace", "  商品名:  ", LabelProductName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrimLabel(tt.in); got != tt.want {
				t.Errorf("TrimLabel(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestTrimLabelLeavesTheLabelItself guards the cutset from growing into
// something that eats part of a name.
//
// 種類-証券番号 contains a hyphen and 【基本情報】 its own brackets; a cutset
// widened by habit would start trimming those, and the label would stop
// matching for a new reason.
func TestTrimLabelLeavesTheLabelItself(t *testing.T) {
	for _, in := range []string{"種類-証券番号", "解約時お支払金額（円支払）", "円換算レート", "保険種類"} {
		if got := TrimLabel(in); got != in {
			t.Errorf("TrimLabel(%q) = %q — it should only strip punctuation around a label", in, got)
		}
	}
}
