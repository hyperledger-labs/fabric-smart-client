/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ECDSASignParams struct {
}

type ECDSASignView struct {
	params ECDSASignParams

	pr *ecdsa.PrivateKey
	r  io.Reader
}

var msgBytes = []byte("hello, world")

func (q *ECDSASignView) Call(viewCtx view.Context) (interface{}, error) {
	hash := sha256.Sum256(msgBytes)

	sig, err := ecdsa.SignASN1(q.r, q.pr, hash[:])
	if err != nil {
		return "error", err
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

type ECDSASignViewFactory struct{}

func (c *ECDSASignViewFactory) NewView(in []byte) (view.View, error) {

	f := &ECDSASignView{}
	if err := json.Unmarshal(in, &f.params); err != nil {
		return nil, err
	}

	// we only want to set up these once
	once.Do(func() {
		// setup signing key
		r = rand.Reader
		pk, err := ecdsa.GenerateKey(elliptic.P256(), r)
		if err != nil {
			panic(err)
		}
		pr = pk
	})

	f.r = r
	f.pr = pr

	return f, nil
}

var (
	once sync.Once
	pr   *ecdsa.PrivateKey
	r    io.Reader
)
