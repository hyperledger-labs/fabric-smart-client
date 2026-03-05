/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

// MutableParentContext models a parent context that is also mutable.
//
//go:generate counterfeiter -o mock/mutable_parent_context.go -fake-name MutableParentContext . MutableParentContext
type MutableParentContext interface {
	ParentContext
	view.MutableContext
}
