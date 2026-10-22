/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import "context"

// listen serves the manager over a single notification stream and stops it once that
// stream fails or ctx ends, settling every pending listener. It returns the stream's
// error, or ctx's. Tests use it to drive one stream's lifecycle without the reconnect
// loop that run adds.
func (n *notificationListenerManager) listen(ctx context.Context) error {
	return n.serve(ctx, n.stream)
}
