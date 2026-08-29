package sync

import (
	"github.com/google/wire"

	"context"
	"fmt"
	"log"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/port"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/usecase/syncassets"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/credentials"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/actionslog"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/adapter"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife"
	mlsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward"
	mfsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward/selector"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
	ppsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// providerSet is everything the scheduled job is built from.
//
// Named here beside the providers, so adding one means touching this file and
// not also the injector. Not shared with the other commands, and it cannot be:
// three of these — the broker, the ledger, the reporter — have a second
// implementation in commands/moneyforward, and wire matches by type. Two
// providers for port.Ledger in one package is an error, which is the same
// reason each command keeps its own package.
var providerSet = wire.NewSet(
	wire.Value(actionslog.Masker{}),
	provideConfig,
	provideMailbox,
	provideBrowserContext,
	providePayPaySecCodes,
	provideManulifeCodes,
	provideMoneyForwardCodes,
	provideMoneyForwardSession,
	provideBridges,
	provideReporter,
	provideAllowEmptyingCategories,
)

// The providers the injector in wire.go is built from.
//
// Wire matches dependencies by type, so anything the graph holds two of needs
// two types. There are two of exactly two things here — a context and a source
// of one-time codes — and both distinctions are ones a reader wants anyway: the
// browser's context is not the job's, and PayPay 証券's OTP mail is not
// MoneyForward's.

// browserContext is the chromedp context, as distinct from the job's own.
//
// Handing the wrong one to a site package would compile and then fail at the
// first CDP call with nothing to say about why.
type browserContext context.Context

// Each service's OTP source. Same interface, different mailbox query and
// different code pattern; crossing them would wait for mail that is not coming.
type (
	payPaySecCodes    otp.Source
	manulifeCodes     otp.Source
	moneyForwardCodes otp.Source
)

// provideConfig reads the environment and registers the identifying values with
// the log masker.
//
// The masking is here because this is the moment the values first exist.
// ::add-mask:: only affects output that comes after it, so the gap between
// reading a secret and masking it is the window in which it can be printed, and
// the narrowest that gap can be is none.
func provideConfig(masker actionslog.Masker) (config.Config, error) {
	c, err := config.Load()
	if err != nil {
		return config.Config{}, err
	}
	// The passwords are not registered: GitHub masks the secrets it injected
	// itself, and registering one here would print its length to anyone reading
	// the directive stream. The rest are values this program derives or echoes.
	masker.Mask(c.MoneyForward.Username)
	for _, source := range c.Sources {
		masker.MaskAll(source.Login.Username, source.AssetID)
		masker.MaskAmount(source.AcquisitionYen)
	}

	// Said once, here, because this is where it is known. Nothing changes
	// because of it — the value was read — but a name that still works and will
	// not forever is only worth mentioning while there is time to act on it.
	for _, name := range c.Deprecated {
		log.Printf("→ %s is the old name for this value; it still works, but "+
			"rename it when convenient", name)
	}
	return c, nil
}

// provideMailbox opens the Gmail account the codes arrive in, and proves the
// credential works before a browser is started for it.
func provideMailbox(ctx context.Context, masker actionslog.Masker) (otp.MailSearcher, error) {
	mailbox, err := credentials.OpenMailbox(ctx, credentials.DefaultCredentialsFile)
	if err != nil {
		return nil, err
	}
	addr, err := mailbox.Profile(ctx)
	if err != nil {
		return nil, fmt.Errorf("gmail credentials do not work: %w", err)
	}
	masker.Mask(addr)
	log.Println("→ Gmail credentials OK")
	return mailbox, nil
}

// provideBrowserContext launches Chrome. The cleanup closes it.
//
// It takes the mailbox it never uses so that wire puts it after the credential
// check. Starting a browser is expensive and pointless when the one-time codes
// cannot be read, and the hand-written wiring got that order simply by being
// written in it. Wire orders by dependency, so an order that matters has to be
// one — the first generated version launched Chrome before checking anything.
func provideBrowserContext(ctx context.Context, c config.Config, _ otp.MailSearcher) (browserContext, func(), error) {
	bctx, closeBrowser, err := browser.New(ctx, browser.DefaultsFor(c.CI))
	if err != nil {
		return nil, nil, err
	}
	return bctx, closeBrowser, nil
}

func providePayPaySecCodes(mailbox otp.MailSearcher, masker actionslog.Masker) payPaySecCodes {
	return codeSource(mailbox, ppsel.OTPMail, masker)
}

func provideManulifeCodes(mailbox otp.MailSearcher, masker actionslog.Masker) manulifeCodes {
	return codeSource(mailbox, mlsel.OTPMail, masker)
}

func provideMoneyForwardCodes(mailbox otp.MailSearcher, masker actionslog.Masker) moneyForwardCodes {
	return codeSource(mailbox, mfsel.OTPMail, masker)
}

// provideMoneyForwardSession is the one sign-in every account is written
// through.
//
// One, not one per account: each source records into its own manual account,
// and a login per account would mail a one-time code per account — same person,
// same mailbox, seconds apart, on a service that stops sending them after a
// handful.
func provideMoneyForwardSession(c config.Config, bctx browserContext, codes moneyForwardCodes) *adapter.MoneyForwardSession {
	return &adapter.MoneyForwardSession{
		Client: &moneyforward.Client{
			Email:    c.MoneyForward.Username,
			Password: c.MoneyForward.Password,
		},
		Browser: bctx,
		Codes:   codes,
		OnLogin: logChallenge("MoneyForward"),
	}
}

// provideBridges pairs each configured source with the account it records into.
//
// The list is built here rather than as separate providers because wire matches
// by type and every source satisfies the same interface. Assembling them in one
// place also puts "which sources does this run have" in one place, which is the
// question a reader of a log line will be asking.
func provideBridges(
	c config.Config,
	bctx browserContext,
	session *adapter.MoneyForwardSession,
	ppCodes payPaySecCodes,
	mlCodes manulifeCodes,
	masker actionslog.Masker,
) ([]syncassets.Bridge, error) {
	// Only what is configured. Absent means not read, which is not the same as
	// read and found empty: an unread category's entries are left alone where an
	// empty one's are deleted. A caller with one account has one bridge.
	var bridges []syncassets.Bridge
	for _, source := range c.Sources {
		switch source.ID {
		case adapter.PayPaySecID:
			bridges = append(bridges, syncassets.Bridge{
				Source: adapter.PayPaySecSource{
					Client: &paypaysec.Client{
						Username: source.Login.Username,
						Password: source.Login.Password,
						OnRead:   maskFigures(masker),
						OnSkip:   reportSkip,
					},
					Browser: bctx,
					Codes:   ppCodes,
					OnLogin: logChallenge("PayPay 証券"),
				},
				Ledger: ledgerFor(session, source.AssetID, masker),
			})
		case adapter.ManulifeID:
			bridges = append(bridges, syncassets.Bridge{
				Source: adapter.ManulifeSource{
					Client: &manulife.Client{
						Username: source.Login.Username,
						Password: source.Login.Password,
					},
					Browser:        bctx,
					Codes:          mlCodes,
					AcquisitionYen: source.AcquisitionYen,
					OnLogin:        logChallenge("マニュライフ生命"),
					OnRead:         maskContract(masker),
					OnSkip:         reportContractSkip,
				},
				Ledger: ledgerFor(session, source.AssetID, masker),
			})
		default:
			// Not skipped. A source the environment configured and this switch
			// does not know about is one that would be silently unread — the
			// run would succeed, its account would never be touched, and its
			// figure would quietly stop being updated. That is the failure this
			// whole arrangement is built to avoid, so it stops here instead.
			return nil, fmt.Errorf("the %s source is configured but this build has no "+
				"reader for it; every entry in secret.Providers needs one", source.ID)
		}
	}
	return bridges, nil
}

// ledgerFor is one manual account, written through the shared sign-in.
func ledgerFor(session *adapter.MoneyForwardSession, assetID string, masker actionslog.Masker) port.Ledger {
	return &adapter.MoneyForwardLedger{
		Session: session,
		AssetID: assetID,
		OnRead:  maskEntries(masker),
	}
}

// provideAllowEmptyingCategories hands the flag to the use case as its own type,
// so wire cannot confuse it with any other bool in the graph.
func provideAllowEmptyingCategories(c config.Config) bool {
	return c.AllowEmptyingCategories
}

func provideReporter(masker actionslog.Masker) port.Reporter {
	return reporter{masker: masker}
}
