/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	CertificationType      string = "CertificationType"
	Certification          string = "Certification"
	ChaincodeCertification string = "ChaincodesCertification"
	CertificationFnc       string = "state_certification"
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

// Certifier verifies and (on the default path) produces certification for input
// states. The concrete implementation is resolved per node from the registered
// Certifier service: ChaincodeCertifier (the default) verifies chaincode
// endorsements; TrustedReadCertifier trusts the vault's committed read.
type Certifier interface {
	CertifyInput(n *Namespace, id string) error
	VerifyInputCertificationAt(n *Namespace, index int, key string) error
}

// resolveCertifier returns the Certifier registered on the transaction's service
// provider, falling back to ChaincodeCertifier (the Fabric default) when none is
// registered. Dispatch is by injected certifier, not by the stored type string.
func (n *Namespace) resolveCertifier() Certifier {
	// n.tx.Provider may be nil in unit tests that exercise pure type-checking paths;
	// guard so resolution degrades to the default rather than panicking.
	if n.tx.Provider != nil {
		if s, err := n.tx.GetService(reflect.TypeFor[Certifier]()); err == nil {
			if c, ok := s.(Certifier); ok {
				return c
			}
		}
	}
	return &ChaincodeCertifier{}
}

func (n *Namespace) VerifyInputCertificationAt(index int, key string) error {
	return n.resolveCertifier().VerifyInputCertificationAt(n, index, key)
}

func (n *Namespace) certifyInput(id string) error {
	return n.resolveCertifier().CertifyInput(n, id)
}

// ChaincodeCertifier is the default (Fabric) certifier. It certifies input states
// by invoking the generic state query chaincode and verifying peer endorsements.
type ChaincodeCertifier struct{}

func (c *ChaincodeCertifier) VerifyInputCertificationAt(n *Namespace, index int, key string) error {
	typ, _, err := GetCertificationType(n.tx)
	if err != nil {
		return errors.Wrapf(err, "failed getting certification type")
	}
	if len(typ) == 0 {
		return errors.Errorf("no certification type found")
	}
	switch typ {
	case ChaincodeCertification:
		rwSet, err := n.tx.RWSet()
		if err != nil {
			return errors.Wrap(err, "failed getting rw set")
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
		_, ch, err := fabric.GetChannel(n.tx.Provider, n.tx.Network(), n.tx.Channel())
		if err != nil {
			return errors.Wrapf(err, "failed getting channel [%s:%s]", n.tx.Network(), n.tx.Channel())
		}
		endorsers, err := ch.Chaincode(cn).Discover().Call()
		if err != nil {
			return errors.Wrapf(err, "failed asking endorsers for to [%s,%s,%s] for [%s]", n.tx.Channel(), cn, cv, id)
		}
		_, certTx, err := endorser.NewTransactionFromEnvelopeBytes(context.Background(), n.tx.Provider, raw)
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

func (c *ChaincodeCertifier) CertifyInput(n *Namespace, id string) error {
	typ, _, err := GetCertificationType(n.tx)
	if err != nil {
		return errors.Wrapf(err, "failed getting certification type")
	}
	if len(typ) == 0 {
		return errors.Errorf("no certification type found")
	}
	switch typ {
	case ChaincodeCertification:
		// Invoke chaincode
		cn, cv := n.tx.Chaincode()
		fns, ch, err := fabric.GetChannel(n.tx.Provider, n.tx.Network(), n.tx.Channel())
		if err != nil {
			return errors.Wrapf(err, "failed getting channel [%s:%s]", n.tx.Network(), n.tx.Channel())
		}
		env, err := ch.Chaincode(cn).Endorse(CertificationFnc, id, n.tx.ID()).WithInvokerIdentity(
			fns.IdentityProvider().DefaultIdentity(),
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

// TrustedReadCertifier certifies inputs by trusting the vault's committed read
// instead of verifying chaincode endorsements. It suits nodes whose vault reads are
// already authoritative, so no per-input endorsement proof is needed. It is opt-in:
// a platform registers it as the Certifier service (the default stays
// ChaincodeCertifier). CertifyInput is a no-op — every node self-reads the committed
// value via GetReadAt, so certifiedInputs stays empty and nothing consumes a produced
// cert. Verification re-reads the committed value and branches on the hiding mode.
type TrustedReadCertifier struct{}

func (c *TrustedReadCertifier) CertifyInput(n *Namespace, id string) error {
	return nil
}

func (c *TrustedReadCertifier) VerifyInputCertificationAt(n *Namespace, index int, key string) error {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		return errors.Wrap(err, "failed getting rw set")
	}
	_, committed, err := rwSet.GetReadAt(n.namespace(), index)
	if err != nil {
		return errors.Wrapf(err, "failed reading committed state [%s, %d]", n.namespace(), index)
	}
	if len(committed) == 0 {
		return errors.Errorf("no committed value for [%s, %s]", n.namespace(), key)
	}

	mapping, err := n.getFieldMapping(n.namespace(), key, true)
	if err != nil {
		return errors.Wrapf(err, "failed getting field mapping [%s, %s]", n.namespace(), key)
	}

	if root, ok := mapping["_root_"]; ok && len(root) != 0 {
		// Whole-state hiding: the committed value is sha256(preimage).
		h := sha256.Sum256(root)
		if !bytes.Equal(h[:], committed) {
			return errors.Errorf("hash mismatch for [%s, %s]", n.namespace(), key)
		}
		return nil
	}

	// Field-level hiding: the committed value IS the marshaled state (the hidden field is
	// already hashed in place). Integrity of []byte hash fields is enforced by unmarshalTags
	// during State() (recompute + compare against the committed hash; a missing or tampered
	// preimage errors there). String hash fields are rejected at (un)marshal time, so no
	// unverified hiding mode reaches this point. Here we only confirm the state is committed
	// (checked above).
	return nil
}

type CertificationRequest struct {
	Channel   string
	Namespace string
	Key       string
}

type CertificationView struct {
	*CertificationRequest
}

func (c *CertificationView) Call(viewCtx view.Context) (any, error) {
	vault, err := GetVaultForChannel(viewCtx, c.Channel)
	if err != nil {
		return nil, err
	}
	cert, err := vault.GetStateCertification(viewCtx.Context(), c.Namespace, c.Key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting certification")
	}

	return cert, nil
}

type CertificationViewFactory struct{}

func (c *CertificationViewFactory) NewView(in []byte) (view.View, error) {
	f := &CertificationView{CertificationRequest: &CertificationRequest{}}
	err := json.Unmarshal(in, f.CertificationRequest)
	if err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling input")
	}
	return f, nil
}
