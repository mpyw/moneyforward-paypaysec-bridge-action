package investapi

import (
	"strings"
	"testing"
)

func TestReadApp(t *testing.T) {
	s := &stub{}
	got, err := serve(t, s).Read(t.Context(), App)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if got.Total != 345678 || got.Acquisition != 300000 || got.Gain != 45678 {
		t.Errorf("totals = %+v", got)
	}
	if len(got.Holdings) != 1 {
		t.Fatalf("holdings = %+v, want the one held", got.Holdings)
	}
	h := got.Holdings[0]
	if h.Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q", h.Name)
	}
	// Derived, not fetched: value minus unrealised gain.
	if h.Acquisition != 300000 {
		t.Errorf("acquisition = %d, want value - gain", h.Acquisition)
	}
	// The catalogue lists a 銘柄 the account does not hold. Only what the top
	// call reported is a holding — reading the catalogue as the portfolio would
	// invent hundreds.
	if s.fields[appTop]["APP_ID"] != "3" {
		t.Errorf("APP_ID = %q, want the app bucket", s.fields[appTop]["APP_ID"])
	}
	if _, asked := s.fields[appInfo]; asked {
		t.Error("the app bucket asked for a MINI_CLIENT_SEQ_NO it does not need")
	}
}

// TestReadAsksForTheCatalogueFirst pins the order the page loads these in.
//
// Nothing here needs the catalogue before the holdings — this is a stateful PHP
// service, and the mini bucket has already refused a top call that arrived out of
// the page's order once.
func TestReadAsksForTheCatalogueFirst(t *testing.T) {
	s := &stub{}
	if _, err := serve(t, s).Read(t.Context(), App); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	var init, top int
	for i, path := range s.mu {
		switch path {
		case appInit:
			init = i
		case appTop:
			top = i
		}
	}
	if init > top {
		t.Errorf("call order = %v, want %s before %s", s.mu, appInit, appTop)
	}
}

// TestReadRefusesAHoldingItCannotName guards the ledger's key. An entry recorded
// under an empty name cannot be matched again, so the next run creates another.
func TestReadRefusesAHoldingItCannotName(t *testing.T) {
	_, err := serve(t, &stub{noName: true}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() returned a holding with no name")
	}
	if !strings.Contains(err.Error(), "does not name it") {
		t.Errorf("error = %v", err)
	}
}

// TestReadSaysWhichPageItIsOn is the header the ミニアプリ bucket will not answer
// without.
//
// Measured against the live service: the identical body is accepted from inside
// the document and refused from a client that sends no Referer, with STATUS 9 and
// システムの不具合 — which names nothing, and cost three releases of guessing from
// CI before the call was made from a terminal instead. A browser User-Agent makes
// no difference; this header is the whole of it.
func TestReadSaysWhichPageItIsOn(t *testing.T) {
	s := &stub{}
	if _, err := serve(t, s).Read(t.Context(), MiniApp); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	// Every call, not only the ミニアプリ ones: the v2 endpoints do not ask, and an
	// exception is a thing to keep true.
	for _, path := range []string{appInfo, miniInit, miniTop} {
		got := s.referers[path]
		if got == "" {
			t.Errorf("%s was sent no Referer", path)
			continue
		}
		if !strings.HasSuffix(got, "/investment_trust/") {
			t.Errorf("%s Referer = %q, want the 投資信託 screen", path, got)
		}
	}
}
