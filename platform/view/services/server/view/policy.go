/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
)

type YesPolicyChecker struct {
}

func (y YesPolicyChecker) Check(sc *protos2.SignedCommand, c *protos2.Command) error {
	return nil
}
