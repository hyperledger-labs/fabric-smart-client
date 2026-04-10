/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"
)

func TestOrganization(t *testing.T) {
	t.Parallel()
	_ = Org(&OrganizationConfig{})
}
