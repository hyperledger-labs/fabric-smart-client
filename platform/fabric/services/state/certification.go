/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	CertificationType string = "CertificationType"
	Certification     string = "Certification"

	ChaincodeCertification string = "ChaincodesCertification"

	CertificationFnc string = "state_certification"
)

type TxTransientStore interface {
	SetTransient(key string, raw []byte) error
	GetTransient(key string) []byte
}

func SetCertificationType(tx TxTransientStore, typ string, value []byte) error {
	switch typ {
	case ChaincodeCertification:
		if err := tx.SetTransient(CertificationType, []byte(ChaincodeCertification)); err != nil {
			return errors.Wrap(err, "failed appending certification type")
		}
		return nil
	default:
		return errors.Errorf("certification type [%s] not recognized", typ)
	}
}

func GetCertificationType(tx TxTransientStore) (string, []byte, error) {
	ctt := tx.GetTransient(CertificationType)
	if len(ctt) == 0 {
		return "", nil, nil
	}

	typ := string(ctt)
	switch typ {
	case ChaincodeCertification:
		return typ, nil, nil
	default:
		return "", nil, errors.Errorf("certification type [%s] not recognized", typ)
	}
}

func SetCertification(tx TxTransientStore, id string, value []byte) error {
	k, err := certificationKey(id)
	if err != nil {
		return errors.Wrap(err, "failed creating certification composite key")
	}
	if err := tx.SetTransient(k, value); err != nil {
		return errors.Wrap(err, "failed appending certification type")
	}

	return nil
}

func GetCertification(tx TxTransientStore, id string) ([]byte, error) {
	k, err := certificationKey(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating certification composite key")
	}
	t := tx.GetTransient(k)
	if len(t) == 0 {
		return nil, errors.Wrap(err, "no certification found")
	}

	return t, nil
}

func certificationKey(key string) (string, error) {
	prefix, attrs, err := rwset.SplitCompositeKey(key)
	if err != nil {
		return "", err
	}
	elems := append([]string{prefix}, attrs...)
	return rwset.CreateCompositeKey(Certification, elems)
}

func (n *Namespace) VerifyInputCertificationAt(index int, key string) error {
	typ, _, err := GetCertificationType(n.tx)
	if err != nil {
		return errors.Wrapf(err, "failed getting certification type")
	}
	if len(typ) == 0 {
		return errors.Wrapf(err, "no certification type found")
	}
	switch typ {
	case ChaincodeCertification:
		rwSet, err := n.tx.RWSet()
		if err != nil {
			return errors.Wrap(err, "filed getting rw set")
		}
		id, err := rwSet.GetReadKeyAt(n.namespace(), index)
		if err != nil {
			return errors.Wrapf(err, "failed getting state [%s, %d]", n.namespace(), index)
		}

		raw, err := GetCertification(n.tx, id)
		if err != nil {
			return errors.Wrapf(err, "failed setting certification from [%s, %d]", n.namespace(), index)
		}

		// raw is an envelope, it must be signed by enough endorsers
		cn, cv := n.tx.Chaincode()
		ch, err := fabric.GetFabricNetworkService(n.tx.ServiceProvider, n.tx.Network()).Channel(n.tx.Channel())
		if err != nil {
			return errors.Wrapf(err, "failed getting channel [%s:%s]", n.tx.Network(), n.tx.Channel())
		}
		endorsers, err := ch.Chaincode(cn).Discover().Call()
		if err != nil {
			return errors.Wrapf(err, "failed asking endorsers for to [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}
		_, certTx, err := endorser.NewTransactionFromEnvelopeBytes(n.tx.ServiceProvider, raw)
		if err != nil {
			return errors.Wrapf(err, "failed parsing certification [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}

		// Check input
		fn, params := certTx.FunctionAndParameters()
		if fn != CertificationFnc || len(params) != 2 || params[0] != key || params[1] != n.tx.ID() {
			return errors.Errorf("invalid certification, expected [CertificationFnc,%s,%s], got [%s,%v]", n.tx.ID(), key, fn, params)
		}

		// Check endorsements
		if err := certTx.HasBeenEndorsedBy(fabric.DiscoveredIdentities(endorsers)...); err != nil {
			return errors.Wrapf(err, "failed validating certification [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}

		// Extract the content
		rws, err := certTx.RWSet()
		if err != nil {
			return errors.Wrapf(err, "failed getting rws [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}
		defer certTx.Close()
		k, v, err := rws.GetWriteAt(n.namespace(), 0)
		if err != nil {
			return errors.Wrapf(err, "failed getting rws write at 0 [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}
		if k != key {
			return errors.Errorf("invalid certification, expected key [%s], got [%s]", key, k)
		}
		n.certifiedInputs[k] = v

		return nil
	default:
		return errors.Errorf("certification type [%s] not recognized", typ)
	}
}

func (n *Namespace) certifyInput(id string) error {
	typ, _, err := GetCertificationType(n.tx)
	if err != nil {
		return errors.Wrapf(err, "failed getting certification type")
	}
	if len(typ) == 0 {
		return errors.Wrapf(err, "no certification type found")
	}
	switch typ {
	case ChaincodeCertification:
		// Invoke chaincode
		cn, cv := n.tx.Chaincode()
		ch, err := fabric.GetFabricNetworkService(n.tx.ServiceProvider, n.tx.Network()).Channel(n.tx.Channel())
		if err != nil {
			return errors.Wrapf(err, "failed getting channel [%s:%s]", n.tx.Network(), n.tx.Channel())
		}
		env, err := ch.Chaincode(cn).Endorse(CertificationFnc, id, n.tx.ID()).WithInvokerIdentity(
			fabric.GetFabricNetworkService(n.tx.ServiceProvider, n.tx.Network()).IdentityProvider().DefaultIdentity(),
		).Call()
		if err != nil {
			return errors.Wrapf(err, "failed asking certification to [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}
		rawEnv, err := env.Bytes()
		if err != nil {
			return errors.Wrapf(err, "failed marshalling tx env [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}
		if err := SetCertification(n.tx, id, rawEnv); err != nil {
			return errors.Wrapf(err, "failed setting certification from [%s,%s,%s] of [%s]", n.tx.Channel(), cn, cv, id)
		}
		return nil
	default:
		return errors.Errorf("certification type [%s] not recognized", typ)
	}
}

type CertificationRequest struct {
	Channel   string
	Namespace string
	Key       string
}

type CertificationView struct {
	*CertificationRequest
}

func (c *CertificationView) Call(context view.Context) (interface{}, error) {
	vault := GetVaultForChannel(context, c.Channel)
	cert, err := vault.GetStateCertification(c.Namespace, c.Key)
	assert.NoError(err, "failed getting certification")

	return cert, nil
}

type CertificationViewFactory struct{}

func (c *CertificationViewFactory) NewView(in []byte) (view.View, error) {
	f := &CertificationView{CertificationRequest: &CertificationRequest{}}
	err := json.Unmarshal(in, f.CertificationRequest)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
