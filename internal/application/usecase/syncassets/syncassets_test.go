package syncassets_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/assetname"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/portfolio"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/usecase/syncassets"
)

type stubBroker struct {
	assets []asset.Asset

	// categories overrides what the broker says it covered. Nil means "derive
	// it from the assets", which is the healthy case.
	categories []string
	err        error
	signIn     error
	signed     int
}

func (s *stubBroker) SignIn(context.Context) error { s.signed++; return s.signIn }

func (s *stubBroker) Holdings(context.Context) (asset.Holdings, error) {
	if s.categories != nil {
		return asset.Holdings{Assets: s.assets, Categories: s.categories}, s.err
	}
	// Default: every category the stub's own assets mention was covered, which
	// is what a healthy read looks like.
	seen := map[string]bool{}
	var categories []string
	for _, a := range s.assets {
		if c, ok := assetname.CategoryOf(a.Name); ok && !seen[c] {
			seen[c] = true
			categories = append(categories, c)
		}
	}
	return asset.Holdings{Assets: s.assets, Categories: categories}, s.err
}

// stubLedger is an in-memory account.
//
// The HTTP shape of a real one is covered where the HTTP is, against a stand-in
// server; what this covers is the sequence — what gets written, in what order,
// and what happens when the account does not end up saying what it was told.
type stubLedger struct {
	held []asset.Asset

	// writes records each operation, so ordering can be asserted.
	writes []string

	// dropCost applies a create or update while discarding the cost, which is
	// how a ledger reached over the web reports partial success: it does not.
	dropCost bool

	// ignore names an entry whose writes are accepted and not applied.
	ignore string

	// rejection is what such a ledger says about it afterwards.
	rejection string

	signed  int
	readErr error
	failOn  string
}

func (s *stubLedger) SignIn(context.Context) error { s.signed++; return nil }

func (s *stubLedger) Recorded(context.Context) ([]asset.Asset, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return slices.Clone(s.held), nil
}

func (s *stubLedger) Create(_ context.Context, a asset.Asset) error {
	s.writes = append(s.writes, "create "+a.Name)
	if a.Name == s.failOn {
		return errors.New("create refused")
	}
	if a.Name == s.ignore {
		return nil
	}
	s.held = append(s.held, s.applied(a))
	return nil
}

func (s *stubLedger) Update(_ context.Context, a asset.Asset) error {
	s.writes = append(s.writes, "update "+a.Name)
	if a.Name == s.ignore {
		return nil
	}
	for i := range s.held {
		if s.held[i].Name == a.Name {
			s.held[i] = s.applied(a)
			return nil
		}
	}
	return errors.New("no such entry")
}

func (s *stubLedger) Delete(_ context.Context, name string) error {
	s.writes = append(s.writes, "delete "+name)
	if name == s.ignore {
		return nil
	}
	s.held = slices.DeleteFunc(s.held, func(a asset.Asset) bool { return a.Name == name })
	return nil
}

// applied is what the ledger ends up holding, which is not always what it was
// sent.
func (s *stubLedger) applied(a asset.Asset) asset.Asset {
	if s.dropCost {
		a.AcquisitionYen, a.HasAcquisition = 0, false
	}
	return a
}

func (s *stubLedger) LastRejection() string { return s.rejection }

type recordingReporter struct {
	phases []string
	read   int
	plans  int
}

func (r *recordingReporter) Phase(name string)        { r.phases = append(r.phases, name) }
func (r *recordingReporter) ReadResult([]asset.Asset) { r.read++ }
func (r *recordingReporter) Planned(portfolio.Plan)   { r.plans++ }

func oneAsset() []asset.Asset {
	return []asset.Asset{{Name: "[米国株] テスト電機", Yen: 456789}}
}

func TestRun(t *testing.T) {
	ledger := &stubLedger{}
	reporter := &recordingReporter{}

	result, err := syncassets.Sync{
		Broker: &stubBroker{assets: oneAsset()}, Ledger: ledger, Reporter: reporter,
	}.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Assets) != 1 || len(result.Plan.Steps) != 1 {
		t.Errorf("Result = %+v", result)
	}
	if len(ledger.held) != 1 || ledger.held[0].Name != "[米国株] テスト電機" {
		t.Errorf("the ledger holds %+v", ledger.held)
	}
	// Which phases, in order — a count is satisfied by reporting the same one
	// twice, and the log this drives is how a stuck run is located.
	if want := []string{"read holdings", "record holdings"}; !slices.Equal(reporter.phases, want) {
		t.Errorf("reporter saw phases %v, want %v", reporter.phases, want)
	}
	if reporter.read != 1 || reporter.plans != 1 {
		t.Errorf("reporter saw read=%d plans=%d, want one of each", reporter.read, reporter.plans)
	}
}

