/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package workload

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
)

var Workloads = []node.Workload{
	{
		Name:    "noop",
		Factory: &views.NoopViewFactory{},
	},
	{
		Name:    "cpu",
		Factory: &views.CPUViewFactory{},
		Params:  &views.CPUParams{N: 200000},
	},
	{
		Name:    "sign",
		Factory: &views.ECDSASignViewFactory{},
		Params:  &views.ECDSASignParams{},
	},
}
