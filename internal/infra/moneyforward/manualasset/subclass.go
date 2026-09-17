package manualasset

import (
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
)

// AssetSubclass identifies what kind of instrument an entry holds. The values
// come from the create form's own select.
type AssetSubclass int

const (
	SubclassDomesticStock AssetSubclass = 14 // 国内株
	SubclassUSStock       AssetSubclass = 15 // 米国株
	SubclassOtherStock    AssetSubclass = 17 // その他株式
	SubclassMutualFund    AssetSubclass = 12 // 投資信託

	// SubclassSavingsInsurance is 積立型保険.
	//
	// CONFIRMED 2026-08-29 from the create form's own select, which offers
	// sixty-odd classes; see `mfpp debug mf subclasses`.
	//
	// The alternative considered was 外債 (9), which describes what the contract
	// is invested in rather than what is owned. Either files a correct figure;
	// they differ only in how the portfolio reads, and this one matches the
	// thing the statement is about.
	SubclassSavingsInsurance AssetSubclass = 32
)

// SubclassFor is how MoneyForward files an instrument of this kind.
//
// The translation lives here, next to the identifiers it produces, because they
// are this site's numbering and no one else's business. PayPay 証券's target
// table used to carry them directly.
func SubclassFor(kind asset.Kind) (AssetSubclass, error) {
	switch kind {
	case asset.DomesticStock:
		return SubclassDomesticStock, nil
	case asset.USStock:
		return SubclassUSStock, nil
	case asset.OtherStock:
		return SubclassOtherStock, nil
	case asset.MutualFund:
		return SubclassMutualFund, nil
	case asset.SavingsInsurance:
		return SubclassSavingsInsurance, nil
	}
	// Not a default on the switch: an unrecognised kind must not quietly become
	// whatever the zero value files as.
	return 0, fmt.Errorf("no MoneyForward 資産クラス for %s", kind)
}

// KindOfSubclass is SubclassFor backwards: what a recorded subclass means.
//
// Needed to read the account back in the same terms it is written in. An
// unrecognised value becomes the zero Kind, which nothing will accept for a
// write — reading is not the place to refuse.
func KindOfSubclass(subclass AssetSubclass) asset.Kind {
	switch subclass {
	case SubclassDomesticStock:
		return asset.DomesticStock
	case SubclassUSStock:
		return asset.USStock
	case SubclassOtherStock:
		return asset.OtherStock
	case SubclassMutualFund:
		return asset.MutualFund
	case SubclassSavingsInsurance:
		return asset.SavingsInsurance
	}
	return asset.KindUnknown
}