// TestRunCreatesUpdatesAndDeletes is the sequence, which used to live in the
// MoneyForward adapter and so could only be tested against a stand-in for that
// one site.
func TestRunCreatesUpdatesAndDeletes(t *testing.T) {
	ledger := &stubLedger{held: []asset.Asset{
		{Name: "変わる", Yen: 100, AcquisitionYen: 90, HasAcquisition: true},
		{Name: "消える", Yen: 5000},
	}}

	result, err := syncassets.Sync{
		Broker: &stubBroker{assets: []asset.Asset{
			{Name: "変わる", Yen: 456789, AcquisitionYen: 400000, HasAcquisition: true},
			{Name: "増える", Yen: 5432},
		}},
		Ledger: ledger,
	}.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	counts := result.Plan.Counts()
	if counts[portfolio.ActionCreate] != 1 || counts[portfolio.ActionUpdate] != 1 ||
		counts[portfolio.ActionDelete] != 1 {
		t.Errorf("plan = %+v, want one of each", counts)
	}
	// Deletes last: a delete before its replacement exists leaves the ledger
	// briefly holding neither, and a run that dies in between leaves it that way.
	if len(ledger.writes) != 3 || !strings.HasPrefix(ledger.writes[2], "delete") {
		t.Errorf("writes went %v, want the delete last", ledger.writes)
	}
	if len(ledger.held) != 2 {
		t.Errorf("the ledger holds %+v, want two entries", ledger.held)
	}
}

// TestRunCatchesAWriteThatWasNotApplied is why every write is read back. A
// ledger reached over the web can report success for a write it discarded, and
// this one does.
func TestRunCatchesAWriteThatWasNotApplied(t *testing.T) {
	ledger := &stubLedger{ignore: "落ちる", rejection: "名称は20文字以内でお願いします"}

	_, err := syncassets.Sync{
		Broker: &stubBroker{assets: []asset.Asset{{Name: "落ちる", Yen: 5432}}},
		Ledger: ledger,
	}.Run(t.Context())
	if err == nil {
		t.Fatal("Run() reported success for a write the ledger discarded")
	}
	// And quotes what the service said, once the read-back has established
	// something went wrong.
	if !strings.Contains(err.Error(), "20文字以内") {
		t.Errorf("error = %v, want the service's own explanation", err)
	}
}

// TestRunCatchesALostAcquisitionCost covers the figure the ledger derives profit
// from. A write that lands the valuation and drops the cost reports a profit of
// exactly zero — plausible, and unquestionable downstream.
func TestRunCatchesALostAcquisitionCost(t *testing.T) {
	_, err := syncassets.Sync{
		Broker: &stubBroker{assets: []asset.Asset{
			{Name: "テスト電機", Yen: 456789, AcquisitionYen: 400000, HasAcquisition: true},
		}},
		Ledger: &stubLedger{dropCost: true},
	}.Run(t.Context())

	if err == nil {
		t.Fatal("Run() accepted a write that lost the cost")
	}
	if !strings.Contains(err.Error(), "cost") {
		t.Errorf("error = %v, want it to name the cost", err)
	}
}

// TestRunRefusesDuplicateNamesInTheLedger guards the assumption everything here
// makes: two entries with one name make every lookup address whichever the map
// kept, and the other adds its figure to the total indefinitely.
func TestRunRefusesDuplicateNamesInTheLedger(t *testing.T) {
	ledger := &stubLedger{held: []asset.Asset{{Name: "同名", Yen: 1}, {Name: "同名", Yen: 1}}}

	_, err := syncassets.Sync{
		Broker: &stubBroker{assets: []asset.Asset{{Name: "同名", Yen: 1}}},
		Ledger: ledger,
	}.Run(t.Context())
	if err == nil {
		t.Fatal("Run() accepted a ledger holding two entries with one name")
	}
	if len(ledger.writes) != 0 {
		t.Errorf("it wrote before noticing: %v", ledger.writes)
	}
}

