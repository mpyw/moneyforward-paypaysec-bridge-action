// Package selector holds every マニュライフ生命 マイページ selector, URL and label,
// plus the facts about its one-time-code mail.
//
// Separate from the code that drives the site for the same reason PayPay 証券's
// is: these are findings, each confirmed against the live pages on a particular
// day, and each is what breaks when the front end changes.
//
// The site is Salesforce Visualforce, and that shapes everything here. Two
// consequences are worth stating before any selector below is read:
//
//   - The contract-detail page numbers its elements by position in a component
//     tree — `j_id0:j_id2:j_id257:0:j_id260:0:…:j_id486`, where the integers are
//     iteration indices. Nothing on that page is addressable by id, and no id
//     from it may be written down here. Its figures are found by label instead;
//     see [Row].
//   - The login page's ids are generated too, but its own validation addresses
//     them by suffix — the page runs `$('[id$=theForm]')`. That is the site
//     saying which half of its ids is stable, so the selectors below match the
//     same way.
package selector

import (
	"regexp"
	"strings"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
)

// Everything here was CONFIRMED against the live site on 2026-08-29.
//
// There was a tier of guesses for a while — candidate selectors the login
// probed, because reaching the challenge step costs a one-time code and it is
// in no other page's markup. That was the right shape for the problem and it
// still cost two sign-ins, because one of the guesses matched the wrong element
// and said it had succeeded. What replaced it is one run's dump and no guesses;
// see the comment on [OTPInput].

// OTPMail is everything the shared OTP sources need for this service.
// CONFIRMED 2026-08-29 against real messages.
var OTPMail = otp.MailSpec{
	Label:   "マニュライフ生命",
	Query:   otpMailQuery,
	Pattern: otpCodePattern,
	Digits:  OTPDigits,
}

// otpMailQuery names where this service's mail comes from.
//
// The sender is on manulife.com, not the manulife.co.jp the site is served
// from. That is not a detail worth rediscovering: a query written from the
// site's own domain matches nothing at all, and a code that never arrives is
// indistinguishable from a mailbox that cannot be read.
//
// in:anywhere because transactional mail can land in Spam, and a code that
// arrived but was filtered looks exactly like one that never came.
//
// No subject filter, for the reason the MoneyForward side documents: a subject
// is a localized string and a fact about the recipient rather than the service.
// Deciding which of a sender's messages carries a code is [otpCodePattern]'s
// job.
const otpMailQuery = `from:manulife_jp_customer_support@manulife.com in:anywhere`

// otpCodePattern pulls the code out of the mail body. CONFIRMED 2026-08-29
// against a real message, an HTML one whose relevant line reads — and the
// spaces here are U+3000, two of them on each side:
//
//	ワンタイムパスワード：　　112233　　<br/><br/>
//
// Anchored to the label rather than hunting for any six digits, and this
// service gives two independent reasons for that. The same body carries two
// call-centre numbers; and 【マニュライフ生命マイページ】ログインのお知らせ arrives
// from the same address thirteen seconds later, well inside the freshness
// window. That notice has no labelled code, so the label is what refuses it.
//
// The separator class is the whole point, and \s is not enough for it. Go's
// \s is ASCII — [\t\n\f\r ] — so it does not match the ideographic space this
// mail actually uses, and a pattern written with \s* matched nothing while
// looking entirely correct. The MoneyForward side had already learned this and
// carries a literal U+3000 in its own class; written as an escape here, because
// an invisible character in source is a fact nobody can review.
//
// U+00A0 is admitted as well. It is not observed — it is what &nbsp; decodes
// to, and this is an HTML mail. Widening what counts as space cannot produce a
// wrong code, only tolerate another way of writing the same gap, which is a
// different thing from guessing at a selector.
//
// The colon is the full-width one in the observed message. The half-width form
// is accepted too — which one a template emits is not something this side gets
// told about.
var otpCodePattern = regexp.MustCompile(`ワンタイムパスワード[：:][\s\x{3000}\x{00A0}]*(\d{6})`)

const (
	// Origin is the site's own host.
	Origin = "https://mypage.manulife.co.jp"

	// LoginURL is the sign-in form, and the only URL here that is ever fetched.
	// CONFIRMED 2026-08-29.
	//
	// Not the site root: that is a landing page whose only control is a button
	// running window.location='/auth'. Pointing at it and expecting a form
	// yields a page with no inputs at all.
	//
	// A completed sign-in lands on /Home?from=auth&cws=1, which holds the
	// contract list. That is not a constant here, and deliberately: asking the
	// site for /Home from the page it had just put us on returned an error page
	// instead of the list. A URL that must not be fetched is not worth naming —
	// it only offers itself to whoever reads this next.
	LoginURL = Origin + "/auth"
)

// The sign-in form. CONFIRMED 2026-08-29 from the live page markup.
//
// Matched on the id's suffix because the prefix — `pageid:thetempPage:` — is
// assigned by Visualforce and is not the site's to promise. The page's own
// jQuery does the same thing, which is the evidence rather than a preference.
const (
	UsernameInput = `input[id$=":username"]`
	PasswordInput = `input[id$=":passw"]`

	// LoginSubmit is an <a> carrying the handler, not the <input type=submit>
	// beside it: that one is named `…:j_id40`, a generated id that will not
	// survive a change to the page.
	LoginSubmit = `a[id$=":login"]`
)

