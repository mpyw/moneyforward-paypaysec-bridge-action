package pagescan

import (
	"context"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// apiWatch waits for the request a tab's figures arrive in.
//
// This replaced watching the DOM. Switching the 投資信託 tab is client-side and
// instant, and the figures for the newly selected bucket arrive about a second
// later; until they do, the previous bucket's figures are on screen and
// perfectly still. Measured against the live page: the tab's own actived class
// and the nav's bucket markers both flip 8ms after the click, the numbers change
// at ~1000ms. Every state derived from the click is already correct while the
// data is still wrong, so no DOM condition can distinguish the two.
//
// The request can. The two tabs fetch different paths — see
// [selector.FiguresAPIApp] and [selector.FiguresAPIMiniApp] — so a wait for one
// tab's response is not satisfied by the other's, which is exactly what went
// wrong: a category holding two 銘柄 read as empty, and the reconciliation
// planned to delete both.
//
// Loading-finished rather than response-received: headers arriving says nothing
// about the body being complete, and the page cannot render what it has not
// received.
type apiWatch struct {
	mu       sync.Mutex
	pending  map[network.RequestID]bool
	finished bool
}

// watchFiguresAPI starts listening for a response whose URL contains api.
//
// The listener is bound to ctx, so it goes away with the target rather than
// accumulating one per page load. Events only arrive after this returns, which
// is why it is called immediately before the click: anything the page fetched
// during its own load is already past and cannot be mistaken for the click's
// answer.
func watchFiguresAPI(ctx context.Context, api string) *apiWatch {
	w := &apiWatch{pending: map[network.RequestID]bool{}}

	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			if strings.Contains(e.Request.URL, api) {
				w.mu.Lock()
				w.pending[e.RequestID] = true
				w.mu.Unlock()
			}
		case *network.EventLoadingFinished:
			w.mu.Lock()
			if w.pending[e.RequestID] {
				w.finished = true
			}
			w.mu.Unlock()
		}
	})
	return w
}

// arrived says whether the response has been received in full.
func (w *apiWatch) arrived() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.finished
}
