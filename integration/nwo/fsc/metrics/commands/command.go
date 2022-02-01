/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type AggregatorStart struct {
	NodeID string
}

func (n AggregatorStart) SessionName() string {
	return n.NodeID
}

func (n AggregatorStart) Args() []string {
	return []string{
		"aggregator",
	}
}
