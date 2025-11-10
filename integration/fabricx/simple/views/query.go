/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type QueryParams struct {
	SomeIDs   []string
	Namespace string
}

type QueryView struct {
	params QueryParams
}

func (q *QueryView) Call(viewCtx view.Context) (interface{}, error) {
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, err
	}

	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	if err != nil {
		return nil, err
	}

	ns := q.params.Namespace
	keys := q.params.SomeIDs

	// single shot
	objs1, err := getObjectsViaSingleGet(qs, ns, keys...)
	if err != nil {
		return nil, err
	}

	// multiget
	objs2, err := getObjectsViaMultiGet(qs, ns, keys...)
	if err != nil {
		return nil, err
	}

	// note this check only works if there is no activity on the ledger as all objs may reference different ledger heights
	if !reflect.DeepEqual(objs1, objs2) {
		return nil, errors.Errorf("Got different values a=%v b=%v", objs1, objs2)
	}

	return objs1, nil
}

func getObjectsViaSingleGet(qs queryservice.QueryService, ns driver.Namespace, keys ...driver.PKey) ([]*SomeObject, error) {
	objs := make([]*SomeObject, 0, len(keys))

	for _, id := range keys {
		val, err := qs.GetState(ns, id)
		if err != nil {
			return nil, err
		}

		// there is no value for this key
		if val == nil {
			objs = append(objs, nil)
			continue
		}

		obj, err := Unmarshal(val.Raw)
		if err != nil {
			return nil, err
		}

		objs = append(objs, obj)
	}

	return objs, nil
}

func getObjectsViaMultiGet(qs queryservice.QueryService, ns driver.Namespace, keys ...driver.PKey) ([]*SomeObject, error) {
	objs := make([]*SomeObject, len(keys))

	input := make(map[driver.Namespace][]driver.PKey)
	input[ns] = keys

	output, err := qs.GetStates(input)
	if err != nil {
		return nil, err
	}

	for i, id := range keys {
		val, ok := output[ns][id]
		if !ok {
			objs[i] = nil
			continue
		}

		obj, err := Unmarshal(val.Raw)
		if err != nil {
			return nil, err
		}

		objs[i] = obj
	}

	return objs, nil
}

type QueryViewFactory struct{}

func (c *QueryViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryView{}
	if err := json.Unmarshal(in, &f.params); err != nil {
		return nil, err
	}

	return f, nil
}
