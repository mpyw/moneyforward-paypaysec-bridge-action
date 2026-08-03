package paypaysec

import (
	"github.com/google/wire"

	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
	ppsite "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
	ppsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
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
	if err := opts.RequireEnv(string(secret.PayPaySecUsername), string(secret.PayPaySecPassword)); err != nil {
		return credentials{}, err
	}
	return credentials{
		username: config.Value(secret.PayPaySecUsername),
		password: config.Value(secret.PayPaySecPassword),
	}, nil
}

// provideCodes is where the one-time code comes from: the mailbox, or a file
// somebody writes it to. See [session.Options.OTPSource].
func provideCodes(ctx context.Context, opts *session.Options) otp.Source {
	return opts.OTPSource(ctx, ppsel.OTPMail)
}

func provideClient(c credentials) *ppsite.Client {
	return &ppsite.Client{Username: c.username, Password: c.password}
}

// signIn is what a login needs: the site client, and where its code comes from.
//
// A pair because Login takes the source as an argument rather than holding it —
// the client is reusable across attempts and the source is not.
type signIn struct {
	client *ppsite.Client
	codes  otp.Source
}
