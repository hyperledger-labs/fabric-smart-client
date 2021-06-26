/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/pkg/errors"
)

type contractMetaHandler struct{}

func (s2 *contractMetaHandler) StoreMeta(ns *Namespace, s interface{}, namespace string, key string, options *addOutputOptions) error {
	if len(options.contract) == 0 {
		return nil
	}

	// update meta
	rws, err := ns.RWSet()
	if err != nil {
		return errors.Wrap(err, "filed getting rw set")
	}

	meta, err := rws.GetStateMetadata(namespace, key, fabric.FromIntermediate)
	if err != nil {
		return errors.Wrap(err, "filed getting metadata")
	}
	if len(meta) == 0 {
		meta = map[string][]byte{}
	}
	meta["contract"] = []byte(options.contract)
	err = rws.SetStateMetadata(namespace, key, meta)
	if err != nil {
		return errors.Wrap(err, "failed setting contract")
	}
	return nil
}
