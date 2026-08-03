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
func account(opts *session.Options) (manualasset.Account, error) {
	if missing := config.Missing(string(secret.AssetID)); len(missing) > 0 {
		return manualasset.Account{}, opts.Missing(missing)
	}
	client, err := cookiestore.Store{Path: opts.CookieFile()}.HTTPClient()
	if err != nil {
		return manualasset.Account{}, fmt.Errorf("%w\n(run `mfpp debug mf login` first)", err)
	}
	return manualasset.Account{HTTP: client, AssetID: assetID()}, nil
}

// accountURL is the page a subcommand defaults to when given no --url.
func accountURL(opts *session.Options) (string, error) {
	if missing := config.Missing(string(secret.AssetID)); len(missing) > 0 {
		return "", opts.Missing(missing)
	}
	return manualasset.Account{AssetID: assetID()}.URL(), nil
}

// assetID is the manual account these commands address.
func assetID() string { return config.Value(secret.AssetID) }
