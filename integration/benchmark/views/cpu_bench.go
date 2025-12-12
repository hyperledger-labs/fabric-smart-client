/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CPUParams struct {
	N int
}

type CPUView struct {
	params CPUParams
}

func (q *CPUView) Call(viewCtx view.Context) (interface{}, error) {
	k := doWork(q.params.N)
	_ = k
	return "OK", nil
}

// doWork just burns CPU cycles
func doWork(n int) uint64 {
	var x uint64
	for i := 0; i < n; i++ {
		x += uint64(i * i)
	}
	return x
}

type CPUViewFactory struct{}

func (c *CPUViewFactory) NewView(in []byte) (view.View, error) {

	f := &CPUView{}
	if err := json.Unmarshal(in, &f.params); err != nil {
		return nil, err
	}

	return f, nil
}
