/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// ErrMaxRetriesExceeded is returned, possibly joined with the last attempt's
// error, when a runner exhausts its attempts without success.
var ErrMaxRetriesExceeded = errors.New("maximum number of retries exceeded")

// Infinitely, passed as maxTimes, makes a runner retry until success or,
// for RunContext, until the context ends.
const Infinitely = -1

type typedRetryRunner[V any] struct {
	*retryRunner
}

// NewTypedRetryRunner returns a runner like NewRetryRunner whose Run also
// returns the value produced by the successful attempt.
func NewTypedRetryRunner[V any](maxTimes int, delay time.Duration, expBackoff bool) *typedRetryRunner[V] {
	return &typedRetryRunner[V]{retryRunner: NewRetryRunner(maxTimes, delay, expBackoff)}
}

// Run retries runner like retryRunner.Run and returns the value of the first
// successful attempt, or the zero value and the error.
func (r *typedRetryRunner[V]) Run(runner func() (V, error)) (V, error) {
	var result V
	err := r.retryRunner.Run(func() error {
		v, e := runner()
		if e != nil {
			return e
		}
		result = v
		return nil
	})
	return result, err
}

// retryRunner runs a function up to maxTimes times, waiting between attempts.
// It is stateful: each delay it hands out advances the backoff, so a runner
// must not be shared between goroutines, and the delay sequence restarts only
// with a new runner.
type retryRunner struct {
	delay        time.Duration
	maxDelay     time.Duration
	interval     int64
	expBackoff   bool
	maxTimes     int
	getNextDelay func() time.Duration

	logger logging.Logger
}

// NewRetryRunner returns a runner that waits delay between attempts, doubling
// it after each attempt if expBackoff is set.
func NewRetryRunner(maxTimes int, delay time.Duration, expBackoff bool) *retryRunner {
	rr := &retryRunner{
		delay:      delay,
		expBackoff: expBackoff,
		maxTimes:   maxTimes,
		logger:     logging.MustGetLogger(),
	}
	rr.getNextDelay = rr.deterministicDelay
	return rr
}

// NewProbabilisticRetryRunner returns a runner whose first delay is a random
// value in [1, interval] milliseconds. Later delays follow NewRetryRunner.
func NewProbabilisticRetryRunner(maxTimes int, interval int64, expBackoff bool) *retryRunner {
	rr := &retryRunner{
		delay:      0,
		expBackoff: expBackoff,
		maxTimes:   maxTimes,
		interval:   interval,
		logger:     logging.MustGetLogger(),
	}
	rr.getNextDelay = rr.probabilisticDelay
	return rr
}

// NewBackoffRetryRunner returns a runner with exponential backoff and full
// jitter: the n-th delay is drawn uniformly from [0, min(initial*2^n, maxDelay)).
// The randomness on every attempt keeps many nodes from retrying a recovering
// service in lockstep, and maxDelay bounds the wait. If maxDelay < initial,
// maxDelay is raised to initial. A non-positive initial retries without waiting.
func NewBackoffRetryRunner(maxTimes int, initial, maxDelay time.Duration) *retryRunner {
	rr := &retryRunner{
		delay:    initial,
		maxDelay: max(initial, maxDelay),
		maxTimes: maxTimes,
		logger:   logging.MustGetLogger(),
	}
	rr.getNextDelay = rr.jitteredDelay
	return rr
}

// NextDelay returns the next delay and advances the backoff. It serves loops
// that manage their own attempts and only need the wait between them.
func (f *retryRunner) NextDelay() time.Duration {
	return f.getNextDelay()
}

// Run retries runner until it returns nil or maxTimes attempts fail; see
// RunWithErrors for the returned error.
func (f *retryRunner) Run(runner func() error) error {
	return f.RunWithErrors(func() (bool, error) {
		err := runner()
		return err == nil, err
	})
}

// RunWithErrors will retry until runner() returns true or until it returns maxTimes false.
// If it returns true, then the error or nil will be returned.
// If it returns maxTimes false, then it will always return an error: either a join of all errors it encountered or an ErrMaxRetriesExceeded.
// The wait between attempts cannot be interrupted; use RunContext when the caller must be able to stop.
func (f *retryRunner) RunWithErrors(runner func() (bool, error)) error {
	errs := make([]error, 0)
	for i := 0; f.maxTimes < 0 || i < f.maxTimes; i++ {
		terminate, err := runner()
		if terminate {
			return err
		}
		if err != nil {
			errs = append(errs, err)
		}
		if i+1 == f.maxTimes {
			break
		}
		f.logger.Debugf("Will retry iteration [%d] after delay. %d errors returned so far", i+1, len(errs))
		time.Sleep(f.getNextDelay())
	}
	if len(errs) == 0 {
		return ErrMaxRetriesExceeded
	}
	return errors.Join(errs...)
}

// RunContext retries runner until it returns nil, maxTimes attempts fail, or
// ctx ends. The wait between attempts stops as soon as ctx is done.
//
// Only the last attempt's error is kept, so memory stays constant under
// Infinitely. Once maxTimes is reached, the result is ErrMaxRetriesExceeded
// joined with that error. If ctx ends first, the result wraps ctx.Err(),
// joined with the last attempt's error if there was one; errors.Is matches
// context.Canceled or context.DeadlineExceeded.
func (f *retryRunner) RunContext(ctx context.Context, runner func() error) error {
	var lastErr error
	for i := 0; f.maxTimes < 0 || i < f.maxTimes; i++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(errors.Wrapf(err, "retry stopped (attempts: %d)", i), lastErr)
		}
		if lastErr = runner(); lastErr == nil {
			return nil
		}
		if i+1 == f.maxTimes {
			break
		}
		delay := f.getNextDelay()
		f.logger.Debugf("Attempt [%d] failed, retrying in [%v]: %v", i+1, delay, lastErr)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done(): // reported by the ctx check at the top of the loop
		case <-timer.C:
		}
		timer.Stop()
	}
	return errors.Join(ErrMaxRetriesExceeded, lastErr)
}

func (f *retryRunner) deterministicDelay() time.Duration {
	delay := f.delay
	if f.expBackoff {
		f.delay = double(f.delay, math.MaxInt64)
	}
	return delay
}

func (f *retryRunner) probabilisticDelay() time.Duration {
	if f.delay == 0 {
		f.delay = time.Duration(rand.Int64N(f.interval)+1) * time.Millisecond
	}
	return f.deterministicDelay()
}

func (f *retryRunner) jitteredDelay() time.Duration {
	ceiling := f.delay
	if ceiling <= 0 {
		return 0
	}
	f.delay = double(f.delay, f.maxDelay)
	return rand.N(ceiling)
}

// double returns 2*d, capped at limit and never overflowing.
func double(d, limit time.Duration) time.Duration {
	if d > limit/2 {
		return limit
	}
	return 2 * d
}
