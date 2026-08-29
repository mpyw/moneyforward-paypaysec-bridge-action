package manulife

import (
	"context"

	"github.com/google/wire"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
	mlsite "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife"
	mlsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
)

// providerSet is what a sign-in is built from.
var providerSet = wire.NewSet(
	provideCredentials,
	provideCodes,
	provideClient,
)

// credentials are the two values a login needs, read once.
type credentials struct {
	username string
	password string
}

// provideCredentials reports both missing names at once, rather than one login
// attempt at a time.
func provideCredentials(opts *session.Options) (credentials, error) {
	if err := opts.RequireEnv(string(secret.ManulifeUsername), string(secret.ManulifePassword)); err != nil {
		return credentials{}, err
	}
	return credentials{
		username: config.Value(secret.ManulifeUsername),
		password: config.Value(secret.ManulifePassword),
	}, nil
}

// provideCodes is where the one-time code comes from: the mailbox, or a file
// somebody writes it to. See [session.Options.OTPSource].
//
// The file source matters more here than anywhere else in this tree. The
// challenge page is unconfirmed, so the first runs are expected to fail on the
// browser — and with --otp file they can only fail on the browser.
func provideCodes(ctx context.Context, opts *session.Options) otp.Source {
	return opts.OTPSource(ctx, mlsel.OTPMail)
}

func provideClient(c credentials) *mlsite.Client {
	return &mlsite.Client{Username: c.username, Password: c.password}
}

// signIn is what a login needs: the site client, and where its code comes from.
type signIn struct {
	client *mlsite.Client
	codes  otp.Source
}
