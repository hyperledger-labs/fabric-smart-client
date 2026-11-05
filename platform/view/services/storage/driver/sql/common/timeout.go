/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// minQueryTimeout is the shortest accepted queryTimeout. It catches a unit-less value:
// the decoder reads `queryTimeout: 30` as 30ns, which would fail every statement.
const minQueryTimeout = 100 * time.Millisecond

// QueryTimeout bounds how long one database statement, or one write transaction, may
// run. Zero adds no deadline.
type QueryTimeout time.Duration

// Context returns ctx with q's deadline, and the function that releases it. A deadline
// ctx already has that expires earlier still applies. With q zero it returns ctx
// unchanged.
func (q QueryTimeout) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	if q <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(q))
}

// Validate returns an error unless q is zero or at least 100ms.
func (q QueryTimeout) Validate() error {
	if q != 0 && time.Duration(q) < minQueryTimeout {
		return errors.Errorf("queryTimeout [%s] must be 0 (off) or at least %s, written with a unit such as 30s",
			time.Duration(q), minQueryTimeout)
	}
	return nil
}
