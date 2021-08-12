/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"encoding/json"
	"strings"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type EnclaveRegistry struct {
	fns *fabric.NetworkService
	ch  *fabric.Channel
}

func (e *EnclaveRegistry) ListProvisionedEnclaves(cid string) ([]string, error) {
	raw, err := e.ch.Chaincode("ercc").Query(
		"QueryListProvisionedEnclaves", cid,
	).WithInvokerIdentity(
		e.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return nil, err
	}

	if len(raw) == 0 {
		return nil, nil
	}

	var res []string
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling [%s:%v]", string(raw), raw)
	}

	return res, nil
}

func (e *EnclaveRegistry) ChaincodeEncryptionKey(cid string) ([]byte, error) {
	raw, err := e.ch.Chaincode("ercc").Query(
		"QueryChaincodeEncryptionKey", cid,
	).WithInvokerIdentity(
		e.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return nil, err
	}

	return raw, nil
}

func (e *EnclaveRegistry) PeerEndpoints(cid string) ([]string, error) {
	raw, err := e.ch.Chaincode("ercc").Query(
		"QueryChaincodeEndPoints", cid,
	).WithInvokerIdentity(
		e.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return nil, err
	}

	return strings.Split(string(raw), ","), nil
}
