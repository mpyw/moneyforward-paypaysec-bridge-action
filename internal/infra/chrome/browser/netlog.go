package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// The page's own calls, recorded so they can be read instead of guessed at.
//
// This exists because of what 投資信託 cost: three releases were spent improving
// how long the scraper waited for a tab to catch up, when the figures were
// available from an endpoint all along and a bucket there is an address rather
// than a state to observe. The lesson written down at the time was to look for
// the API before improving the wait — and there was no way to look, short of
// reading a minified bundle or opening DevTools by hand and not being able to
// hand the result to anyone.
//
// A site whose DOM offers no stable handle makes the same question urgent
// sooner: Salesforce Visualforce numbers its elements by position in a
// component tree, so nothing on the page is addressable and the traffic behind
// it is the only thing that might be.

// maxRecordedBody caps one recorded body.
//
// Generous, because the point is to read a payload in full, and small enough
// that a page streaming something large does not fill a disk. A truncated body
// says so rather than looking complete.
const maxRecordedBody = 512 << 10

// Exchange is one request and what came back.
//
// Flat and JSON, one per line, because the consumer is a person with jq and a
// question — "which call carries the number on the screen".
type Exchange struct {
	Time        string `json:"time"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	Type        string `json:"type"`
	Status      int64  `json:"status,omitempty"`
	MIME        string `json:"mime,omitempty"`
	RequestBody string `json:"requestBody,omitempty"`
	Body        string `json:"body,omitempty"`

	// Note carries what went wrong, when something did. A recorded exchange
	// with no body and no explanation is indistinguishable from an empty one.
	Note string `json:"note,omitempty"`

	// fetchPostData asks for the request body over CDP because the event did
	// not carry it. Not serialised; it is a note to this package.
	fetchPostData bool `json:"-"`
}

// postDataOf reassembles a request body from the event.
//
// The entries are base64, and there can be more than one — Chrome splits a body
// it received in pieces. An entry that will not decode is left out rather than
// guessed at, the same rule the rest of this project applies to a figure it
// cannot read.
func postDataOf(req *network.Request) string {
	var out []byte
	for _, entry := range req.PostDataEntries {
		if entry == nil || entry.Bytes == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(entry.Bytes)
		if err != nil {
			continue
		}
		out = append(out, decoded...)
	}
	return string(out)
}

// Recorder writes the XHR and fetch traffic of a page to a file.
//
// Only those two: a run loads hundreds of images, styles and scripts, and
// nothing about them answers the question this is for. A document navigation is
// left out too — that is what the page dumps are.
type Recorder struct {
	mu      sync.Mutex
	file    *os.File
	pending map[network.RequestID]*Exchange
	written int
	closed  bool

	ctx  context.Context
	wg   sync.WaitGroup
	path string
}

// Record starts recording into dir, and returns the recorder to stop.
//
// ctx must be a browser context; it is used both to listen and to ask for
// bodies afterwards.
func Record(ctx context.Context, dir, label string) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create network log dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s-network.jsonl", time.Now().Format("20060102-150405"), label))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create network log: %w", err)
	}
	// Network events are off until the domain is enabled. Without this the
	// listener below is attached correctly and simply never hears anything,
	// which reads as "the page made no calls".
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("enable network events: %w", err)
	}

	r := &Recorder{
		file:    f,
		pending: map[network.RequestID]*Exchange{},
		ctx:     ctx,
		path:    path,
	}
	chromedp.ListenTarget(ctx, r.handle)
	return r, nil
}

// Path is where the recording is being written.
func (r *Recorder) Path() string { return r.path }

// handle runs on chromedp's event goroutine, so it must not block. Fetching a
// body is a round trip, so that happens elsewhere.
func (r *Recorder) handle(ev any) {
	switch e := ev.(type) {
	case *network.EventRequestWillBeSent:
		if !recordable(e.Type) {
			return
		}
		x := &Exchange{
			Time:   time.Now().Format(time.RFC3339),
			Method: e.Request.Method,
			URL:    e.Request.URL,
			Type:   e.Type.String(),
		}
		// The request body matters as much as the reply: an endpoint that takes
		// a token or a sequence number cannot be called again without knowing
		// what it was given.
		x.RequestBody = truncate(postDataOf(e.Request))
		if x.RequestBody == "" && e.Request.HasPostData {
			// Chrome omits the entries when the body is large. Asked for
			// separately in [Recorder.finish], where a round trip is allowed.
			x.fetchPostData = true
		}
		r.mu.Lock()
		r.pending[e.RequestID] = x
		r.mu.Unlock()

	case *network.EventResponseReceived:
		r.mu.Lock()
		if x, ok := r.pending[e.RequestID]; ok {
			x.Status = e.Response.Status
			x.MIME = e.Response.MimeType
			x.Type = e.Type.String()
		}
		r.mu.Unlock()

	case *network.EventLoadingFinished:
		r.mu.Lock()
		x, ok := r.pending[e.RequestID]
		delete(r.pending, e.RequestID)
		closed := r.closed
		r.mu.Unlock()
		if !ok || closed {
			return
		}
		r.wg.Add(1)
		go r.finish(e.RequestID, x)

	case *network.EventLoadingFailed:
		r.mu.Lock()
		x, ok := r.pending[e.RequestID]
		delete(r.pending, e.RequestID)
		r.mu.Unlock()
		if !ok {
			return
		}
		x.Note = "loading failed: " + e.ErrorText
		r.write(x)
	}
}

// finish asks for the body and writes the exchange out.
//
// The body has to be fetched while the browser still holds it — Chrome evicts
// response bodies as it goes — which is why this happens on the event rather
// than at the end of the run.
func (r *Recorder) finish(id network.RequestID, x *Exchange) {
	defer r.wg.Done()

	if x.fetchPostData {
		var post []byte
		if err := chromedp.Run(r.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			p, e := network.GetRequestPostData(id).Do(ctx)
			post = p
			return e
		})); err == nil {
			x.RequestBody = truncate(string(post))
		}
	}

	var body []byte
	err := chromedp.Run(r.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		b, e := network.GetResponseBody(id).Do(ctx)
		body = b
		return e
	}))
	switch {
	case err != nil:
		// Normal rather than exceptional: a body already evicted, or a target
		// gone. The exchange is still worth having — the URL and the request
		// body are most of the answer.
		x.Note = "body unavailable: " + err.Error()
	default:
		x.Body = truncate(string(body))
	}
	r.write(x)
}

// write appends one exchange.
func (r *Recorder) write(x *Exchange) {
	blob, err := json.Marshal(x)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if _, err := r.file.Write(append(blob, '\n')); err != nil {
		return
	}
	r.written++
}

// Stop waits for the bodies still in flight and closes the file, reporting how
// many exchanges were recorded.
func (r *Recorder) Stop() (int, error) {
	r.wg.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.written, nil
	}
	r.closed = true
	return r.written, r.file.Close()
}

// recordable reports whether this is traffic worth keeping.
func recordable(t network.ResourceType) bool {
	switch t {
	case network.ResourceTypeXHR, network.ResourceTypeFetch, network.ResourceTypeEventSource:
		return true
	default:
		return false
	}
}

// truncate caps a body and says so when it had to.
func truncate(s string) string {
	if len(s) <= maxRecordedBody {
		return s
	}
	return s[:maxRecordedBody] + fmt.Sprintf("…[truncated at %d bytes]", maxRecordedBody)
}
