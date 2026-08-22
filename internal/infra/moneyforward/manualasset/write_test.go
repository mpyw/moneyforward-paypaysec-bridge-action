package manualasset

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The write path had no coverage at all, which is a poor place for it to be
// absent: this is the code that changes a financial record, and the site it
// talks to answers a rejected write with 200 and the page re-rendered. The
// tests below run against a stand-in account that actually holds rows, so a
// write that is reported but not applied fails here rather than in production.

// fakeAccount is the manual account page and its three write endpoints, holding
// the rows it has been told about.
//
// Stateful on purpose. A server that returns canned HTML would let every one of
// these pass while the writes went nowhere — which is precisely the failure the
// read-back verification exists to catch.
type fakeAccount struct {
	mu     sync.Mutex
	rows   []Entry
	nextID int

	// dropCost makes the server apply a write but discard the acquisition cost,
	// which the site reports exactly as it reports success.
	dropCost bool

	// drop names an entry the server accepts and then silently does not apply,
	// answering with the reason in an error block. That is how the real site
	// reports a name longer than 20 characters.
	drop       string
	dropReason string

	// writes records each accepted request, so ordering can be asserted.
	writes []string

	// badToken records any write that arrived with the wrong CSRF token.
	badToken []string

	// malformed records a write that broke one of the protocol details
	// writer.go documents as load-bearing: the commit button's value, the
	// tunnelled _method, the X-CSRF-Token header.
	//
	// The fake used to accept all of those missing, so removing any of them
	// from the production code passed the whole suite while the real site
	// answered 200 with the form re-rendered and nothing written.
	malformed []string
}

// checkProtocol records what a request got wrong. Recorded rather than answered
// with an error, because that is what the site does: it accepts the request,
// changes nothing, and re-renders.
func (f *fakeAccount) checkProtocol(r *http.Request, what, wantMethod, wantHeaderToken string) bool {
	ok := true
	note := func(why string) {
		f.malformed = append(f.malformed, what+": "+why)
		ok = false
	}
	if r.Header.Get("X-CSRF-Token") != wantHeaderToken {
		note("no matching X-CSRF-Token header")
	}
	if r.PostForm.Get(fieldMethod) != wantMethod {
		note(fmt.Sprintf("_method=%q, want %q", r.PostForm.Get(fieldMethod), wantMethod))
	}
	if wantMethod != "delete" && r.PostForm.Get(fieldCommit) != commitCreateText {
		note("no commit value; Rails branches on it and answers 200 either way")
	}
	if r.Header.Get("Referer") == "" {
		note("no Referer")
	}
	return ok
}

func (f *fakeAccount) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(f.render()))
		return
	}
	_ = r.ParseForm()

	switch {
	case strings.HasSuffix(r.URL.Path, createPath):
		f.create(r)
	case strings.HasSuffix(r.URL.Path, editPath):
		f.update(r)
	default:
		f.delete(r)
	}
	// Every write lands back on the account page, as the real one does.
	_, _ = w.Write([]byte(f.render()))
}

func (f *fakeAccount) create(r *http.Request) {
	name := r.PostForm.Get(fieldName)
	f.writes = append(f.writes, "create "+name)
	if r.PostForm.Get(fieldToken) != "CREATE-TOKEN" {
		f.badToken = append(f.badToken, "create "+name)
		return
	}
	if !f.checkProtocol(r, "create "+name, "", "CREATE-TOKEN") {
		return
	}
	if name == f.drop {
		return
	}
	f.nextID++
	f.rows = append(f.rows, Entry{
		ID:             strconv.Itoa(1000 + f.nextID),
		Hash:           fmt.Sprintf("HASH-%d", f.nextID),
		Name:           name,
		Yen:            atoi64(r.PostForm.Get(fieldValue)),
		AcquisitionYen: f.costOf(r.PostForm.Get(fieldEntriedPrice)),
		HasAcquisition: !f.dropCost && r.PostForm.Get(fieldEntriedPrice) != "",
		Subclass:       AssetSubclass(atoi64(r.PostForm.Get(fieldSubclass))),
	})
}

