package investapi

import (
	"fmt"

	"github.com/samber/lo"
)

// checked is anything this package decodes: every reply carries the envelope, and
// no reply is read for its numbers before the envelope has been believed.
type checked interface{ check(path string) error }

// envelope is what every one of these replies carries, and what has to be true
// before anything else in it is worth reading.
//
// Both fields are checked because the page checks both, and because of what the
// second one costs to miss. A signed-out session answers with LOGIN_STATUS 1 and
// no holdings — which, taken at face value, is a category that emptied, and this
// program deletes those. An expired cookie must not be able to look like a sale.
type envelope struct {
	// Status is 0 on success. Anything else is an error, described in Messages.
	Status laxInt64 `json:"STATUS"`

	// LoginStatus is 1 when the session is no longer signed in.
	LoginStatus laxInt64 `json:"LOGIN_STATUS"`

	Messages []struct {
		Message string `json:"MESSAGE"`
	} `json:"MESSAGE_ARRAY"`
}

// check reports whatever is wrong with the reply, before its numbers are used.
func (e envelope) check(path string) error {
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
type topResponse struct {
	envelope
	SecuritiesValueTotal        laxInt64 `json:"SECURITIES_VALUE_TOTAL"`
	TotalAcquisitionFeeTaxTotal laxInt64 `json:"TOTAL_ACQUISITION_FEE_TAX_TOTAL"`
	SumGrossProfitTotal         laxInt64 `json:"SUM_GROSS_PROFIT_TOTAL"`

	// InvestBrandArray is the holdings, and only the holdings.
	InvestBrandArray brandList[struct {
		BrandID         laxInt64 `json:"BRAND_ID"`
		SecuritiesValue laxInt64 `json:"SECURITIES_VALUE"`
		SumGrossProfit  laxInt64 `json:"SUM_GROSS_PROFIT"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// initResponse is pc_invest_init: the catalogue of every 銘柄 the bucket offers,
// which is where names come from and which is not a portfolio.
type initResponse struct {
	envelope
	InvestBrandArray brandList[struct {
		BrandID laxInt64 `json:"BRAND_ID"`
		BrandNM string   `json:"BRAND_NM"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// infoResponse is pc_invest_info: what the account is. See [Info].
type infoResponse struct {
	envelope
	MiniClientSeqNo laxString `json:"MINI_CLIENT_SEQ_NO"`
	InvTrustUsable  laxString `json:"INV_TRUST_USABLE"`
	PPKYC           laxString `json:"PP_KYC"`
}
