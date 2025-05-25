/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
)

type SDK interface {
	Install(ctx context.Context) error

	Start(ctx context.Context) error
}
