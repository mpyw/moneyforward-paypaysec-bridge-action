package adapter

import (
	"context"
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
)

// PayPaySecBroker reads holdings from PayPay 証券.
type PayPaySecBroker struct {
	Client *paypaysec.Client

	// Browser is the chromedp context the site is driven through.
	Browser context.Context

	// Codes supplies the one-time code the login needs.
	Codes otp.Source

	// OnLogin, if set, is told whether a challenge was presented.
	OnLogin func(challenged bool)
}

// SignIn logs in, obtaining a one-time code if the site asks for one.
func (b PayPaySecBroker) SignIn(context.Context) error {
	result, err := b.Client.Login(b.Browser, b.Codes)
	if err != nil {
		if step := paypaysec.StepOf(err); step != "" {
			return fmt.Errorf("paypaysec: login failed at %s: %w", step, err)
		}
		return fmt.Errorf("paypaysec: login: %w", err)
	}
	if b.OnLogin != nil {
		b.OnLogin(result.OTPRequired)
	}
	return nil
}

// Holdings reads every target and returns one entry per 銘柄.
func (b PayPaySecBroker) Holdings(context.Context) (asset.Holdings, error) {
	balances, err := b.Client.GetBalances(b.Browser)
	if err != nil {
		return asset.Holdings{}, fmt.Errorf("paypaysec: %w", err)
	}
	assets, err := balances.Assets()
	if err != nil {
		return asset.Holdings{}, fmt.Errorf("paypaysec: %w", err)
	}
	// Every target that produced a reading, not every target configured:
	// GetBalances fails the whole read rather than skipping one, so these are
	// exactly the pages that were verified.
	return asset.Holdings{Assets: assets, Categories: balances.Categories()}, nil
}