// TestRunSignsInToBothBeforeWriting pins an order that is not incidental: both
// services mail a one-time code, and a code stamped before its own request
// belongs to a previous attempt.
func TestRunSignsInToBothBeforeWriting(t *testing.T) {
	broker := &stubBroker{assets: oneAsset()}
	ledger := &stubLedger{}

	if _, err := (syncassets.Sync{Broker: broker, Ledger: ledger}).Run(t.Context()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if broker.signed != 1 || ledger.signed != 1 {
		t.Errorf("signed in broker=%d ledger=%d, want once each", broker.signed, ledger.signed)
	}
}

// TestRunRefusesAnEmptyRead is the guard that matters most here. Reconciliation
// deletes what is no longer held, so a scrape that silently returned nothing
// would empty the ledger — and look like a clean run against an empty account.
func TestRunRefusesAnEmptyRead(t *testing.T) {
	ledger := &stubLedger{held: oneAsset()}
	_, err := syncassets.Sync{Broker: &stubBroker{}, Ledger: ledger}.Run(t.Context())

	if err == nil {
		t.Fatal("Run() accepted an empty read")
	}
	if !strings.Contains(err.Error(), "empty the ledger") {
		t.Errorf("error = %v, want it to say why it refused", err)
	}
	if len(ledger.writes) != 0 {
		t.Error("the ledger was written to despite the refusal")
	}
}

// TestRunAllowsAnEmptyReadWhenAskedTo is the one caller that means it:
// `mfpp debug mf sync --empty`. Off by default and never set by the scheduled
// job.
func TestRunAllowsAnEmptyReadWhenAskedTo(t *testing.T) {
	ledger := &stubLedger{held: oneAsset()}
	_, err := syncassets.Sync{Broker: &stubBroker{}, Ledger: ledger, AllowEmpty: true}.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(ledger.held) != 0 {
		t.Errorf("the ledger still holds %+v", ledger.held)
	}
}

func TestRunStopsOnAReadFailure(t *testing.T) {
	ledger := &stubLedger{}
	_, err := syncassets.Sync{
		Broker: &stubBroker{err: errors.New("login failed")}, Ledger: ledger,
	}.Run(t.Context())

	if err == nil {
		t.Fatal("Run() succeeded despite a read failure")
	}
	if ledger.signed != 0 || len(ledger.writes) != 0 {
		t.Error("the ledger was touched after the read failed")
	}
}

// TestRunReturnsThePlanEvenOnFailure lets a caller report what a partial run
// managed to do — the first question worth answering after a write fails
// halfway.
func TestRunReturnsThePlanEvenOnFailure(t *testing.T) {
	result, err := syncassets.Sync{
		Broker: &stubBroker{assets: []asset.Asset{{Name: "だめ", Yen: 1}}},
		Ledger: &stubLedger{failOn: "だめ"},
	}.Run(t.Context())

	if err == nil {
		t.Fatal("Run() succeeded despite a write failure")
	}
	if len(result.Plan.Steps) != 1 {
		t.Errorf("Result.Plan = %+v, want the attempted plan", result.Plan)
	}
}

// TestRunWithoutAReporterIsSilent keeps the dependency optional.
func TestRunWithoutAReporterIsSilent(t *testing.T) {
	_, err := syncassets.Sync{
		Broker: &stubBroker{assets: oneAsset()}, Ledger: &stubLedger{},
	}.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// TestRunRefusesDeletesFromAnUnreadCategory is the guard the first CI run
// needed, stated as what it is actually guarding.
//
// One of eight pages came back with no holdings and a zero total — internally
// consistent, so every cross-check passed, and three of five positions were
// read. The empty-read abort does not fire on three, and the reconciliation
// would have deleted the other two as no longer held.
//
// That page now fails the read outright, so it never reaches here. What does
// reach here is the version that cannot be caught at the source: a category
// that produced no reading at all, whose entries are unverified rather than
// stale.
func TestRunRefusesDeletesFromAnUnreadCategory(t *testing.T) {
	ledger := &stubLedger{held: []asset.Asset{
		{Name: "[米国株] テスト電機", Yen: 1}, {Name: "[米国株] テスト商事", Yen: 1},
		{Name: "[投信ミ] テストAIファンド", Yen: 1},
	}}

	_, err := syncassets.Sync{
		Broker: &stubBroker{
			assets: []asset.Asset{{Name: "[米国株] テスト電機", Yen: 1}, {Name: "[米国株] テスト商事", Yen: 1}},
			// 投信ミ produced nothing at all — not an empty reading, no reading.
			categories: []string{"米国株"},
		},
		Ledger: ledger,
	}.Run(t.Context())

	if err == nil {
		t.Fatal("Run() deleted an entry from a category it never read")
	}
	if !errors.Is(err, portfolio.ErrUnverifiedDeletes) {
		t.Errorf("error = %v, want ErrUnverifiedDeletes", err)
	}
	if len(ledger.writes) != 0 {
		t.Errorf("it deleted before refusing: %v", ledger.writes)
	}
}

// TestRunAllowsSellingOutOfACategory is the case the old share-based limit
// refused and this one must not.
//
// Four of five positions sold. Every page was read; one of them is now empty.
// Under the old limit that was 80% of the ledger and the run failed, every
// weekday, with an error telling the reader to raise a limit no flag exposed.
func TestRunAllowsSellingOutOfACategory(t *testing.T) {
	ledger := &stubLedger{held: []asset.Asset{
		{Name: "[米国株] テスト電機", Yen: 1}, {Name: "[米国株] テスト商事", Yen: 1},
		{Name: "[ミニ] テスト電機", Yen: 1}, {Name: "[投信ミ] テストAIファンド", Yen: 1},
		{Name: "[投信ミ] テスト・ファンド", Yen: 1},
	}}

	_, err := syncassets.Sync{
		Broker: &stubBroker{
			assets:     []asset.Asset{{Name: "[米国株] テスト電機", Yen: 1}},
			categories: []string{"米国株", "ミニ", "投信ミ"},
		},
		Ledger: ledger,
	}.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() refused a sale it had read every page for: %v", err)
	}
	if len(ledger.held) != 1 {
		t.Errorf("the ledger holds %d entries, want the one still held", len(ledger.held))
	}
}
