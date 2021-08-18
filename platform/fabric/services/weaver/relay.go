/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fabric3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver/relay/fabric"
)

type Relay struct {
	fns *fabric.NetworkService
}

func (r *Relay) FabricQuery(destination, function string, args ...interface{}) (*fabric3.Query, error) {
	id, err := fabric3.URLToID(destination)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing destination [%s]", destination)
	}

	return fabric3.NewQuery(r.fns, id, function, args), nil
}
