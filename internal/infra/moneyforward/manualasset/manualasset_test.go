package manualasset

import (
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/assetname"
)

// accountPageHTML builds a stand-in for the manual account page.
//
// Shaped after the real one, including the parts that made naive parsing fail:
// several forms before the one that matters, each with its own
// authenticity_token, and an unrelated error block that is present on every
// render.
func accountPageHTML(rows string) string {
	return `<html><head><meta name="csrf-token" content="PAGE-TOKEN"></head><body>
<div class="alert alert-error">振替元と振替先のどちらかは必ず選択してください。</div>

<form id="js-form-change-group" action="/groups/change_group" method="post">
  <input type="hidden" name="authenticity_token" value="WRONG-TOKEN-1" />
</form>

<form class="form-horizontal" id="new_user_asset_det" action="/bs/portfolio/new" method="post">
  <input type="hidden" name="authenticity_token" value="CREATE-TOKEN" />
  <input type="hidden" name="user_asset_det[id]" id="user_asset_det_id" />
  <select required="required" name="user_asset_det[sub_account_id_hash]">
    <option selected="selected" value="SUBACC-HASH">PayPay証券</option>
    <option value="0">なし</option>
  </select>
  <select required="required" name="user_asset_det[asset_subclass_id]">
    <option value="15">米国株</option>
  </select>
  <input required="required" type="text" name="user_asset_det[name]" />
  <input required="required" value="" type="text" name="user_asset_det[value]" />
  <input value="" type="text" name="user_asset_det[entried_price]" />
  <input value="" type="text" name="user_asset_det[entried_at]" />
  <input type="submit" name="commit" value="この内容で登録する" />
</form>
` + rows + `

<form id="rollover_form" action="/accounts/rollover" method="post">
  <input type="hidden" name="authenticity_token" value="WRONG-TOKEN-2" />
</form>
</body></html>`
}

// entryRowHTML builds one existing holding's edit form, as the page renders it.
//
// The name is HTML-escaped, because any real page must escape an attribute
// value. A fake that did not let "AT&T" survive a round trip it cannot survive
// on the live site, and the duplicate-row bug that caused was invisible here.
func entryRowHTML(hash, id, name, value, acquisition, subclass, token string) string {
	name = html.EscapeString(name)
	return `
<form class="form-horizontal" id="new_user_asset_det_` + hash + `" action="/bs/portfolio/edit" method="post">
  <input type="hidden" name="authenticity_token" value="` + token + `" />
  <input type="hidden" name="_method" value="put" />
  <input value="` + id + `" type="hidden" name="user_asset_det[id]" />
  <input value="SUBACC-HASH" type="hidden" name="user_asset_det[sub_account_id_hash]" />
  <input value="` + subclass + `" type="hidden" name="user_asset_det[asset_subclass_id]" />
  <input required="required" value="` + name + `" type="text" name="user_asset_det[name]" />
  <input required="required" value="` + value + `" type="text" name="user_asset_det[value]" />
  <input value="` + acquisition + `" type="text" name="user_asset_det[entried_price]" />
  <input value="" type="text" name="user_asset_det[entried_at]" />
  <input type="submit" name="commit" value="この内容で登録する" />
</form>`
}

// TestWriterPicksTheCreateForm is the regression that matters most here: the
// page carries several forms, and taking the first authenticity_token gets the
// write treated as forged. Rails answers that by nullifying the session and
// redirecting to sign-in, which is indistinguishable from an expired login.
func TestWriterPicksTheCreateForm(t *testing.T) {
	w, err := accountServing(t, accountPageHTML("")).Writer(t.Context())
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	if w.Token != "CREATE-TOKEN" {
		t.Errorf("Token = %q, want the create form's own token", w.Token)
	}
	if w.MetaToken != "PAGE-TOKEN" {
		t.Errorf("MetaToken = %q, want the page-level token deletes need", w.MetaToken)
	}
	if w.SubAssetID != "SUBACC-HASH" {
		t.Errorf("SubAssetID = %q", w.SubAssetID)
	}
	if w.SubAccountLabel != "PayPay証券" {
		t.Errorf("SubAccountLabel = %q", w.SubAccountLabel)
	}
	if w.Account.AssetID != "ASSET-HASH" {
		t.Errorf("the writer is not bound to the account it was read from: %q", w.Account.AssetID)
	}
}

// accountServing is an Account whose every request is answered with body.
//
// The code under test builds absolute moneyforward.com URLs, so the transport is
// rewritten rather than the URL — which also keeps the test honest about the
// path being requested.
func accountServing(t *testing.T, body string) Account {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.Transport = redirectTo(srv.URL)
	return Account{HTTP: client, AssetID: "ASSET-HASH"}
}

// redirectTo sends every request to the test server, whatever host it names.
type redirectTo string

func (r redirectTo) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.Clone(req.Context())
	base := strings.TrimPrefix(string(r), "http://")
	target.URL.Scheme = "http"
	target.URL.Host = base
	return http.DefaultTransport.RoundTrip(target)
}

func TestWriterRejectsAnUnauthenticatedPage(t *testing.T) {
	_, err := accountServing(t, `<html><body>ログイン</body></html>`).Writer(t.Context())
	if err == nil {
		t.Fatal("Writer() succeeded on a page with no create form")
	}
}

