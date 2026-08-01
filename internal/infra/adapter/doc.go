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
//	broker.go  PayPay 証券, read from
//	ledger.go  MoneyForward, written to
package adapter
