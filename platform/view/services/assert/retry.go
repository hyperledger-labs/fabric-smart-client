/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package assert

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// EventuallyWithRetry will call the function provided, and asserts that the
// function returns with no error within the provided number of attempts.
func EventuallyWithRetry(t *testing.T, attempts int, sleep time.Duration, f func() error, msgAndArgs ...interface{}) {
	assert.NoError(t, Retry(attempts, sleep, f), msgAndArgs...)
}

// Retry retries the given function until it returns nil or the given
// number of attempts has been reached.
func Retry(attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(sleep)
			sleep *= 2
		}

		err = f()
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("no luck after %d attempts: last error: %v", attempts, err)
}
