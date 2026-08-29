package asset

import "testing"

// TestKindsCoversEveryValidKind ties the list to the predicate.
//
// They are two statements of one thing, and the list is what tells the
// MoneyForward side which of its 資産クラス options nothing maps to. A kind
// added to the constants and forgotten here would simply not appear there — a
// gap that looks like a complete answer.
func TestKindsCoversEveryValidKind(t *testing.T) {
	listed := map[Kind]bool{}
	for _, k := range Kinds() {
		if !k.Valid() {
			t.Errorf("Kinds() includes %v, which Valid() rejects", k)
		}
		if listed[k] {
			t.Errorf("Kinds() lists %v twice", k)
		}
		listed[k] = true
	}

	// Every kind the type can hold, walked from the zero value up: a constant
	// added to the iota block is caught here rather than by whoever notices its
	// absence downstream.
	for k := KindUnknown; k < KindUnknown+64; k++ {
		if k.Valid() && !listed[k] {
			t.Errorf("%v is valid but Kinds() does not list it", k)
		}
	}
	if KindUnknown.Valid() {
		t.Error("the zero value reports itself as valid")
	}
}