// The OTP challenge. CONFIRMED 2026-08-29 from the live page markup.
//
// It is not a separate page. The sign-in form posts back to itself: on the
// second render the username and password inputs are switched to display:none
// and mirrored into read-only companions, and these two appear. So the URL does
// not change, and neither does the form.
//
// Both selectors below were arrived at the hard way, and the way they were got
// wrong is worth keeping:
//
//	<input id="…:inputpassword" type="tel"    class="form-control imeInactive imemaxLen">
//	<input id="…:otpsend"       type="submit" value="ログイン">
//
// The field is called inputpassword. It does not contain "otp" anywhere — the
// button does. A probe looking for `input[name*="otp" i]` therefore matched the
// submit button, reported that it had found the code field, and the login
// clicked it: the form went through with an empty code, the page re-rendered
// looking untouched, and the six digits were typed into nothing. Every step
// reported success.
//
// That is the whole argument against a loose selector. A wrong one that matches
// nothing fails honestly; a wrong one that matches something else does the
// wrong thing quietly. It is also why [Row] and the labels above are matched on
// text rather than on anything that merely looks related.
const (
	// OTPInput is the code field. type="tel", one field for all six digits.
	OTPInput = `input[id$=":inputpassword"]`

	// OTPSubmit is the button that submits the code — not [LoginSubmit].
	//
	// The anchor that submits the credentials is still in the page at this
	// point, and still looks like the primary button, but it carries
	// style="pointer-events: none;" on this render. Clicking it does nothing at
	// all, which is a failure with no error attached to it.
	OTPSubmit = `input[id$=":otpsend"]`
)

// OTPDigits is how long the code is. CONFIRMED 2026-08-29 from the mail.
const OTPDigits = 6

// The contract list on /Home. CONFIRMED 2026-08-29.
//
// The one part of this site with real class names, and the reason the read
// starts here rather than at a saved contract URL. A card looks like:
//
//	<div class="c-card c-card--v1"
//	     onclick="RedirectToPageOrFFFModal(false,'homeToPolDetail','<token>')">
//	  <div class="c-card__head"><div class="c-card__brand">
//	    <div class="c-card__title">テスト終身保険</div>
//	  <div class="c-card__body"><div class="c-desc-table"><table>
//	    <tr><th>種類-証券番号：</th><td class="tdCss">…</td></tr>
//	    <tr><th>契約日：</th><td class="tdCss">…</td></tr>
//	    <tr><th>契約状況 :</th><td class="tdCss">契約継続中</td></tr>
const (
	ContractCard      = ".c-card"
	ContractCardTitle = ".c-card__title"
	ContractCardTable = ".c-desc-table"

	// ContractMarkAttr is put on the card that is about to be opened, and
	// MarkedContract addresses it.
	//
	// The site has no attribute of its own that identifies a card, and the one
	// thing that does — the 種類-証券番号 in a cell — cannot be written as a CSS
	// selector. So the page is asked to mark it, and then to open the mark.
	ContractMarkAttr = "data-mfpp-open"
	MarkedContract   = "[" + ContractMarkAttr + "]"
)

// ContractOpenerReady is true once the card's click handler can run.
// CONFIRMED 2026-08-29 from the page's own markup.
//
// A card opens itself with an inline onclick calling
// RedirectToPageOrFFFModal(…), which one of the page's scripts defines. Until
// that script has run, clicking the card raises a ReferenceError inside the
// handler — and an exception in an event handler goes to window.onerror, not to
// whoever dispatched the click. The click reports success and the page does not
// move.
//
// The cards themselves are server-rendered, so waiting for one to be visible
// says nothing about this. That gap is what three attempts at fixing the click
// were actually chasing.
//
// Waited for rather than slept past. A sleep is a guess about someone else's
// machine on a day nobody has seen yet — the 投資信託 reader spent three
// releases learning that, and its lesson is that a wait needs something
// definite to be waiting for. This is definite: the function is there or it is
// not.
const ContractOpenerReady = `typeof RedirectToPageOrFFFModal === 'function'`

// Labels, written without their trailing punctuation, because the site does not
// punctuate them consistently. CONFIRMED 2026-08-29:
//
//	list card:  種類-証券番号：   full-width colon (U+FF1A)
//	detail:     種類-証券番号:    half-width colon (U+003A)
//	list card:  契約状況 :        a space, then a half-width colon
//
// The same label, three ways, on one site. So they are stored bare and compared
// through [TrimLabel]. Matching them literally would work on whichever page was
// looked at first and silently find nothing on the other — and finding nothing
// is how a category comes to look empty, which is what this program deletes
// things over.
//
// 契約状況 is read rather than ignored: a lapsed or paid-out contract is not one
// whose figures should keep being recorded, and the list is the only place that
// says so.
const (
	LabelPolicyNumber = "種類-証券番号"
	LabelProductName  = "商品名"
	LabelStatus       = "契約状況"
	StatusInForce     = "契約継続中"
)

