// Package helpers is the parent of the small utilities under internal/infra
// that are not adapters to anything.
//
// Everything else at that level speaks to something outside the program — a
// site, a browser, a mailbox, a runner's log. What lives here does not, and is
// grouped so that reading internal/infra shows what this program talks to
// rather than a mixture of that and its odds and ends.
//
// This package holds no code of its own.
package helpers
