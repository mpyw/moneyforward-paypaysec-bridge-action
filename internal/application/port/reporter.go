package port

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/portfolio"
)

// Reporter receives progress. Optional; a nil Reporter is silent.
//
// The odd one out here: not a service the program talks to, but the caller it
// reports back to. An interface rather than a logger, so the use case says what
// happened and the caller decides how it is phrased — and, in the scheduled
// job, what has to be masked before it can be phrased at all.
type Reporter interface {
	Phase(name string)

	// Everything below names its source. There is more than one, they are read
	// and recorded independently, and a line that does not say which one it is
	// about is a line nobody can act on.
	ReadResult(source string, assets []asset.Asset)

	// Planned is called before anything is written, Applied after everything
	// was. Two calls rather than one, because the checks that can refuse the
	// whole thing run between them: a single report at plan time reads as a
	// result, and a run that was refused looks like a run that succeeded.
	Planned(source string, plan portfolio.Plan)
	Applied(source string, plan portfolio.Plan)

	// Failed reports a source that could not be completed, at the moment it is
	// given up on.
	//
	// The run carries on with the others and fails at the end, so without this
	// the only account of what went wrong arrives after everything that
	// succeeded, about something that happened minutes earlier.
	Failed(source string, err error)
}