func TestAccountEntries(t *testing.T) {
	page := accountPageHTML(
		entryRowHTML("HASH-A", "1001", "[米国株] テスト電機", "456789", "400000", "15", "TOKEN-A") +
			entryRowHTML("HASH-B", "1002", "[投信ミ] テストAIファンド", "5432", "", "12", "TOKEN-B"),
	)

	entries, err := accountServing(t, page).Entries(t.Context())
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Entries() returned %d entries, want 2", len(entries))
	}

	a := entries[0]
	if a.Name != "[米国株] テスト電機" || a.Yen != 456789 {
		t.Errorf("first entry = %q %d", a.Name, a.Yen)
	}
	// Both identifiers, because an update needs one and a delete the other.
	if a.ID != "1001" {
		t.Errorf("ID = %q, want the numeric id an update addresses", a.ID)
	}
	if a.Hash != "HASH-A" {
		t.Errorf("Hash = %q, want the hash a delete addresses", a.Hash)
	}
	// The row's own token, not the create form's.
	if a.Token != "TOKEN-A" {
		t.Errorf("Token = %q, want this row's edit-form token", a.Token)
	}
	if !a.HasAcquisition || a.AcquisitionYen != 400000 {
		t.Errorf("acquisition = %d (known=%v), want 400000", a.AcquisitionYen, a.HasAcquisition)
	}
	if a.Subclass != SubclassUSStock {
		t.Errorf("Subclass = %d, want %d", a.Subclass, SubclassUSStock)
	}

	// A blank acquisition is "not recorded", not zero: MoneyForward would then
	// take the cost to equal the value and report no profit at all.
	b := entries[1]
	if b.HasAcquisition {
		t.Errorf("second entry reports a known acquisition of %d, want none", b.AcquisitionYen)
	}
}

func TestAccountEntriesOnEmptyAccount(t *testing.T) {
	entries, err := accountServing(t, accountPageHTML("")).Entries(t.Context())
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Entries() returned %d entries for an empty account", len(entries))
	}
}

func TestEntryEntriedPrice(t *testing.T) {
	if got := (Entry{AcquisitionYen: 400000, HasAcquisition: true}).entriedPrice(); got != "400000" {
		t.Errorf("entriedPrice() = %q, want %q", got, "400000")
	}
	// Blank rather than "0": zero is a claim, absence is not.
	if got := (Entry{}).entriedPrice(); got != "" {
		t.Errorf("entriedPrice() with no known cost = %q, want empty", got)
	}
}

func TestRejectionReason(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "validation message",
			html: `<div class="alert alert-error">名称は20文字以内でお願いします</div>`,
			want: "名称は20文字以内でお願いします",
		},
		{
			name: "tags and whitespace are stripped",
			html: "<div class=\"error\">\n  名称は<b>20文字</b>以内\n</div>",
			want: "名称は20文字以内",
		},
		{
			name: "no error block",
			html: `<div class="alert alert-success">資産を登録しました</div>`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Response{Body: []byte(tt.html)}).RejectionReason(); got != tt.want {
				t.Errorf("RejectionReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAccountURL(t *testing.T) {
	got := Account{AssetID: "ABC"}.URL()
	if !strings.HasSuffix(got, "/accounts/show_manual/ABC") {
		t.Errorf("URL() = %q", got)
	}
}

// TestMaxEntryNameLengthMatchesTheDomain ties the two places the limit is
// written down together. The domain cannot import this package to learn it, so
// the number is duplicated — this is what stops the copies from drifting.
func TestMaxEntryNameLengthMatchesTheDomain(t *testing.T) {
	if assetname.Limit != MaxEntryNameLength {
		t.Errorf("assetname.Limit = %d but MoneyForward enforces %d", assetname.Limit, MaxEntryNameLength)
	}
}

// TestRejectionReasonReportsEveryBlock is why this returns all of them.
//
// The account page carries an unrelated transfer-form error on every render, so
// taking the first match makes the answer depend on where the site happens to
// put each block — the useful one was quoted only when it came first, which is
// an ordering nothing has confirmed against the live page.
func TestRejectionReasonReportsEveryBlock(t *testing.T) {
	const noise = "振替元と振替先のどちらかは必ず選択してください。"
	const real = "名称は20文字以内でお願いします"

	for _, order := range []string{noise + "|" + real, real + "|" + noise} {
		first, second, _ := strings.Cut(order, "|")
		body := `<div class="alert alert-error">` + first + `</div>` +
			`<div class="alert alert-error">` + second + `</div>`

		got := (Response{Body: []byte(body)}).RejectionReason()
		if !strings.Contains(got, real) {
			t.Errorf("with %q first, the reason %q was dropped: %q", first, real, got)
		}
	}
}

// TestRejectionReasonDeduplicates keeps the same message from being repeated
// when the page renders it in more than one place.
func TestRejectionReasonDeduplicates(t *testing.T) {
	const msg = "名称は20文字以内でお願いします"
	body := `<div class="alert alert-error">` + msg + `</div>` +
		`<div class="error">` + msg + `</div>`

	if got := (Response{Body: []byte(body)}).RejectionReason(); got != msg {
		t.Errorf("RejectionReason() = %q, want it said once", got)
	}
}
