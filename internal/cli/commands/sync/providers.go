package sync

import (
	"github.com/google/wire"

	"context"
	"fmt"
	"log"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/port"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/credentials"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/actionslog"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/adapter"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward"
	mfsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward/selector"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/otp"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec"
	ppsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec/selector"
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
	provideMoneyForwardCodes,
	provideBroker,
	provideLedger,
	provideReporter,
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

// payPaySecCodes and moneyForwardCodes are each service's OTP source. Same
// interface, different mailbox query and different code pattern; crossing them
// would wait for mail that is not coming.
type (
	payPaySecCodes    otp.Source
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
	masker.MaskAll(c.PayPaySec.Username, c.MoneyForward.Username, c.AccountIDHash)
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

func provideMoneyForwardCodes(mailbox otp.MailSearcher, masker actionslog.Masker) moneyForwardCodes {
	return codeSource(mailbox, mfsel.OTPMail, masker)
}

func provideBroker(c config.Config, bctx browserContext, codes payPaySecCodes, masker actionslog.Masker) port.Broker {
	return adapter.PayPaySecBroker{
		Client: &paypaysec.Client{
			Username: c.PayPaySec.Username,
			Password: c.PayPaySec.Password,
			OnRead:   maskFigures(masker),
		},
		Browser: bctx,
		Codes:   codes,
		OnLogin: logChallenge("PayPay 証券"),
	}
}

func provideLedger(c config.Config, bctx browserContext, codes moneyForwardCodes, masker actionslog.Masker) port.Ledger {
	return &adapter.MoneyForwardLedger{
		Client: &moneyforward.Client{
			Email:    c.MoneyForward.Username,
			Password: c.MoneyForward.Password,
			AssetID:  c.AccountIDHash,
		},
		Browser:       bctx,
		AccountIDHash: c.AccountIDHash,
		Codes:         codes,
		OnLogin:       logChallenge("MoneyForward"),
		OnRead:        maskEntries(masker),
	}
}

func provideReporter(masker actionslog.Masker) port.Reporter {
	return reporter{masker: masker}
}
