package moneyforward

import (
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/cookiestore"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward/manualasset"
)

// account addresses the configured manual account over HTTP, using the session
// saved by an earlier `mfpp debug mf login`.
//
// Shared by every subcommand that reads or writes entries, which is why it is
// here rather than in any one of them.
//
//declscope:package // as above: every subcommand addresses the account through this
func account(opts *session.Options) (manualasset.Account, error) {
	if missing := config.MissingCredentials(secret.PayPaySecAssetID); len(missing) > 0 {
		return manualasset.Account{}, opts.Missing(missing)
	}
	client, err := cookiestore.Store{Path: opts.CookieFile()}.HTTPClient()
	if err != nil {
		return manualasset.Account{}, fmt.Errorf("%w\n(run `mfpp debug mf login` first)", err)
	}
	return manualasset.Account{HTTP: client, AssetID: accountAssetID()}, nil
}

// accountURL is the page a subcommand defaults to when given no --url.
//
//declscope:package // fetch and probe default their --url to this
func accountURL(opts *session.Options) (string, error) {
	if missing := config.MissingCredentials(secret.PayPaySecAssetID); len(missing) > 0 {
		return "", opts.Missing(missing)
	}
	return manualasset.Account{AssetID: accountAssetID()}.URL(), nil
}

// accountAssetID is the manual account these commands address.
//
// Resolved rather than read, so a local .envrc still carrying the retired name
// works here exactly as it does in the scheduled job. An error can only be the
// two names disagreeing, which the callers above have already reported.
func accountAssetID() string {
	res, _ := config.Resolve(secret.PayPaySecAssetID)
	return res.Value
}
