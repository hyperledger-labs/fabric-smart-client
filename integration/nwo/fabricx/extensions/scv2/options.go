/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"

func WithApproverRole() node.Option {
	return func(o *node.Options) error {
		o.Put("approver.role", "fabricx")
		return nil
	}
}

func IsApprover(options *node.Options) bool {
	o := options.Get("approver.role")
	return o != nil && o != ""
}
