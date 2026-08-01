package manualasset

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// The account page is read with regexps rather than parsed as a document. The
// markup that matters here is a handful of hidden inputs inside forms whose ids
// are stable, and each pattern below encodes something learned by getting it
// wrong against the live page — which a general-purpose selector would not
// carry with it.
var (
	// createFormPattern isolates the create form before anything is read out of
	// it. The page carries six forms, and the first authenticity_token belongs
	// to none of them in particular — using it gets the POST treated as a forged
	// request, which Rails answers by nullifying the session and redirecting to
	// sign-in. That looks exactly like an expired login.
	createFormPattern = regexp.MustCompile(`(?s)<form[^>]*id="new_user_asset_det"[^>]*>(.*?)</form>`)

	tokenPattern      = regexp.MustCompile(`name="authenticity_token"[^>]*value="([^"]+)"`)
	metaTokenPattern  = regexp.MustCompile(`<meta[^>]*name="csrf-token"[^>]*content="([^"]+)"`)
	subAccountPattern = regexp.MustCompile(
		`name="user_asset_det\[sub_account_id_hash\]"[\s\S]*?<option[^>]*value="([^"]+)"[^>]*>\s*([^<]*)</option>`)

	// Each existing row carries its own edit form, inside a modal. Those forms
	// are a better source than the rendered table: they hold both identifiers,
	// the exact stored value, and the subclass, with no formatting to undo.
	entryFormPattern = regexp.MustCompile(
		`(?s)<form[^>]*id="new_user_asset_det_([^"]+)"[^>]*>(.*?)</form>`)
	entryFieldPattern = regexp.MustCompile(
		`<input[^>]*value="([^"]*)"[^>]*name="user_asset_det\[([a-z_]+)\]"`)
	entryFieldAltPattern = regexp.MustCompile(
		`<input[^>]*name="user_asset_det\[([a-z_]+)\]"[^>]*value="([^"]*)"`)

	// errorPattern finds the message the page shows when it rejects a write.
	//
	// Scoped to error classes specifically: success is announced through the
	// same kind of block ("資産を削除しました"), and treating that as a rejection
	// turns a completed delete into a reported failure.
	errorPattern = regexp.MustCompile(`(?s)<div[^>]*class="[^"]*(?:alert-error|alert-danger|error)[^"]*"[^>]*>(.*?)</div>`)
	tagPattern   = regexp.MustCompile(`<[^>]+>`)
)

// accountPage is the HTML of one manual account page, and the only thing that
// knows how to get anything out of it.
//
// A named type rather than passing strings to a set of parse helpers: at
// package scope those helpers were reachable from the write path, where a
// "parse the account page" function applied to a POST response would find
// nothing and say so unhelpfully.
type accountPage string

// writerFor extracts what a write to account needs.
func (p accountPage) writerFor(account Account) (Writer, error) {
	w := Writer{Account: account}

	if mm := metaTokenPattern.FindStringSubmatch(string(p)); mm != nil {
		w.MetaToken = mm[1]
	}

	createForm := createFormPattern.FindStringSubmatch(string(p))
	if createForm == nil {
		return w, fmt.Errorf("no create form on %s — the session is probably not authenticated", account.URL())
	}
	form := createForm[1]

	m := tokenPattern.FindStringSubmatch(form)
	if m == nil {
		return w, fmt.Errorf("no authenticity_token in the create form on %s", account.URL())
	}
	w.Token = m[1]

	if sm := subAccountPattern.FindStringSubmatch(form); sm != nil {
		w.SubAccountIDHash = sm[1]
		w.SubAccountLabel = html.UnescapeString(strings.TrimSpace(sm[2]))
	}
	if w.SubAccountIDHash == "" {
		return w, fmt.Errorf("no sub-account option on %s", account.URL())
	}
	return w, nil
}

// entries reads every row the page records.
func (p accountPage) entries() ([]Entry, error) {
	var entries []Entry
	for _, m := range entryFormPattern.FindAllStringSubmatch(string(p), -1) {
		entry, err := parseEntryForm(m[1], m[2])
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parseEntryForm reads one row's edit form.
//
// Attribute values come back HTML-escaped, and are unescaped here. A 銘柄 whose
// name contains & — AT&T, S&P500 — is stored as sent and rendered as &amp;, so
// a name read back raw never equals the one that was written. The verification
// step matches by name, so every run reported the write as failed, having
// already applied it, and created the row again on the next run: one duplicate
// of a real position per run, with the balance double-counted and the delete
// steps that would have cleaned up never reached.
func parseEntryForm(hash, body string) (Entry, error) {
	fields := map[string]string{}
	for _, f := range entryFieldPattern.FindAllStringSubmatch(body, -1) {
		fields[f[2]] = html.UnescapeString(f[1])
	}
	// The same inputs with the attributes the other way round. Both orders
	// appear on the page depending on which of them Rails rendered.
	for _, f := range entryFieldAltPattern.FindAllStringSubmatch(body, -1) {
		if _, seen := fields[f[1]]; !seen {
			fields[f[1]] = html.UnescapeString(f[2])
		}
	}

	yen, err := strconv.ParseInt(strings.TrimSpace(fields["value"]), 10, 64)
	if err != nil {
		return Entry{}, fmt.Errorf("row %s: unreadable value %q", hash, fields["value"])
	}
	subclass, _ := strconv.Atoi(fields["asset_subclass_id"])

	token := ""
	if tm := tokenPattern.FindStringSubmatch(body); tm != nil {
		token = tm[1]
	}

	// A blank acquisition means "not recorded", which is not the same as zero —
	// see [Entry].
	acquisition, hasAcquisition := int64(0), false
	if raw := strings.TrimSpace(fields["entried_price"]); raw != "" {
		if v, aerr := strconv.ParseInt(raw, 10, 64); aerr == nil {
			acquisition, hasAcquisition = v, true
		}
	}

	return Entry{
		ID:             fields["id"],
		Hash:           hash,
		Token:          token,
		Name:           fields["name"],
		Yen:            yen,
		AcquisitionYen: acquisition,
		HasAcquisition: hasAcquisition,
		Subclass:       AssetSubclass(subclass),
	}, nil
}
