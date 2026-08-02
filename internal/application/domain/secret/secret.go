// Package secret names the credentials the scheduled job cannot run without.
//
// One list, in the domain, because two places once disagreed about it: the job
// checked five environment variables and a setup command prompted for five,
// and nothing but care kept them the same set. The setup command is gone, but
// the list is also the action's input contract, so it stays in one place.
package secret

// Name is one credential's identifier, as the environment and the repository
// both know it.
type Name string

// The credentials the sync needs.
const (
	PayPaySecUsername Name = "PAYPAYSEC_USERNAME"
	PayPaySecPassword Name = "PAYPAYSEC_PASSWORD"
	MoneyForwardEmail Name = "MONEYFORWARD_EMAIL"
	MoneyForwardPass  Name = "MONEYFORWARD_PASSWORD"
	AssetID           Name = "MONEYFORWARD_ASSET_ID"
)

// Required is every credential the sync needs.
//
// Spec used to carry a prompt and an echo flag for the setup command that
// asked for them. That command is gone; the names are not, because the job
// still has to say which ones are missing before it starts.
var Required = []Name{
	PayPaySecUsername,
	PayPaySecPassword,
	MoneyForwardEmail,
	MoneyForwardPass,
	AssetID,
}

// RequiredNames is [Required] as plain names, for checking an environment.
func RequiredNames() []string {
	out := make([]string, 0, len(Required))
	for _, n := range Required {
		out = append(out, string(n))
	}
	return out
}