// TrimLabel strips the punctuation a label may or may not carry.
//
// Both colon widths, and whitespace of both widths around them: this site
// writes "契約状況 :" with a leading space, and its OTP mail separates a label
// from its value with U+3000 — the same page authors, the same habit. Go's
// strings.TrimSpace does not touch U+3000 either, so the cutset is explicit.
func TrimLabel(s string) string {
	return strings.Trim(s, ": \t\r\n：　")
}

// A contract's URL cannot be stored. CONFIRMED 2026-08-29 by observing two
// sign-ins produce two different values for the same single contract.
//
// The detail page is reached as /policyinquiry?id=<token>, and the token is
// minted per rendering of the list — it is a continuation, not an identifier.
// So there is no shortcut past the list, and nothing here may cache one.
//
// It also inverts a rule this project learned elsewhere. PayPay 証券's holdings
// are identified by the URL they land on, precisely because a URL does not move
// while a price does. Here the URL is the thing that moves, and identity has to
// come from the page: see [DetailPolicyType].
const PolicyPathPrefix = "/policyinquiry"

// The contract detail page. CONFIRMED 2026-08-29.
//
// Every figure sits in a row of this shape, and none of it is addressable by
// id:
//
//	<tr class="row">
//	  <th class="col-md-3 col-xs-5"><span><span><p>解約時お支払金額（円支払）</p></span></span></th>
//	  <td class="col-md-9 col-xs-7"><span><p><span class="customerCareLink">1,234,567 円</span></p></span></td>
//	</tr>
//
// So a figure is found by matching the label cell's text and reading the value
// cell — which is why the labels below are constants and the ids are not
// mentioned.
//
// Every value must be read from a *visible* element. CONFIRMED 2026-08-29: the
// detail page for a single contract also carried a 変額保険 section the customer
// does not hold, including two panels — one for a zero balance, one for a
// non-zero one — both display:none. Reading text without regard to visibility
// mixes another product's figures into this one's.
const (
	Row       = "tr"
	RowLabel  = "th"
	RowValue  = "td"
	ValueText = "span.customerCareLink"
)

// The labels a reading needs. CONFIRMED 2026-08-29.
//
// 解約時お支払金額（円支払）is the only yen figure the contract has: 積立金額 and
// 払込保険料 are stated in 米ドル and nothing on the page converts them. That
// makes it the figure to record, by elimination rather than by choice.
//
// The site states its own conversion rate and the date it was struck, so
// 契約通貨支払 × レート is an arithmetic check on 円支払 — the same shape as
// 投資元本 + 含み益 = 評価額合計 on the PayPay 証券 side, and for the same reason:
// it is the source checking itself, not this program inventing a rate it has no
// way to verify.
const (
	LabelSurrenderYen = "解約時お支払金額（円支払）"
	LabelSurrenderFCY = "解約時お支払金額（契約通貨支払）"
	LabelRate         = "円換算レート"
	LabelRateDate     = "円換算為替基準日"

	LabelAccountValue     = "積立金額"
	LabelAccountValueDate = "積立金計算基準日"
	LabelPremiumPaid      = "払込保険料"
)

// The summary block at the top of a contract's detail page. CONFIRMED
// 2026-08-29, and the second place on this site with real class names:
//
//	<div class="policySummary c-box-v1 clearfix">
//	  <div class="row row-margin">
//	    <div class="col-md-5 col-xs-4">商品名:</div>
//	    <div class="bold col-md-7 col-xs-8">テスト終身保険</div>
//	  </div>
//	  <div class="row row-margin">
//	    <div class="col-md-5 col-xs-4">種類-証券番号:</div>
//	    <div class="bold col-md-7 col-xs-8">…</div>
//
// This is where a detail page proves which contract it is showing. The URL
// cannot: see [PolicyPathPrefix]. 種類-証券番号 is the anchor because the list
// card carries it too, so the two can be compared — where 商品名 and 保険種類
// are both true of every contract of the same product, and would not tell two
// of them apart.
const (
	PolicySummary      = ".policySummary"
	SummaryRow         = ".row-margin"
	SummaryValueMarker = ".bold"
)

// DetailPolicyType is the label whose value names the kind of insurance, in the
// 【基本情報】 table. CONFIRMED 2026-08-29.
//
// Not the identity — [PolicySummary] is — but it is what says the contract is
// the foreign-currency single-premium kind whose figures this code knows how to
// read. The card in the list gives the product's brand name instead, which is a
// different string for the same contract.
const DetailPolicyType = "保険種類"

// HomeCandidates are ways to tell a sign-in completed and the contract list
// rendered. CONFIRMED 2026-08-29: both are present on /Home.
//
// Two of them, and raced rather than chosen, because they fail differently. The
// wrapper renders whether or not there is a contract in it — which also means it
// can be empty and therefore invisible. A card is proof there is something to
// read, but an account with no contracts has none.
var HomeCandidates = map[string]string{
	"home-card": ContractCard,
	"home-wrap": ".c-card-wrap",
}
