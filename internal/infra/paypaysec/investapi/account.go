package investapi

import (
	"context"
	"errors"
)

// ErrNoMiniApp says the page's own test reports no ミニアプリ 投資信託 for this
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
var ErrNoMiniApp = errors.New("the account has no ミニアプリ 投資信託")

// Info is what pc_invest_info says about the account, beyond the client number.
type Info struct {
	// MiniClientSeqNo identifies the account to the ミニアプリ endpoints. Absent
	// accounts report it as 0, not as an empty string: the field is numeric.
	MiniClientSeqNo string

	// InvTrustUsable is the other half of [Info.HasMiniApp].
	InvTrustUsable string

	// PPKYC is carried and acted on nowhere; see [Info.HasMiniApp].
	PPKYC string
}

// HasMiniApp is the page's own test, kept in its terms.
//
// Verbatim from the bundle: `"" != (MINI_CLIENT_SEQ_NO && INV_TRUST_USABLE)`,
// which in JavaScript is "both are truthy". Spelled out rather than paraphrased,
// because the values arrive as text and the shapes differ — the client number is a
// number, and INV_TRUST_USABLE has been seen as the string "true".
//
// PPKYC is deliberately not part of this. The bundle gates the tab menu on
// `hasMiniApp && PP_KYC` and blocks the app-side portfolio route when PP_KYC is 0,
// which says something about the アプリ bucket rather than this one — and says it
// about screens, not about what the endpoints return. Reading a bucket out of that
// would be a guess, so PPKYC is carried for the debug command to show and nothing
// here acts on it.
func (i Info) HasMiniApp() bool {
	return truthy(i.MiniClientSeqNo) && truthy(i.InvTrustUsable)
}

// truthy reads one of these text-carried flags the way the page would.
func truthy(v string) bool {
	switch v {
	case "", "0", "false":
		return false
	default:
		return true
	}
}

// ReadInfo reports what the account is, asking as the ミニアプリ.
//
// Exported for the debug command: these three fields decide whether a bucket is
// there to be read, and when a read fails they are the first thing worth seeing.
func (c *Client) ReadInfo(ctx context.Context) (Info, error) {
	var info infoResponse
	if err := c.post(ctx, appInfo, miniInfoFields(), &info); err != nil {
		return Info{}, err
	}
	return Info{
		MiniClientSeqNo: string(info.MiniClientSeqNo),
		InvTrustUsable:  string(info.InvTrustUsable),
		PPKYC:           string(info.PPKYC),
	}, nil
}

// miniClientSeqNo is the account's ミニアプリ client number, asked for once.
//
// The page's own availability test is applied here rather than by the caller,
// because this is the only place the answer is known and the only place a caller
// could act on it too late. What the ミニアプリ endpoints do for an account without
// the bucket is not known — this account has it, and the other case cannot be
// observed from here — so the site's judgement is used as the judgement and the
// bucket is not asked about at all.
func (c *Client) miniClientSeqNo(ctx context.Context) (string, error) {
	if c.miniSeqNo != "" {
		return c.miniSeqNo, nil
	}

	account, err := c.ReadInfo(ctx)
	if err != nil {
		return "", err
	}
	if !account.HasMiniApp() {
		return "", ErrNoMiniApp
	}
	c.miniSeqNo = account.MiniClientSeqNo
	return c.miniSeqNo, nil
}
