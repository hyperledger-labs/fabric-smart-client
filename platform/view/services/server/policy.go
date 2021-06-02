/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package server

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"

type YesPolicyChecker struct {
}

func (y YesPolicyChecker) Check(sc *protos.SignedCommand, c *protos.Command) error {
	return nil
}
