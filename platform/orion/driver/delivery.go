/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "context"

// DeliveryService models the delivery service
type DeliveryService interface {
	// StartDelivery starts the delivery
	StartDelivery(context.Context) error
	// Stop stops delivery
	Stop()
}
