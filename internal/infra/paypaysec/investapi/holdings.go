package investapi

import (
	"fmt"
	"strconv"
)

// Holding is one 銘柄 as the API reports it.
//
// Acquisition is derived, not fetched: the per-brand payload carries the current
// value and the unrealised gain, and the cost is the difference. Exact integer
// arithmetic, where the page had it rounded to one decimal place in 万.
type Holding struct {
	BrandID     int
	Name        string
	Yen         int64
	Acquisition int64
}

// nameHoldings joins what the account holds to what the bucket calls it.
//
// The two come from different calls for a reason. top reports holdings; init is a
// catalogue of everything the bucket offers, most of which the account does not
// own. Reading the catalogue as the portfolio invents hundreds of entries.
//
// A holding the catalogue cannot name is refused rather than recorded under a
// blank. The ledger keys on the name, so an entry written without one cannot be
// matched again and the next run creates another.
func nameHoldings(top topResponse, catalogue initResponse, initPath string) ([]Holding, error) {
	names := catalogueNames(catalogue)

	holdings := make([]Holding, 0, len(top.InvestBrandArray.Entries))
	for _, held := range top.InvestBrandArray.Entries {
		id := held.Key
		if id == "" {
			id = strconv.FormatInt(int64(held.Item.BrandID), 10)
		}
		name := names[id]
		if name == "" {
			return nil, fmt.Errorf("brand %s is held but %s does not name it", id, initPath)
		}
		holdings = append(holdings, Holding{
			BrandID:     int(held.Item.BrandID),
			Name:        name,
			Yen:         int64(held.Item.SecuritiesValue),
			Acquisition: int64(held.Item.SecuritiesValue - held.Item.SumGrossProfit),
		})
	}
	return holdings, nil
}

// catalogueNames indexes the catalogue under both the key an entry arrived with
// and its BRAND_ID.
//
// Both, because a holding can arrive with either — the object shape carries a key
// and the array shape does not — and they agree wherever both are present. Keying
// on one alone works until the account's holdings change shape, which is a thing
// they do.
func catalogueNames(catalogue initResponse) map[string]string {
	names := make(map[string]string, len(catalogue.InvestBrandArray.Entries)*2)
	for _, entry := range catalogue.InvestBrandArray.Entries {
		if entry.Key != "" {
			names[entry.Key] = entry.Item.BrandNM
		}
		if entry.Item.BrandID != 0 {
			names[strconv.FormatInt(int64(entry.Item.BrandID), 10)] = entry.Item.BrandNM
		}
	}
	return names
}
