/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

type fscOrg struct {
	c *Topology
	o *Organization
}

func (fo *fscOrg) AddPeer(name string) *fscOrg {
	fo.c.AddPeer(name, fo.o.ID, FabricPeer, false, "")

	return fo
}
