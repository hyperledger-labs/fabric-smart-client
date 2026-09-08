/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// YesPolicyChecker is a policy checker that always returns no error.
type YesPolicyChecker struct{}

// Check checks if the given command is authorized. It always returns no error.
func (YesPolicyChecker) Check(_ *protos2.SignedCommand, _ *protos2.Command) error {
	return nil
}
