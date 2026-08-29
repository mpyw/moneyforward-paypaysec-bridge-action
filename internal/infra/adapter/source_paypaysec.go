package adapter

import (
	"context"
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
)

// PayPaySecID names this source in logs, in errors, and in the input names its
// credentials arrive under.
const PayPaySecID = "paypaysec"

// PayPaySecSource reads holdings from PayPay 証券.
type PayPaySecSource struct {
	Client *paypaysec.Client

	// Browser is the chromedp context the site is driven through.
	Browser context.Context

	// Codes supplies the one-time code the login needs.
	Codes otp.Source

	// OnLogin, if set, is told whether a challenge was presented.
	OnLogin func(challenged bool)
}

// ID names this source.
func (b PayPaySecSource) ID() string { return PayPaySecID }

// SignIn logs in, obtaining a one-time code if the site asks for one.
func (b PayPaySecSource) SignIn(context.Context) error {
	result, err := b.Client.Login(b.Browser, b.Codes)
	if err != nil {
		if step := paypaysec.StepOf(err); step != "" {
			return fmt.Errorf("login failed at %s: %w", step, err)
		}
		return fmt.Errorf("login: %w", err)
	}
	if b.OnLogin != nil {
		b.OnLogin(result.OTPRequired)
	}
	return nil
}

// Holdings reads every target and returns one entry per 銘柄.
func (b PayPaySecSource) Holdings(context.Context) (asset.Holdings, error) {
	balances, err := b.Client.GetBalances(b.Browser)
	if err != nil {
		return asset.Holdings{}, err
	}
	assets, err := balances.Assets()
	if err != nil {
		return asset.Holdings{}, err
	}
	// Every target that produced a reading, not every target configured:
	// GetBalances fails the whole read rather than skipping one, so these are
	// exactly the pages that were verified.
	return asset.Holdings{Assets: assets, Categories: balances.Categories()}, nil
}
