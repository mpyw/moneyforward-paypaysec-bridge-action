package gmail

import (
	"fmt"
	"strings"
)

// invalidGrant is how the token endpoint reports a refresh token that is no
// longer good. The accompanying text is "Token has been expired or revoked."
const invalidGrant = "invalid_grant"

// explainDeadCredential says where to look, because the API does not.
//
// Google reports a dead refresh token as `invalid_grant: "Token has been expired
// or revoked."` — not which of the two, and not why. The causes are a short list
// and they need different responses, so the message is the list rather than a
// guess at which one applied.
//
// It is a list and not a diagnosis on purpose. The first time this happened the
// consent screen's publishing status looked like the answer: the credential was
// installed on 2026-08-02 17:39Z and the run at 2026-08-10 00:15Z failed, which
// is a shade over seven days, and seven days is exactly what Testing status
// grants. The status was In production. The seven days were an artifact of when
// anybody happened to look — the last success was 2026-08-08 09:02Z, so all that
// was really known is that the token died inside a 39-hour window over a weekend
// when nothing ran.
func explainDeadCredential(err error) error {
	if err == nil || !strings.Contains(err.Error(), invalidGrant) {
		return err
	}
	return fmt.Errorf("%w\n"+
		"  The refresh token is no longer accepted. Google invalidates one for a few\n"+
		"  distinct reasons, and they are told apart before reissuing — a replacement\n"+
		"  issued against the wrong one dies the same way:\n"+
		"    - the app's OAuth consent screen is in Testing, which caps refresh tokens\n"+
		"      at seven days (Google Auth Platform → Audience)\n"+
		"    - the Google account's password changed, which invalidates every refresh\n"+
		"      token carrying a Gmail scope\n"+
		"    - access was revoked by hand at myaccount.google.com/permissions\n"+
		"    - the OAuth client itself was deleted or its secret rotated\n"+
		"  A revoke removes the app from that permissions page; the others leave it\n"+
		"  listed, which is the cheapest thing to look at first", err)
}
