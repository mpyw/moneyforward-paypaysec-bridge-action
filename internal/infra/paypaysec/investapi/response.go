package investapi

import (
	"fmt"

	"github.com/samber/lo"
)

// checkedResponse is anything this package decodes: every reply carries the responseEnvelope, and
// no reply is read for its numbers before the responseEnvelope has been believed.
//
//declscope:package // the transport believes every reply through this
type checkedResponse interface{ check(path string) error }

// responseEnvelope is what every one of these replies carries, and what has to be true
// before anything else in it is worth reading.
//
// Both fields are checkedResponse because the page checks both, and because of what the
// second one costs to miss. A signed-out session answers with LOGIN_STATUS 1 and
// no holdings — which, taken at face value, is a category that emptied, and this
// program deletes those. An expired cookie must not be able to look like a sale.
type responseEnvelope struct {
	// Status is 0 on success. Anything else is an error, described in Messages.
	Status laxInt64 `json:"STATUS"`

	// LoginStatus is 1 when the session is no longer signed in.
	LoginStatus laxInt64 `json:"LOGIN_STATUS"`

	Messages []struct {
		Message string `json:"MESSAGE"`
	} `json:"MESSAGE_ARRAY"`
}

// check reports whatever is wrong with the reply, before its numbers are used.
//
//declscope:package // called by the transport on every reply
func (e responseEnvelope) check(path string) error {
	if e.LoginStatus == 1 {
		return fmt.Errorf("%s reports the session is signed out; its empty holdings "+
			"are not a portfolio", path)
	}
	if e.Status != 0 {
		// The service's own words. STATUS 9 is システムの不具合, which names nothing,
		// so the message is passed on rather than interpreted.
		if detail, ok := lo.Find(e.Messages, func(m struct {
			Message string `json:"MESSAGE"`
		}) bool {
			return m.Message != ""
		}); ok {
			return fmt.Errorf("%s returned STATUS %d: %s", path, e.Status, detail.Message)
		}
		return fmt.Errorf("%s returned STATUS %d", path, e.Status)
	}
	return nil
}

// topResponse is pc_invest_top: the holdings and the totals over them.
//
//declscope:package // holding.go joins the two replies it decodes
type topResponse struct {
	responseEnvelope
	SecuritiesValueTotal        laxInt64 `json:"SECURITIES_VALUE_TOTAL"`
	TotalAcquisitionFeeTaxTotal laxInt64 `json:"TOTAL_ACQUISITION_FEE_TAX_TOTAL"`
	SumGrossProfitTotal         laxInt64 `json:"SUM_GROSS_PROFIT_TOTAL"`

	// InvestBrandArray is the holdings, and only the holdings.
	InvestBrandArray laxBrandList[struct {
		BrandID         laxInt64 `json:"BRAND_ID"`
		SecuritiesValue laxInt64 `json:"SECURITIES_VALUE"`
		SumGrossProfit  laxInt64 `json:"SUM_GROSS_PROFIT"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// initResponse is pc_invest_init: the catalogue of every 銘柄 the bucket offers,
// which is where names come from and which is not a portfolio.
//
//declscope:package // holding.go joins the two replies it decodes
type initResponse struct {
	responseEnvelope
	InvestBrandArray laxBrandList[struct {
		BrandID laxInt64 `json:"BRAND_ID"`
		BrandNM string   `json:"BRAND_NM"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// infoResponse is pc_invest_info: what the account is. See [Info].
//
//declscope:package // account.go decodes the info reply
type infoResponse struct {
	responseEnvelope
	MiniClientSeqNo laxString `json:"MINI_CLIENT_SEQ_NO"`
	InvTrustUsable  laxString `json:"INV_TRUST_USABLE"`
	PPKYC           laxString `json:"PP_KYC"`
}
