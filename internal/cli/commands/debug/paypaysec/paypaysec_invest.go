package paypaysec

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/cookiestore"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/investapi"
)

func investCommand() *cli.Command {
	var trace, viaPage bool
	return &cli.Command{
		Name:  "invest",
		Usage: "read both 投資信託 buckets through the API the page calls, and say what the account is",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "trace",
				Usage:       "print each reply verbatim (holds the account's balances)",
				Destination: &trace,
			},
			&cli.BoolFlag{
				Name: "via-page",
				Usage: "make the same call from inside the page instead, so a refusal " +
					"can be attributed to the body or to everything else",
				Destination: &viaPage,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runInvest(ctx, session.From(cmd), trace, viaPage)
		},
	}
}

// runInvest exercises the 投資信託 endpoints one bucket at a time.
//
// It exists because the service answers a request it does not like with STATUS 9
// and システムの不具合, which names nothing that can be acted on. Three releases
// went into guessing at that from CI — one guess per run, at a login and a full
// scrape each — and the guessing is the problem, not the guesses. A person with
// the reply in front of them settles it once.
//
// Each bucket is reported separately rather than aborting the way the job does:
// what is worth knowing here is which of the two failed and how they differ.
func runInvest(ctx context.Context, opts *session.Options, trace, viaPage bool) error {
	s, err := opts.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	// Any page on the site will do; this one is the 投資信託 screen so that the
	// session is the same one the job's read would be made with.
	if err := s.Open("https://www.paypay-sec.co.jp/investment_trust/"); err != nil {
		return err
	}

	http, err := cookiestore.HTTPClientFor(s.Context())
	if err != nil {
		return fmt.Errorf("borrow session: %w", err)
	}
	client := &investapi.Client{HTTP: http}
	if trace {
		client.Trace = func(path string, fields map[string]string, body []byte) {
			_, _ = fmt.Fprintf(os.Stderr, "\n  ── %s\n     sent %v\n     %s\n", path, fields, body)
		}
	}

	// The account first: the page shows the ミニアプリ tab only when the client
	// number and INV_TRUST_USABLE are both set, so an account failing that has no
	// ミニアプリ 投資信託 at all — which is a different thing from a read that
	// failed, and looks identical from the job's side.
	info, ierr := client.ReadInfo(ctx)
	if ierr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "  ! info: %v\n", ierr)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "\nAccount:\n"+
			"  MINI_CLIENT_SEQ_NO  %q\n"+
			"  INV_TRUST_USABLE    %q\n"+
			"  PP_KYC              %q\n",
			info.MiniClientSeqNo, info.InvTrustUsable, info.PPKYC)
	}

	if viaPage {
		return fetchFromPage(s.Context(), info.MiniClientSeqNo)
	}

	failures := 0
	for _, bucket := range []struct {
		name string
		kind investapi.Bucket
	}{
		{"アプリ", investapi.App},
		{"ミニアプリ", investapi.MiniApp},
	} {
		figures, err := client.Read(ctx, bucket.kind)
		if err != nil {
			failures++
			_, _ = fmt.Fprintf(os.Stderr, "\n%s: %v\n", bucket.name, err)
			continue
		}
		_, _ = fmt.Fprintf(os.Stderr, "\n%s: 評価額合計 %d  取得原価 %d  含み益 %d  (%d 銘柄)\n",
			bucket.name, figures.Total, figures.Acquisition, figures.Gain, len(figures.Holdings))
		for _, h := range figures.Holdings {
			_, _ = fmt.Fprintf(os.Stderr, "  [%d] %-40q %d (cost %d)\n",
				h.BrandID, h.Name, h.Yen, h.Acquisition)
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d bucket(s) could not be read", failures)
	}
	return nil
}

// fetchFromPage repeats the ミニアプリ call from inside the document.
//
// The body Go sends is field-for-field what the page's own transport builds, and
// the service still refuses it. That leaves everything a request carries besides
// its body — headers, above all — and the way to find out is to let the page make
// the call and see whether the same body is accepted there.
//
// If it is, the difference is a header, and this narrows it from "the endpoint
// rejects us" to a list that can be tried one at a time. If it is not, the body is
// wrong after all and the bundle is not the whole contract.
func fetchFromPage(ctx context.Context, seq string) error {
	const miniInitPath = "/v3/invest/brand/pc_invest_init"
	const script = `(async () => {
	  const body = new FormData();
	  body.append('APP_VERSION', '');
	  body.append('UUID', 'uuid_pc');
	  body.append('DEVICE_TOKEN', 'device_token');
	  body.append('OS', 'pc');
	  body.append('APP_ID', '6');
	  body.append('MINI_CLIENT_SEQ_NO', %q);
	  const res = await fetch('/v3/invest/brand/pc_invest_init', {
	    method: 'POST', body, credentials: 'include',
	  });
	  return res.status + ' ' + (await res.text());
	})()`

	// The headers the browser puts on it, captured rather than guessed. Guessing
	// is what the last three releases were.
	headers := make(chan map[string]any, 1)
	chromedp.ListenTarget(ctx, func(ev any) {
		sent, ok := ev.(*network.EventRequestWillBeSent)
		if !ok || !strings.Contains(sent.Request.URL, miniInitPath) {
			return
		}
		select {
		case headers <- sent.Request.Headers:
		default:
		}
	})

	var out string
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Evaluate(fmt.Sprintf(script, seq), &out,
			func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}))
	if err != nil {
		return fmt.Errorf("fetch from the page: %w", err)
	}

	status := out
	if i := strings.IndexByte(out, ' '); i > 0 {
		status = out[:i]
	}
	_, _ = fmt.Fprintf(os.Stderr, "\nミニアプリ init, called by the page itself: HTTP %s (%d bytes)\n",
		status, len(out))

	select {
	case h := <-headers:
		names := make([]string, 0, len(h))
		for k := range h {
			names = append(names, k)
		}
		sort.Strings(names)
		_, _ = fmt.Fprintf(os.Stderr, "\nHeaders the browser sent:\n")
		for _, k := range names {
			// Cookie is the session itself, and Content-Type carries a boundary
			// that changes per request. Neither is what this is looking for.
			if strings.EqualFold(k, "Cookie") {
				_, _ = fmt.Fprintf(os.Stderr, "  %-26s <%d bytes, not printed>\n", k, len(fmt.Sprint(h[k])))
				continue
			}
			_, _ = fmt.Fprintf(os.Stderr, "  %-26s %v\n", k, h[k])
		}
	default:
		_, _ = fmt.Fprintln(os.Stderr, "\n(the request was not observed)")
	}
	return nil
}