func (f *fakeAccount) update(r *http.Request) {
	id, name := r.PostForm.Get(fieldID), r.PostForm.Get(fieldName)
	f.writes = append(f.writes, "update "+name)
	for i := range f.rows {
		if f.rows[i].ID != id {
			continue
		}
		// The row's own token, not the create form's — reusing that one gets
		// the request treated as forged by the real site.
		if r.PostForm.Get(fieldToken) != tokenFor(f.rows[i].Hash) {
			f.badToken = append(f.badToken, "update "+name)
			return
		}
		if !f.checkProtocol(r, "update "+name, "put", tokenFor(f.rows[i].Hash)) {
			return
		}
		f.rows[i].Name = name
		f.rows[i].Yen = atoi64(r.PostForm.Get(fieldValue))
		f.rows[i].AcquisitionYen = atoi64(r.PostForm.Get(fieldEntriedPrice))
		f.rows[i].HasAcquisition = r.PostForm.Get(fieldEntriedPrice) != ""
		return
	}
}

func (f *fakeAccount) delete(r *http.Request) {
	hash := strings.TrimPrefix(r.URL.Path, entryPath)
	for i := range f.rows {
		if f.rows[i].Hash != hash {
			continue
		}
		f.writes = append(f.writes, "delete "+f.rows[i].Name)
		// Deletes are validated against the page-level token, not a form's.
		if r.PostForm.Get(fieldToken) != "PAGE-TOKEN" {
			f.badToken = append(f.badToken, "delete "+f.rows[i].Name)
			return
		}
		if !f.checkProtocol(r, "delete "+f.rows[i].Name, "delete", "PAGE-TOKEN") {
			return
		}
		f.rows = append(f.rows[:i], f.rows[i+1:]...)
		return
	}
}

// render draws the account page from what the server currently holds.
func (f *fakeAccount) render() string {
	var rows strings.Builder
	for _, e := range f.rows {
		acquisition := ""
		if e.HasAcquisition {
			acquisition = strconv.FormatInt(e.AcquisitionYen, 10)
		}
		rows.WriteString(entryRowHTML(e.Hash, e.ID, e.Name,
			strconv.FormatInt(e.Yen, 10), acquisition,
			strconv.Itoa(int(e.Subclass)), tokenFor(e.Hash)))
	}
	page := accountPageHTML(rows.String())
	if f.dropReason != "" {
		// After the block the page always carries, not before it. The earlier
		// version of this fake put it first, which is the only placement that
		// made a first-match reader look correct.
		page = strings.Replace(page, "</body>",
			`<div class="alert alert-error">`+f.dropReason+`</div></body>`, 1)
	}
	return page
}

// costOf applies dropCost, standing in for a write the site accepts while
// keeping only part of it.
func (f *fakeAccount) costOf(raw string) int64 {
	if f.dropCost {
		return 0
	}
	return atoi64(raw)
}

func tokenFor(hash string) string { return "TOKEN-" + hash }

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// accountBackedBy wires an Account to a running fake.
func accountBackedBy(t *testing.T, f *fakeAccount) Account {
	t.Helper()
	srv := httptest.NewTestServer(t, http.HandlerFunc(f.serve))
	return Account{HTTP: srv.Client(), AssetID: "ASSET-HASH"}
}

// TestWriterPostReportsAnErrorStatus covers a guard the stateful fake never
// reaches, because it answers 200 to everything the real site would.
func TestWriterPostReportsAnErrorStatus(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(accountPageHTML("")))
	}))
	account := Account{HTTP: srv.Client(), AssetID: "ASSET-HASH"}

	writer, err := account.Writer(t.Context())
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	if _, err := writer.Create(t.Context(), Entry{Name: "テスト電機", Yen: 1000}); err == nil {
		t.Fatal("Create() ignored a 500")
	}
}

// TestWriterPostNoticesTheSignInBounce covers the other untested guard.
//
// Rails answers a request it considers forged by nullifying the session and
// redirecting to sign-in. Without noticing that, it lands as "the entry is not
// there" from the verification step — true, and no help at all in working out
// why.
//
// The hostnames have to survive to the response, because the check reads
// resp.Request.URL.Host. The in-memory network leaves them alone — every request
// reaches this handler with the host the code asked for — so the two sites are
// told apart by r.Host rather than by dialling them to different listeners.
func TestWriterPostNoticesTheSignInBounce(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "id.moneyforward.com" {
			_, _ = w.Write([]byte("<html>sign in</html>"))
			return
		}
		if r.Method == http.MethodPost {
			http.Redirect(w, r, "https://id.moneyforward.com/sign_in", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(accountPageHTML("")))
	}))

	account := Account{HTTP: srv.Client(), AssetID: "ASSET-HASH"}

	writer, err := account.Writer(t.Context())
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	_, err = writer.Create(t.Context(), Entry{Name: "テスト電機", Yen: 1000})
	if err == nil {
		t.Fatal("Create() treated a bounce to sign-in as success")
	}
	if !strings.Contains(err.Error(), "CSRF") && !strings.Contains(err.Error(), "session") {
		t.Errorf("Create() error = %v, want it to name the likely cause", err)
	}
}

