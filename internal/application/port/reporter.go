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
	ReadResult(assets []asset.Asset)

	// Planned is called before anything is written, Applied after everything
	// was. Two calls rather than one, because the checks that can refuse the
	// whole plan run between them: a single report at plan time reads as a
	// result, and a run that was refused looks like a run that succeeded.
	Planned(plan portfolio.Plan)
	Applied(plan portfolio.Plan)
}
