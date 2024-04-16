/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"encoding/binary"
	"math"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

//go:generate  protoc -I=. --go_out=. ftxid.proto

const (
	txidNamespace = "txid"
	ctrKey        = "ctr"
	byCtrPrefix   = "C"
	byTxidPrefix  = "T"
	lastTX        = "last"
)

type SimpleTXIDStore[V vault.ValidationCode] struct {
	persistence driver.Persistence
	ctr         uint64
	vcProvider  vault.ValidationCodeProvider[V]
}

func NewSimpleTXIDStore[V vault.ValidationCode](persistence driver.Persistence, vcProvider vault.ValidationCodeProvider[V]) (*SimpleTXIDStore[V], error) {
	ctrBytes, err := persistence.GetState(txidNamespace, ctrKey)
	if err != nil {
		return nil, errors.Errorf("error retrieving txid counter [%s]", err.Error())
	}

	if ctrBytes == nil {
		if err = persistence.BeginUpdate(); err != nil {
			return nil, errors.Errorf("error starting update to store counter [%s]", err.Error())
		}

		err = setCtr(persistence, 0)
		if err != nil {
			persistence.Discard()
			return nil, err
		}

		if err = persistence.Commit(); err != nil {
			return nil, errors.Errorf("error committing update to store counter [%s]", err.Error())
		}

		ctrBytes = make([]byte, binary.MaxVarintLen64)
	}

	return &SimpleTXIDStore[V]{
		persistence: persistence,
		ctr:         getCtrFromBytes(ctrBytes),
		vcProvider:  vcProvider,
	}, nil
}

func (s *SimpleTXIDStore[V]) get(txID core.TxID) (*ByTxid, error) {
	bytes, err := s.persistence.GetState(txidNamespace, keyByTxID(txID))
	if err != nil {
		return nil, errors.Errorf("error retrieving txid %s [%s]", txID, err.Error())
	}

	if len(bytes) == 0 {
		return nil, nil
	}

	bt := &ByTxid{}
	err = proto.Unmarshal(bytes, bt)
	if err != nil {
		return nil, errors.Errorf("error unmarshalling data for txid %s [%s]", txID, err.Error())
	}

	return bt, nil
}

func (s *SimpleTXIDStore[V]) Get(txID core.TxID) (V, string, error) {
	bt, err := s.get(txID)
	if err != nil {
		return s.vcProvider.Unknown(), "", err
	}

	if bt == nil {
		return s.vcProvider.Unknown(), "", nil
	}

	return s.vcProvider.FromInt32(bt.Code), bt.Message, nil
}

func (s *SimpleTXIDStore[V]) Set(txID core.TxID, code V, message string) error {
	// NOTE: we assume that the commit is in progress so no need to update/commit
	// err := s.persistence.BeginUpdate()
	// if err != nil {
	// 	return errors.Errorf("error starting update to set txid %s [%s]", txid, err.Error())
	// }

	// 1: increment ctr in persistence
	err := setCtr(s.persistence, s.ctr+1)
	if err != nil { // TODO: && !errors2.HasCause(err, driver.UniqueKeyViolation)
		s.persistence.Discard()
		return errors.Errorf("error storing updated counter for txid %s [%s]", txID, err.Error())
	}

	// 2: store by counter
	byCtrBytes, err := proto.Marshal(&ByNum{
		Txid:    txID,
		Code:    s.vcProvider.ToInt32(code),
		Message: message,
	})
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error marshalling ByNum for txID %s [%s]", txID, err.Error())
	}
	err = s.persistence.SetState(txidNamespace, keyByCtr(s.ctr), byCtrBytes)
	if err != nil { // TODO: && !errors2.HasCause(err, driver.UniqueKeyViolation)
		s.persistence.Discard()
		return errors.Errorf("error storing ByNum for txid %s [%s]", txID, err.Error())
	}

	// 3: store by txid
	byTxidBytes, err := proto.Marshal(&ByTxid{
		Pos:     s.ctr,
		Code:    s.vcProvider.ToInt32(code),
		Message: message,
	})
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error marshalling ByTxid for txid %s [%s]", txID, err.Error())
	}
	err = s.persistence.SetState(txidNamespace, keyByTxID(txID), byTxidBytes)
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error storing ByTxid for txid %s [%s]", txID, err.Error())
	}

	if s.vcProvider.IsValid(code) {
		err = s.persistence.SetState(txidNamespace, lastTX, []byte(txID))
		if err != nil { // TODO: && !errors2.HasCause(err, driver.UniqueKeyViolation)
			s.persistence.Discard()
			return errors.Errorf("error storing ByTxid for txid %s [%s]", txID, err.Error())
		}
	}
	// NOTE: we assume that the commit is in progress so no need to update/commit
	// err = s.persistence.Commit()
	// if err != nil {
	// 	return errors.Errorf("error committing update to set txid %s [%s]", txid, err.Error())
	// }

	s.ctr++

	return nil
}

