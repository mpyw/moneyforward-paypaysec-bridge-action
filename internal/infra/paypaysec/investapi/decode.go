package investapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/lo"
)

// This file is about one thing: the service does not commit to the JSON type of
// anything it sends.
//
// On the far side it is PHP over a database driver, so whether a scalar keeps its
// quotes follows where the value came from rather than what it means, and whether
// a collection is an object or an array follows whether its keys happen to be
// dense. Both were observed within a single run. A field's JSON type is not
// something to design around one observation of.
//
// Lenient is not the same as guessing. A string that is not a number is refused
// rather than read as zero, because a zero here is an amount and this program acts
// on amounts.

// laxInt64 is a number sent either as a JSON number or as a decimal string.
//
// A missing field stays zero, as it would without this.
type laxInt64 int64

func (n *laxInt64) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var quoted string
		if err := json.Unmarshal(data, &quoted); err != nil {
			return err
		}
		text = strings.TrimSpace(quoted)
	}
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a number", text)
	}
	*n = laxInt64(v)
	return nil
}

// laxString is an identifier sent either quoted or bare.
//
// The digits are kept exactly as they arrived rather than parsed and reprinted:
// this is a name for something, not a quantity, and a round trip through an
// integer is a chance to lose a leading zero or overflow a width nobody promised.
type laxString string

func (l *laxString) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var quoted string
		if err := json.Unmarshal(data, &quoted); err != nil {
			return err
		}
		*l = laxString(strings.TrimSpace(quoted))
		return nil
	}
	*l = laxString(text)
	return nil
}

// keyed is one entry of a [brandList], with the key it arrived under.
//
// Key is empty for the array form, which carries none. Nothing here decides what a
// key means; see [nameHoldings] for how one is joined to a name.
type keyed[T any] struct {
	Key  string
	Item T
}

// brandList is INVEST_BRAND_ARRAY, which arrives in either of two shapes.
//
// Observed live within one run: the ミニアプリ bucket answered with an object keyed
// by brand id, and the アプリ bucket with a bare array. Both are one PHP array on
// the far side, so which one arrives is a property of the account's holdings rather
// than of the endpoint, and neither can be assumed even once.
//
// An empty bucket is `[]` — the array shape again — so refusing to decode it would
// turn every genuinely empty category into a failed run.
type brandList[T any] struct {
	Entries []keyed[T]
}

func (b *brandList[T]) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return nil
	}

	if strings.HasPrefix(text, "[") {
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		b.Entries = lo.Map(items, func(item T, _ int) keyed[T] {
			return keyed[T]{Item: item}
		})
		return nil
	}

	var byKey map[string]T
	if err := json.Unmarshal(data, &byKey); err != nil {
		return err
	}
	// Sorted so that a log line and an error name the holdings in the same order
	// twice running. Map order would be a new order every run.
	keys := lo.Keys(byKey)
	sort.Strings(keys)
	b.Entries = lo.Map(keys, func(key string, _ int) keyed[T] {
		return keyed[T]{Key: key, Item: byKey[key]}
	})
	return nil
}
