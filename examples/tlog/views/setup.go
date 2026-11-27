/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"

	"golang.org/x/mod/sumdb/note"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Setup is the input of the init view: this node's log key and what it needs
// to know about the other two log nodes.
type Setup struct {
	// LogKey is this node's ECDSA P-256 log key, PKCS#8 DER.
	LogKey []byte
	// Verifiers are the public log keys of all three orgs, PKIX DER, in
	// Org1..Org3 order.
	Verifiers [][]byte
	// Peers are the other two log nodes.
	Peers []view.Identity
}

// InitView loads a Setup into this node's LogService.
//
// ponytail: the keys live in memory and a restart loses them; load them from
// the node's config if restarts matter.
type InitView struct {
	Setup
}

func (i *InitView) Call(viewCtx view.Context) (any, error) {
	key, err := x509.ParsePKCS8PrivateKey(i.LogKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed parsing the log key")
	}
	priv, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.Errorf("log key is a [%T], not an ECDSA key", key)
	}
	signer, err := translog.NewSigner(priv)
	if err != nil {
		return nil, err
	}
	verifiers := make([]note.Verifier, len(i.Verifiers))
	for j, der := range i.Verifiers {
		pub, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return nil, errors.Wrapf(err, "failed parsing verifier %d", j)
		}
		ecPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.Errorf("verifier %d is a [%T], not an ECDSA key", j, pub)
		}
		if verifiers[j], err = translog.NewVerifier(ecPub); err != nil {
			return nil, err
		}
	}

	ls, err := logServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}
	ls.setup(signer, verifiers, i.Peers)
	return nil, nil
}

// InitViewFactory creates an InitView from a JSON Setup.
type InitViewFactory struct{}

func (*InitViewFactory) NewView(in []byte) (view.View, error) {
	f := &InitView{}
	if err := json.Unmarshal(in, &f.Setup); err != nil {
		return nil, errors.Wrap(err, "failed decoding the setup")
	}
	return f, nil
}
