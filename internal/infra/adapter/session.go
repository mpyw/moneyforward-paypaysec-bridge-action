package adapter

import (
	"context"
	"fmt"
	"sync"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
)

// MoneyForwardSession is one sign-in to MoneyForward.
//
// Separate from the account because there are several accounts and one login.
// Every source records into its own manual account, and signing in per account
// would mail a one-time code per account — for the same person, to the same
// mailbox, seconds apart, on a service that stops sending them after a handful.
//
// Used through a pointer, and it signs in at most once however many ledgers ask.
type MoneyForwardSession struct {
	Client *moneyforward.Client

	// Browser is the chromedp context the sign-in is driven through.
	Browser context.Context

	// Codes supplies the one-time code the login needs.
	Codes otp.Source

	// OnLogin, if set, is told whether a challenge was presented.
	OnLogin func(challenged bool)

	once sync.Once
	err  error
}

// SignIn logs in, once, and reports the same outcome to every later caller.
//
// A failure is remembered rather than retried: a second attempt would mail a
// second code, and whatever stopped the first — wrong password, no code, a
// challenge nobody answered — is not something a retry fixes.
func (s *MoneyForwardSession) SignIn() error {
	s.once.Do(func() {
		result, err := s.Client.Login(s.Browser, s.Codes)
		if err != nil {
			if step := moneyforward.StepOf(err); step != "" {
				s.err = fmt.Errorf("moneyforward: login failed at %s: %w", step, err)
				return
			}
			s.err = fmt.Errorf("moneyforward: login: %w", err)
			return
		}
		if s.OnLogin != nil {
			s.OnLogin(result.OTPRequired)
		}
	})
	return s.err
}
