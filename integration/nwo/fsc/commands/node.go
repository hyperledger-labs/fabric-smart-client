/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type NodeStart struct {
	NodeID string
}

func (n NodeStart) SessionName() string {
	return n.NodeID
}

func (NodeStart) Args() []string {
	return []string{
		"node", "start",
	}
}
