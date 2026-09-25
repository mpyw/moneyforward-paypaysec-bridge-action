package investapi

import (
	"context"
	"errors"
)

// ErrNoMiniAppAccount says the page's own test reports no ミニアプリ 投資信託 for this
// account.
//
// Not a failure, and treated as neither a failure nor an empty portfolio. The two
// need opposite handling — a read that failed must stop the run, an empty bucket
// licenses deleting everything recorded under it — and this is a third thing: a
// bucket that was never asked about. Skipping the target leaves the category
// uncovered, which is what [portfolio.Plan.CheckCoverage] refuses to delete from.
//
// The condition comes from the page, not from the service: what these endpoints
// answer for an account without the bucket has not been observed, because the
// account this was built against has it. Deciding not to ask is the part that can
// be got right without that observation.
var ErrNoMiniAppAccount = errors.New("the account has no ミニアプリ 投資信託")

// AccountInfo is what pc_invest_info says about the account, beyond the client number.
type AccountInfo struct {
	// MiniClientSeqNo identifies the account to the ミニアプリ endpoints. Absent
	// accounts report it as 0, not as an empty string: the field is numeric.
	MiniClientSeqNo string

	// InvTrustUsable is the other half of [AccountInfo.hasMiniApp].
	InvTrustUsable string

	// PPKYC is carried and acted on nowhere; see [AccountInfo.hasMiniApp].
	PPKYC string
}

// hasMiniApp is the page's own test, kept in its terms.
//
// Verbatim from the bundle: `"" != (MINI_CLIENT_SEQ_NO && INV_TRUST_USABLE)`,
// which in JavaScript is "both are accountTruthy". Spelled out rather than paraphrased,
// because the values arrive as text and the shapes differ — the client number is a
// number, and INV_TRUST_USABLE has been seen as the string "true".
//
// PPKYC is deliberately not part of this. The bundle gates the tab menu on
// `hasMiniApp && PP_KYC` and blocks the app-side portfolio route when PP_KYC is 0,
// which says something about the アプリ bucket rather than this one — and says it
// about screens, not about what the endpoints return. Reading a bucket out of that
// would be a guess, so PPKYC is carried for the debug command to show and nothing
// here acts on it.
//
//declscope:package // investapi.go refuses the mini-app read without it
func (i AccountInfo) hasMiniApp() bool {
	return accountTruthy(i.MiniClientSeqNo) && accountTruthy(i.InvTrustUsable)
}

// accountTruthy reads one of these text-carried flags the way the page would.
func accountTruthy(v string) bool {
	switch v {
	case "", "0", "false":
		return false
	default:
		return true
	}
}

// ReadAccountInfo reports what the account is, asking as the ミニアプリ.
//
// Exported for the debug command: these three fields decide whether a bucket is
// there to be read, and when a read fails they are the first thing worth seeing.
func (c *Client) ReadAccountInfo(ctx context.Context) (AccountInfo, error) {
	var info infoResponse
	if err := c.post(ctx, appInfo, miniInfoFields(), &info); err != nil {
		return AccountInfo{}, err
	}
	return AccountInfo{
		MiniClientSeqNo: string(info.MiniClientSeqNo),
		InvTrustUsable:  string(info.InvTrustUsable),
		PPKYC:           string(info.PPKYC),
	}, nil
}
