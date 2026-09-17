// Package moneyforward drives MoneyForward Me: authenticate with ID/PW plus the
// new-device email OTP, then update a manual asset's balance.
//
// File layout:
//
//	client.go     — the Client and its step vocabulary
//	login.go      — authentication
//	asset.go      — the balance write
//	selectors.go  — every DOM selector and URL
package moneyforward

import (
	"fmt"
)

// Client holds the credentials for one MoneyForward account. The zero value is
// not usable; call Validate before driving a browser with it.
type Client struct {
	Email    string
	Password string

	// AssetID is the account_id_hash of the manual asset to update. Only needed
	// by UpdateAssetBalance, so login-only callers may leave it empty.
	AssetID string
}

// Validate reports missing credentials up front, rather than letting a browser
// launch and an empty form submission produce a confusing failure three steps
// later.
func (c *Client) Validate() error {
	var missing []string
	if c.Email == "" {
		missing = append(missing, "Email")
	}
	if c.Password == "" {
		missing = append(missing, "Password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("moneyforward: missing credentials: %v", missing)
	}
	return nil
}
