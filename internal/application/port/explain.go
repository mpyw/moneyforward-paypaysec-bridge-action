package port

// Explainer is an optional extra on a [Ledger], asked — after a write that
// reported success turns out not to have taken effect — whether the service
// said anything about why.
//
// Optional because it is a diagnostic, not a verdict. MoneyForward answers a
// rejected write with 200 and the page re-rendered, and carries unrelated error
// blocks on every render, so what it says cannot decide anything. But it is
// where "名称は20文字以内でお願いします" comes from, and a failure without it is one
// somebody has to reproduce.
type Explainer interface {
	// LastRejection returns what the service said about the most recent write,
	// or "" if it said nothing.
	LastRejection() string
}
