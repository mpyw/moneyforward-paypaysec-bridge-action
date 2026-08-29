// Package port declares what a use case needs from the outside world.
//
// One operation per method, each a single act of communication: sign in, read,
// create, update, delete. Not "reconcile the ledger" — that is the use case,
// and an interface shaped like it hands the whole decision to whoever
// implements it. The order of the writes, what has to be read back afterwards
// and what counts as having worked then live in infrastructure, where they
// cannot be read without a browser or tested without a website.
//
// So these are deliberately dumb. An implementation is expected to know how to
// talk to its service and nothing else: which identifier addresses a row, which
// token a form needs, how a session is established. It is not expected to know
// why it is being asked.
//
// Everything is in domain terms. The direction is enforced — internal/application
// may not import internal/infra; see the layering test one level up.
//
// One file per party the program deals with, rather than one file of
// interfaces: what an implementer has to provide is what a whole file says,
// and which use case wants it follows from that.
//
//	source.go      an account holdings are read from, of which there are several
//	ledger.go      the account they are recorded in
//	secret.go      where the scheduled job's credentials are kept
//	credential.go  granting and keeping a credential to read mail with
//	reporter.go    the caller, told what is happening
package port
