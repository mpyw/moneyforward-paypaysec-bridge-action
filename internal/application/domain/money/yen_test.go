package money

import (
	"errors"
	"testing"
)

func TestParseYen(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		// Figures observed on the live account, 2026-08-01.
		{"man with remainder", "78万9012円", 789012},
		{"man with remainder, investment trust", "25万1234円", 251234},
		{"round million is zero padded", "60万0000円", 600000},
		{"acquisition total", "22万0000円", 220000},
		{"signed gain", "+3万1234円", 31234},
		{"signed loss, plain", "-237円", -237},
		{"plain amount", "3210円", 3210},
		{"plain zero", "0円", 0},

		// Formatting the site may apply.
		{"comma separated", "789,012円", 789012},
		{"full width comma", "789，012円", 789012},
		{"no yen marker", "3210", 3210},
		{"surrounding whitespace", "  3210円  ", 3210},
		{"full width space", "　3210円", 3210},
		{"man with no remainder", "33万", 330000},
		{"signed man loss", "-1万2000円", -12000},
		{"explicit plus on plain", "+3210円", 3210},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseYen(tt.input)
			if err != nil {
				t.Fatalf("ParseYen(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseYen(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseYenPlaceholders(t *testing.T) {
	// An empty holding is a legitimate state and must be distinguishable from
	// markup we failed to understand.
	for _, input := range []string{"", "—", "―", "−", "-", "  —  ", "—円"} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseYen(input)
			if !errors.Is(err, ErrNoValue) {
				t.Errorf("ParseYen(%q) error = %v, want ErrNoValue", input, err)
			}
		})
	}
}

func TestParseYenRejectsAmbiguousAndUnknown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		why   string
	}{
		{
			name:  "short man remainder",
			input: "1万23円",
			why:   "10,023 or 12,300 cannot be told apart, so it must not be guessed",
		},
		{
			name:  "long man remainder",
			input: "1万23456円",
			why:   "not a shape the site produces",
		},
		{
			name:  "trailing unit",
			input: "12株",
			why:   "a lenient parse would silently yield 12",
		},
		{
			name:  "leading text",
			input: "約3210円",
			why:   "an approximation marker changes the meaning",
		},
		{
			name:  "decimal",
			input: "3210.5円",
			why:   "yen figures are whole; a decimal means this is not a yen cell",
		},
		{
			name:  "oku notation",
			input: "1億2000万円",
			why:   "unsupported rather than mis-scaled",
		},
		{
			name:  "double man",
			input: "1万2万円",
			why:   "malformed",
		},
		{
			name:  "sign only",
			input: "+円",
			why:   "no digits",
		},
		{
			name:  "full width digits",
			input: "２０９３円",
			why:   "not a shape the site produces; better to fail than to mis-read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseYen(tt.input)
			if err == nil {
				t.Fatalf("ParseYen(%q) = %d, want an error (%s)", tt.input, got, tt.why)
			}
			if errors.Is(err, ErrNoValue) {
				t.Errorf("ParseYen(%q) returned ErrNoValue, want a parse error (%s)", tt.input, tt.why)
			}
		})
	}
}

// TestParseYenManAndPlainAgree guards the arithmetic of the 万 form against the
// plain form for the same value.
func TestParseYenManAndPlainAgree(t *testing.T) {
	pairs := []struct{ man, plain string }{
		{"1万0000円", "10000円"},
		{"78万9012円", "789012円"},
		{"-1万2000円", "-12000円"},
	}
	for _, p := range pairs {
		t.Run(p.man, func(t *testing.T) {
			a, err := ParseYen(p.man)
			if err != nil {
				t.Fatalf("ParseYen(%q) error = %v", p.man, err)
			}
			b, err := ParseYen(p.plain)
			if err != nil {
				t.Fatalf("ParseYen(%q) error = %v", p.plain, err)
			}
			if a != b {
				t.Errorf("ParseYen(%q) = %d but ParseYen(%q) = %d", p.man, a, p.plain, b)
			}
		})
	}
}

// TestParseYenRefusesMoreThanOneAmount is the drift this package exists to
// survive.
//
// 円, commas and spaces used to be deleted wherever they appeared, so the only
// thing separating one figure from the next went with them and two amounts
// concatenated into a third that looked entirely reasonable. A selector that
// starts matching a container rather than a cell is not hypothetical here: an
// earlier version of the scraper read an unrelated "0円" as the balance of an
// account holding 25万1234円.
func TestParseYenRefusesMoreThanOneAmount(t *testing.T) {
	for _, in := range []string{
		"100円200円",       // would have been 100200
		"1,234円5,678円",   // would have been 12345678
		"25万1234円 1000円", // two cells, one string
		"100 200",        // separated by nothing but space
		"円100",           // 円 leading rather than trailing
		"100円200",
	} {
		if got, err := ParseYen(in); err == nil {
			t.Errorf("ParseYen(%q) = %d, want a refusal", in, got)
		}
	}
}

// TestParseYenRefusesMisplacedSeparators covers a comma that is not a thousands
// separator. Deleting it produced a number the text never said.
func TestParseYenRefusesMisplacedSeparators(t *testing.T) {
	for _, in := range []string{"1,00円", "1,0,0円", "12,34円", "1,2345円", ",123円"} {
		if got, err := ParseYen(in); err == nil {
			t.Errorf("ParseYen(%q) = %d, want a refusal", in, got)
		}
	}
}

// TestParseYenStillAcceptsWhatTheSitesRender is the other half: the rules above
// must not reject anything real. Every form here has been seen on a live page.
func TestParseYenStillAcceptsWhatTheSitesRender(t *testing.T) {
	tests := map[string]int64{
		"25万1234円":    251234,
		"78万9012円":    789012,
		"60万0000円":    600000,
		"+3万1234円":    31234,
		"-237円":       -237,
		"345,678円":    345678,
		"5432円":       5432,
		"  456789円  ": 456789,
		"0円":          0,
		"941,356円":    941356,
	}
	for in, want := range tests {
		got, err := ParseYen(in)
		if err != nil {
			t.Errorf("ParseYen(%q) error = %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseYen(%q) = %d, want %d", in, got, want)
		}
	}
}
