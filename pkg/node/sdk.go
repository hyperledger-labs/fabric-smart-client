/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
)

// SDK models an abstract kit that can be installed and started.
type SDK interface {
	// Install signals the SDK that it is time to install itself.
	Install() error

	// Start signals the SDK that it is time to start any computation/server this SDK offers.
	Start(ctx context.Context) error
}
