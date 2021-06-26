/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package handlers

import (
	"encoding/base64"
	"encoding/json"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
)

type keyStoreEntry struct {
	Type string
	Raw  []byte
}

type store struct {
	sp   view2.ServiceProvider
	user User
}

type nymSecretKeyEntry struct {
	Sk []byte
	Pk []byte
}

func NewStore(sp view2.ServiceProvider, user User) *store {
	return &store{sp: sp, user: user}
}

func (s *store) ReadOnly() bool {
	return false
}

func (s *store) GetKey(ski []byte) (k bccsp.Key, err error) {
	ck := kvs.CreateCompositeKeyOrPanic(
		"fabric-sdk.csp.idemix.kvsstore",
		[]string{
			base64.StdEncoding.EncodeToString(ski),
		},
	)
	kvss := kvs.GetService(s.sp)
	if !kvss.Exists(ck) {
		return nil, errors.Errorf("key with ski [%x] not found", ski)
	}
	kse := &keyStoreEntry{}
	if err := kvss.Get(ck, &kse); err != nil {
		return nil, err
	}
	switch kse.Type {
	case "userSecretKey":
		sk, err := s.user.NewKeyFromBytes(kse.Raw)
		if err != nil {
			return nil, err
		}
		return NewUserSecretKey(sk, false), nil
	case "nymSecretKey":
		e := &nymSecretKeyEntry{}
		if err := json.Unmarshal(kse.Raw, e); err != nil {
			return nil, err
		}

		sk, err := s.user.NewKeyFromBytes(e.Sk)
		if err != nil {
			return nil, err
		}
		pk, err := s.user.NewPublicNymFromBytes(e.Pk)
		if err != nil {
			return nil, err
		}

		return NewNymSecretKey(sk, pk, false)
	}
	return nil, errors.Errorf("key type [%s] not recognized", kse.Type)
}

func (s *store) StoreKey(k bccsp.Key) error {
	kvss := kvs.GetService(s.sp)

	// marshal key
	var kType string
	var raw []byte
	var err error
	var ski []byte

	switch kk := k.(type) {
	case *userSecretKey:
		kType = "userSecretKey"
		ski = k.SKI()
		raw, err = k.Bytes()
		if err != nil {
			return err
		}
	case *nymSecretKey:
		kType = "nymSecretKey"
		skRaw, err := kk.sk.Bytes()
		if err != nil {
			return err
		}
		pk, err := k.PublicKey()
		if err != nil {
			return err
		}
		pkRaw, err := pk.Bytes()
		if err != nil {
			return err
		}
		ski = pk.SKI()
		raw, err = json.Marshal(&nymSecretKeyEntry{
			Sk: skRaw,
			Pk: pkRaw,
		})
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("key type not recognized")
	}
	// store entry
	ck := kvs.CreateCompositeKeyOrPanic(
		"fabric-sdk.csp.idemix.kvsstore",
		[]string{
			base64.StdEncoding.EncodeToString(ski),
		},
	)
	if err := kvss.Put(ck, &keyStoreEntry{
		Type: kType,
		Raw:  raw,
	}); err != nil {
		return err
	}
	return nil
}
