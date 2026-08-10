package gmail

import (
	"fmt"
	"strings"
)

// invalidGrant is how the token endpoint reports a refresh token that is no
// longer good. The accompanying text is "Token has been expired or revoked."
const invalidGrant = "invalid_grant"

// explainDeadCredential names the cause this project has actually had.
//
// The API says only `invalid_grant: "Token has been expired or revoked."` — which
// of the two, and why, it does not say. Diagnosing it once took arithmetic on run
// timestamps: the credential was installed on 2026-08-02 17:39Z, worked on
// 2026-08-08 09:02Z, and was dead by 2026-08-10 00:15Z. Seven days.
//
// Seven days is not a coincidence and not a token lifetime anybody chose. It is
// what Google grants while the OAuth consent screen's publishing status is
// Testing. So the first thing to check is that status, and the reason to check it
// before reissuing is that a new token issued under the same status dies the same
// way a week later, having taught nobody anything.
//
// Deliberately not a claim about which cause it was: a token can also be revoked
// by hand, and the message says what to look at rather than what happened.
func explainDeadCredential(err error) error {
	if err == nil || !strings.Contains(err.Error(), invalidGrant) {
		return err
	}
	return fmt.Errorf("%w\n"+
		"  The refresh token is no longer accepted. Before issuing another, check the\n"+
		"  OAuth consent screen's publishing status (Google Auth Platform → Audience).\n"+
		"  While it is Testing, Google expires refresh tokens after seven days, and a\n"+
		"  replacement issued under the same status will stop working next week too.\n"+
		"  If it is already In production, the token was revoked instead — by hand at\n"+
		"  myaccount.google.com/permissions, or by a change to the Google account", err)
}
