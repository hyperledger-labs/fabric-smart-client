/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type Relay struct {
	fns *fabric.NetworkService
}

func (r *Relay) RequestState() error {
	address := r.fns.ConfigService().GetString("weaver.relay.address")

	names := fabric.GetFabricNetworkNames(r.fns.SP)
	fmt.Println(">>>>", names)

	fmt.Println("^^^^^", r.fns.Name())

	var otherName string
	for _, name := range names {
		if r.fns.Name() == name {
			continue
		}
		otherName = name
		break
	}

	otherNS := fabric.GetFabricNetworkService(r.fns.SP, otherName)

	otherAddress := otherNS.ConfigService().GetString("weaver.relay.address")

	fmt.Println("$$$$$$$", address, otherAddress)

	panic("bla")

	return nil
}