func (s *SimpleTXIDStore[V]) GetLastTxID() (core.TxID, error) {
	v, err := s.persistence.GetState(txidNamespace, lastTX)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get last TxID")
	}
	if len(v) == 0 {
		return "", nil
	}
	return string(v), nil
}

func (s *SimpleTXIDStore[V]) Iterator(pos interface{}) (vault.TxIDIterator[V], error) {
	var iterator utils.Iterator[*ByNum]
	if ppos, ok := pos.(*vault.SeekSet); ok {
		it, err := s.persistence.GetStateSetIterator(txidNamespace, ppos.TxIDs...)
		if err != nil {
			return nil, err
		}
		iterator = &SimpleTxIDIterator{it}

	} else {
		startKey, err := s.getStartKey(pos)
		if err != nil {
			return nil, err
		}

		it, err := s.persistence.GetStateRangeScanIterator(txidNamespace, keyByCtr(startKey), keyByCtr(math.MaxUint64))
		if err != nil {
			return nil, err
		}
		iterator = &SimpleTxIDIterator{it}
	}

	return utils.Map(iterator, s.mapByNum), nil
}

func (s *SimpleTXIDStore[V]) getStartKey(pos interface{}) (uint64, error) {
	switch ppos := pos.(type) {
	case *vault.SeekStart:
		return 0, nil
	case *vault.SeekEnd:
		ctr, err := getCtr(s.persistence)
		if err != nil {
			return 0, err
		}
		return ctr - 1, nil
	case *vault.SeekPos:
		bt, err := s.get(ppos.Txid)
		if err != nil {
			return 0, err
		}
		if bt == nil {
			return 0, errors.Errorf("txid %s was not found", ppos.Txid)
		}
		return bt.Pos, nil
	}
	return 0, errors.Errorf("invalid position %T", pos)
}

func (s *SimpleTXIDStore[V]) mapByNum(bn *ByNum) (*vault.ByNum[V], error) {
	return &vault.ByNum[V]{
		TxID:    bn.Txid,
		Code:    s.vcProvider.FromInt32(bn.Code),
		Message: bn.Message,
	}, nil
}

type SimpleTxIDIterator struct {
	t driver.ResultsIterator
}

func (i *SimpleTxIDIterator) Next() (*ByNum, error) {
	d, err := i.t.Next()
	if err != nil {
		return nil, err
	}

	if d == nil {
		return nil, nil
	}

	bn := &ByNum{}
	err = proto.Unmarshal(d.Raw, bn)
	if err != nil {
		return nil, err
	}
	return bn, err
}

func (i *SimpleTxIDIterator) Close() {
	i.t.Close()
}

func keyByCtr(ctr uint64) string {
	ctrBytes := new([binary.MaxVarintLen64]byte)
	binary.BigEndian.PutUint64(ctrBytes[:], ctr)

	return byCtrPrefix + string(ctrBytes[:])
}

func keyByTxID(txID core.TxID) string {
	return byTxidPrefix + txID
}

func setCtr(persistence driver.Persistence, ctr uint64) error {
	ctrBytes := make([]byte, binary.MaxVarintLen64)
	binary.BigEndian.PutUint64(ctrBytes, ctr)

	err := persistence.SetState(txidNamespace, ctrKey, ctrBytes)
	if err != nil && !errors2.HasCause(err, driver.UniqueKeyViolation) {
		return errors.Errorf("error storing the counter [%s]", err.Error())
	}

	return nil
}

func getCtr(persistence driver.Persistence) (uint64, error) {
	ctrBytes, err := persistence.GetState(txidNamespace, ctrKey)
	if err != nil {
		return 0, errors.Errorf("error retrieving txid counter [%s]", err.Error())
	}

	return getCtrFromBytes(ctrBytes), nil
}

func getCtrFromBytes(ctrBytes []byte) uint64 {
	return binary.BigEndian.Uint64(ctrBytes)
}
