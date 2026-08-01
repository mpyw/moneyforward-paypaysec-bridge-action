package otp

import (
	"context"
	"errors"
	"time"
)

// defaultTimeout caps how long any source waits for a code.
const defaultTimeout = 5 * time.Minute

// errWaitedTooLong ends a poll that ran out of time.
//
// A sentinel rather than a message, because the useful message differs by
// source — "no mail matching this query arrived" and "nobody wrote to this
// file" send a reader to different places — and only the source can write it.
var errWaitedTooLong = errors.New("otp: waited too long")

// poller is the wait loop both sources share: try, and if there is nothing yet,
// try again shortly, until the deadline.
//
// A type rather than the loop written out twice. The two copies had already
// drifted on their default cadence, which is a real difference — a mailbox is
// polled over the network and a file is not — but that is the only difference
// there should be, and side by side it was not obvious which others were
// deliberate.
type poller struct {
	timeout  time.Duration
	interval time.Duration
}

// newPoller applies the defaults for whatever the caller left unset.
func newPoller(timeout, interval, defaultInterval time.Duration) poller {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	return poller{timeout: timeout, interval: interval}
}

// run calls attempt until it yields a code, ctx ends, or the timeout elapses.
//
// attempt returning ("", nil) means "nothing yet" and costs another round;
// returning an error abandons the wait. The first attempt happens immediately,
// so a code already waiting is not made to sit out an interval.
func (p poller) run(ctx context.Context, attempt func() (string, error)) (string, error) {
	deadline := time.Now().Add(p.timeout)
	for {
		code, err := attempt()
		if err != nil {
			return "", err
		}
		if code != "" {
			return code, nil
		}
		if time.Now().After(deadline) {
			return "", errWaitedTooLong
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.interval):
		}
	}
}
