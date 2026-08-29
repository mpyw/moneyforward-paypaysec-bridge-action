// Package adapter implements the ports the use cases are served through.
//
// Its own package so a use case depends on an interface rather than on a
// browser, and so the site packages stay unaware there is a use case at all.
//
// The methods are thin on purpose. Each is a single act of communication,
// because that is the shape of [port]: what to do with them is the use case's
// business, and anything resembling a decision appearing here is in the wrong
// place.
//
// One file per port implemented, matching internal/application/port — so the
// interface and the thing satisfying it are found under the same name.
//
//	source_paypaysec.go  PayPay 証券, read from
//	source_manulife.go   マニュライフ生命, read from
//	ledger.go            a MoneyForward manual account, written to
//
// One file per source rather than one file of sources, for the same reason
// [port] keeps one file per party: what an implementer has to provide is what a
// whole file says. The ledger is one file and several instances — the sources
// write into an account each, and which account is the caller's to decide.
package adapter
