/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"errors"
	"math/rand"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

// RetryRunner receives a function that potentially fails and retries according to the specified strategy
type RetryRunner interface {
	Run(func() error) error
	RunWithErrors(runner func() (bool, error)) error
}

var ErrMaxRetriesExceeded = errors.New("maximum number of retries exceeded")

const Infinitely = -1

type retryRunner struct {
	delay        time.Duration
	interval     int64
	expBackoff   bool
	maxTimes     int
	getNextDelay func() time.Duration

	logger *flogging.FabricLogger
}

func NewRetryRunner(maxTimes int, delay time.Duration, expBackoff bool) *retryRunner {
	rr := &retryRunner{
		delay:      delay,
		expBackoff: expBackoff,
		maxTimes:   maxTimes,
		logger:     flogging.MustGetLogger("retry-runner"),
	}
	rr.getNextDelay = rr.deterministicDelay
	return rr
}

// NewProbabilisticRetryRunner returns a new runner that sets delay to time.Duration(rand.Int63n(f.interval)+1) * time.Millisecond
func NewProbabilisticRetryRunner(maxTimes int, interval int64, expBackoff bool) *retryRunner {
	rr := &retryRunner{
		delay:      0,
		expBackoff: expBackoff,
		maxTimes:   maxTimes,
		interval:   interval,
		logger:     flogging.MustGetLogger("retry-runner"),
	}
	rr.getNextDelay = rr.probabilisticDelay
	return rr
}

func (f *retryRunner) Run(runner func() error) error {
	return f.RunWithErrors(func() (bool, error) {
		err := runner()
		return err == nil, err
	})
}

// RunWithErrors will retry until runner() returns true or until it returns maxTimes false.
// If it returns true, then the error or nil will be returned.
// If it returns maxTimes false, then it will always return an error: either a join of all errors it encountered or a ErrMaxRetriesExceeded.
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
		f.logger.Debugf("Will retry iteration [%d] after delay. %d errors returned so far", i+1, len(errs))
		time.Sleep(f.getNextDelay())
	}
	if len(errs) == 0 {
		return ErrMaxRetriesExceeded
	}
	return errors.Join(errs...)
}

func (f *retryRunner) deterministicDelay() time.Duration {
	delay := f.delay
	if f.expBackoff {
		f.delay = 2 * f.delay
	}
	return delay
}

func (f *retryRunner) probabilisticDelay() time.Duration {
	if f.delay == 0 {
		f.delay = time.Duration(rand.Int63n(f.interval)+1) * time.Millisecond
	}
	return f.deterministicDelay()
}