// The sequence — what to write and in what order — is the use case's, and is
// tested there against an in-memory ledger. What is left here is the part that
// only shows up against something answering like the real site: the Rails
// protocol details it silently ignores a write for getting wrong, and the round
// trip through HTML escaping.

func TestWriterCreateAndReadBack(t *testing.T) {
	fake := &fakeAccount{}
	account := accountBackedBy(t, fake)

	writer, err := account.Writer(t.Context())
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	if _, err := writer.Create(t.Context(), Entry{
		Name: "[米国株] テスト電機", Yen: 456789,
		AcquisitionYen: 400000, HasAcquisition: true, Subclass: SubclassUSStock,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	entries, err := account.Entries(t.Context())
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("the account holds %d entries, want 1", len(entries))
	}
	got := entries[0]
	if got.Name != "[米国株] テスト電機" || got.Yen != 456789 {
		t.Errorf("read back %q = %d", got.Name, got.Yen)
	}
	// A blank cost is not "unknown" to MoneyForward: it takes the cost to equal
	// the value and reports a profit of exactly zero.
	if !got.HasAcquisition || got.AcquisitionYen != 400000 {
		t.Errorf("cost read back as %d (known=%v)", got.AcquisitionYen, got.HasAcquisition)
	}
	if got.Subclass != SubclassUSStock {
		t.Errorf("subclass = %d", got.Subclass)
	}
	if len(fake.malformed) > 0 || len(fake.badToken) > 0 {
		t.Errorf("the real site would have ignored this write: %v %v", fake.malformed, fake.badToken)
	}
}

// TestWriterSurvivesAnAmpersandInTheName is the round trip that used to be
// impossible. MoneyForward escapes attribute values, so "AT&T" was written as
// sent and read back as "AT&amp;T" — and the verification, which matches by
// name, then reported every successful write as failed and created the row
// again on the next run.
func TestWriterSurvivesAnAmpersandInTheName(t *testing.T) {
	fake := &fakeAccount{}
	account := accountBackedBy(t, fake)

	writer, err := account.Writer(t.Context())
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	if _, err := writer.Create(t.Context(), Entry{Name: "[米国株] AT&T", Yen: 123456}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	entries, err := account.Entries(t.Context())
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "[米国株] AT&T" {
		t.Fatalf("read back %+v, want the name as written", entries)
	}
}

func TestWriterUpdateAndDelete(t *testing.T) {
	fake := &fakeAccount{
		nextID: 1,
		rows: []Entry{{
			ID: "1001", Hash: "HASH-1", Name: "[米国株] テスト電機",
			Yen: 100, Subclass: SubclassUSStock,
		}},
	}
	account := accountBackedBy(t, fake)

	existing, ok := account.EntryNamed(t.Context(), "[米国株] テスト電機")
	if !ok {
		t.Fatal("EntryNamed() did not find the row it was seeded with")
	}
	writer, err := account.Writer(t.Context())
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}

	existing.Yen = 456789
	existing.AcquisitionYen, existing.HasAcquisition = 400000, true
	if _, err := writer.Update(t.Context(), existing); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if fake.rows[0].Yen != 456789 || fake.rows[0].AcquisitionYen != 400000 {
		t.Errorf("after the update the account holds %+v", fake.rows[0])
	}

	if _, err := writer.Delete(t.Context(), fake.rows[0]); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(fake.rows) != 0 {
		t.Errorf("after the delete the account holds %+v", fake.rows)
	}
	if len(fake.malformed) > 0 || len(fake.badToken) > 0 {
		t.Errorf("the real site would have ignored these: %v %v", fake.malformed, fake.badToken)
	}
}
