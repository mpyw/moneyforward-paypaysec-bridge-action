package sync

import (
	"io"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/actionslog"
)

// TestEveryProviderHasAReader ties the list of sources a run may be configured
// with to the ones this command can actually build.
//
// The two are separate statements and they drift in a way nothing else catches:
// a provider added to the domain without a case in provideBridges is configured,
// accepted at load, and then never read. The run succeeds, that account is never
// touched, and its figure quietly stops being updated — which is worse than a
// failure, because a failure sends mail.
//
// provideBridges is called, rather than compared against a list of ids written
// here. The first version of this test held secret.Providers against its own
// copy of what the switch supports, so deleting a case from the switch left it
// green — it was checking that two statements agreed, and one of them was in
// the test.
func TestEveryProviderHasAReader(t *testing.T) {
	for _, provider := range secret.Providers {
		t.Run(provider.ID, func(t *testing.T) {
			bridges, err := provideBridges(
				config.Config{Sources: []config.Source{{
					ID:      provider.ID,
					Login:   config.Login{Username: "someone", Password: "secret"},
					AssetID: "an-account",
				}}},
				nil, nil, nil, nil, actionslog.Masker{Out: io.Discard},
			)
			if err != nil {
				t.Fatalf("provideBridges = %v — a configured source with no reader "+
					"would read nothing and say nothing", err)
			}
			if len(bridges) != 1 {
				t.Fatalf("bridges = %d, want one per configured source", len(bridges))
			}
			if got := bridges[0].Source.ID(); got != provider.ID {
				t.Errorf("bridge source = %q, want %q", got, provider.ID)
			}
		})
	}
}

// TestAnUnknownSourceIsRefused: the other direction, and the one that matters
// at runtime. A source the environment configured and this switch does not know
// about must stop the run rather than be skipped.
func TestAnUnknownSourceIsRefused(t *testing.T) {
	_, err := provideBridges(
		config.Config{Sources: []config.Source{{ID: "みらい証券", AssetID: "an-account"}}},
		nil, nil, nil, nil, actionslog.Masker{Out: io.Discard},
	)
	if err == nil {
		t.Fatal("provideBridges accepted a source it cannot read")
	}
	if !strings.Contains(err.Error(), "みらい証券") {
		t.Errorf("error = %v, want it to name the source", err)
	}
}

// TestBridgesAreBuiltInProvidersOrder: the read order is the domain's, so the
// log reads the same way twice.
func TestBridgesAreBuiltInProvidersOrder(t *testing.T) {
	var sources []config.Source
	var want []string
	for _, provider := range secret.Providers {
		sources = append(sources, config.Source{ID: provider.ID, AssetID: provider.ID})
		want = append(want, provider.ID)
	}

	bridges, err := provideBridges(
		config.Config{Sources: sources}, nil, nil, nil, nil,
		actionslog.Masker{Out: io.Discard},
	)
	if err != nil {
		t.Fatalf("provideBridges = %v", err)
	}
	var got []string
	for _, b := range bridges {
		got = append(got, b.Source.ID())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}
