package sync

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/actionslog"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward/manualasset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/otp"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec"
)

// Registering values with the Actions log masker, which has to happen before
// anything can print them: ::add-mask:: only affects output that comes after
// it, so a value masked late is a value that was printed.

// maskFigures registers everything a reading knows, the moment it is known.
//
// Not when the holdings are reported: the reconciliation errors name the
// amounts that disagree, and those fire before any reporting happens.
// ::add-mask:: only affects output that comes after it, so a figure masked once
// an error has been printed is a figure that was printed.
//
// What counts as a figure is [paypaysec.Reading]'s own business — including the
// sums it computes only to report a disagreement, which exist nowhere else.
func maskFigures(masker actionslog.Masker) func(paypaysec.Reading) {
	return func(r paypaysec.Reading) {
		for _, yen := range r.Amounts() {
			masker.MaskAmount(yen)
		}
		for _, text := range r.Texts() {
			masker.MaskText(text)
		}
		reportTarget(r)
	}
}

// maskEntries registers what the MoneyForward account already held.
//
// The other half of [maskFigures]. These balances did not come from PayPay, so
// nothing has masked them, and the verification failure names one of them
// exactly when it differs from the figure that was sent.
func maskEntries(masker actionslog.Masker) func([]manualasset.Entry) {
	return func(entries []manualasset.Entry) {
		for _, e := range entries {
			for _, yen := range e.Amounts() {
				masker.MaskAmount(yen)
			}
		}
	}
}

// codeSource builds a masked Gmail source for one service.
func codeSource(mailbox otp.MailSearcher, spec otp.MailSpec, masker actionslog.Masker) otp.Source {
	return actionslog.CodeSource{
		Source: &otp.Gmail{Mail: mailbox, Spec: spec, Timeout: otpTimeout, Interval: otpInterval},
		Masker: masker,
	}
}
