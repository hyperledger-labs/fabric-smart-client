/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"encoding/binary"
	"math"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
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

type (
	UnversionedPersistence     = dbdriver.UnversionedPersistence
	UnversionedResultsIterator = dbdriver.UnversionedResultsIterator
)

var (
	UniqueKeyViolation = dbdriver.UniqueKeyViolation
	logger             = flogging.MustGetLogger("simple-txid-store")
)

type SimpleTXIDStore[V driver.ValidationCode] struct {
	Persistence UnversionedPersistence
	ctr         uint64
	vcProvider  driver.ValidationCodeProvider[V]
}

func NewSimpleTXIDStore[V driver.ValidationCode](persistence UnversionedPersistence, vcProvider driver.ValidationCodeProvider[V]) (*SimpleTXIDStore[V], error) {
	ctrBytes, err := persistence.GetState(txidNamespace, ctrKey)
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving txid counter")
	}

	if ctrBytes == nil {
		if err = persistence.BeginUpdate(); err != nil {
			return nil, errors.Wrapf(err, "error starting update to store counter")
		}

		err = setCtr(persistence, 0)
		if err != nil {
			persistence.Discard()
			return nil, err
		}

		if err = persistence.Commit(); err != nil {
			return nil, errors.Wrapf(err, "error committing update to store counter")
		}

		ctrBytes = make([]byte, binary.MaxVarintLen64)
	}

	return &SimpleTXIDStore[V]{
		Persistence: persistence,
		ctr:         getCtrFromBytes(ctrBytes),
		vcProvider:  vcProvider,
	}, nil
}

func (s *SimpleTXIDStore[V]) get(txID driver.TxID) (*ByTxid, error) {
	bytes, err := s.Persistence.GetState(txidNamespace, keyByTxID(txID))
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving txid %s", txID)
	}

	if len(bytes) == 0 {
		return nil, nil
	}

	bt := &ByTxid{}
	err = proto.Unmarshal(bytes, bt)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling data for txid %s", txID)
	}

	return bt, nil
}

func (s *SimpleTXIDStore[V]) Get(txID driver.TxID) (V, string, error) {
	bt, err := s.get(txID)
	if err != nil {
		return s.vcProvider.Unknown(), "", err
	}

	if bt == nil {
		return s.vcProvider.Unknown(), "", nil
	}

	return s.vcProvider.FromInt32(bt.Code), bt.Message, nil
}

func (s *SimpleTXIDStore[V]) Set(txID driver.TxID, code V, message string) error {
	return s.SetMultiple([]driver.ByNum[V]{{TxID: txID, Code: code, Message: message}})
}

func (s *SimpleTXIDStore[V]) SetMultiple(txs []driver.ByNum[V]) error {
	// NOTE: we assume that the commit is in progress so no need to update/commit
	// err := s.UnversionedPersistence.BeginUpdate()
	// if err != nil {
	// 	return errors.Errorf("error starting update to set txid %s [%s]", txid, err.Error())
	// }

	states, err := s.newStoreStates(txs)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate states")
	}
	errs := s.Persistence.SetStates(txidNamespace, states)
	for _, err := range errs {
		if err != nil && !errors.HasCause(err, UniqueKeyViolation) {
			s.Persistence.Discard()
			return errors.Wrapf(err, "error updating states")
		}
	}

	// NOTE: we assume that the commit is in progress so no need to update/commit
	// err = s.UnversionedPersistence.Commit()
	// if err != nil {
	// 	return errors.Errorf("error committing update to set txid %s [%s]", txid, err.Error())
	// }

	s.ctr += uint64(len(txs))

	return nil
}

func (s *SimpleTXIDStore[V]) newStoreStates(txs []driver.ByNum[V]) (map[driver.PKey]driver.RawValue, error) {
	states := make(map[driver.PKey]driver.RawValue, 2+2*len(txs))

	logger.Debugf("Incrementing ctr value in Unversioned persistence: %d by %d", s.ctr, len(txs))

	// 1: increment ctr in UnversionedPersistence
	ctrBytes := convCtrKey(s.ctr + uint64(len(txs)))
	states[ctrKey] = ctrBytes[:]

	// 2: store by counter
	for i, tx := range txs {
		logger.Debugf("Store TX [%s] with status [%v] by counter: %d", tx.TxID, tx.Code, s.ctr)
		byCtrBytes, err := proto.Marshal(&ByNum{
			Txid:    tx.TxID,
			Code:    s.vcProvider.ToInt32(tx.Code),
			Message: tx.Message,
		})
		if err != nil {
			return nil, err
		}
		states[keyByCtr(s.ctr+uint64(i))] = byCtrBytes
	}

	// 3: store by txid
	var lastValidTxID driver.TxID
	for i, tx := range txs {
		logger.Debugf("Store TX [%s] with status [%v] by txid", tx.TxID, tx.Code)
		byTxidBytes, err := proto.Marshal(&ByTxid{
			Pos:     s.ctr + uint64(i),
			Code:    s.vcProvider.ToInt32(tx.Code),
			Message: tx.Message,
		})
		if err != nil {
			return nil, err
		}
		states[keyByTxID(tx.TxID)] = byTxidBytes

		if tx.Code == s.vcProvider.Valid() {
			lastValidTxID = tx.TxID
		}
	}

	if len(lastValidTxID) > 0 {
		states[lastTX] = []byte(lastValidTxID)
	}
	return states, nil
}

func (s *SimpleTXIDStore[V]) GetLastTxID() (driver.TxID, error) {
	v, err := s.Persistence.GetState(txidNamespace, lastTX)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get last TxID")
	}
	if len(v) == 0 {
		return "", nil
	}
	return string(v), nil
}

func (s *SimpleTXIDStore[V]) Iterator(pos interface{}) (driver.TxIDIterator[V], error) {
	var iterator collections.Iterator[*ByNum]
	if ppos, ok := pos.(*driver.SeekSet); ok {
		keys := make([]string, len(ppos.TxIDs))
		for i, txID := range ppos.TxIDs {
			keys[i] = keyByTxID(txID)
		}
		it, err := s.Persistence.GetStateSetIterator(txidNamespace, keys...)
		if err != nil {
			return nil, err
		}
		iterator = &SimpleTxIDIteratorByTxID{it}

	} else {
		startKey, err := s.getStartKey(pos)
		if err != nil {
			return nil, err
		}

		it, err := s.Persistence.GetStateRangeScanIterator(txidNamespace, keyByCtr(startKey), keyByCtr(math.MaxUint64))
		if err != nil {
			return nil, err
		}
		iterator = &SimpleTxIDIteratorByNum{it}
	}

	return collections.Map(iterator, s.mapByNum), nil
}

func (s *SimpleTXIDStore[V]) getStartKey(pos interface{}) (uint64, error) {
	switch ppos := pos.(type) {
	case *driver.SeekStart:
		return 0, nil
	case *driver.SeekEnd:
		ctr, err := getCtr(s.Persistence)
		if err != nil {
			return 0, err
		}
		return ctr - 1, nil
	case *driver.SeekPos:
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

func (s *SimpleTXIDStore[V]) mapByNum(bn *ByNum) (*driver.ByNum[V], error) {
	if bn == nil {
		return nil, nil
	}
	return &driver.ByNum[V]{
		TxID:    bn.Txid,
		Code:    s.vcProvider.FromInt32(bn.Code),
		Message: bn.Message,
	}, nil
}

type SimpleTxIDIteratorByNum struct {
	UnversionedResultsIterator
}

func (i *SimpleTxIDIteratorByNum) Next() (*ByNum, error) {
	d, err := i.UnversionedResultsIterator.Next()
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

type SimpleTxIDIteratorByTxID struct {
	UnversionedResultsIterator
}

func (i *SimpleTxIDIteratorByTxID) Next() (*ByNum, error) {
	d, err := i.UnversionedResultsIterator.Next()
	if err != nil {
		return nil, err
	}

	if d == nil {
		return nil, nil
	}

	bn := &ByTxid{}
	err = proto.Unmarshal(d.Raw, bn)
	if err != nil {
		return nil, err
	}
	return &ByNum{Txid: strings.TrimLeft(d.Key, byTxidPrefix), Code: bn.Code, Message: bn.Message}, nil
}

func keyByCtr(ctr uint64) string {
	ctrBytes := convCtrKey(ctr)
	return byCtrPrefix + string(ctrBytes[:])
}

func keyByTxID(txID driver.TxID) string {
	return byTxidPrefix + txID
}

func setCtr(persistence UnversionedPersistence, ctr uint64) error {
	ctrBytes := convCtrKey(ctr)
	err := persistence.SetState(txidNamespace, ctrKey, ctrBytes[:])
	if err != nil && !errors.HasCause(err, UniqueKeyViolation) {
		return errors.Wrapf(err, "error storing the counter")
	}

	return nil
}

func convCtrKey(ctr uint64) [binary.MaxVarintLen64]byte {
	ctrBytes := new([binary.MaxVarintLen64]byte)
	binary.BigEndian.PutUint64(ctrBytes[:], ctr)
	return *ctrBytes
}

func getCtr(persistence UnversionedPersistence) (uint64, error) {
	ctrBytes, err := persistence.GetState(txidNamespace, ctrKey)
	if err != nil {
		return 0, errors.Wrapf(err, "error retrieving txid counter")
	}

	return getCtrFromBytes(ctrBytes), nil
}

func getCtrFromBytes(ctrBytes []byte) uint64 {
	return binary.BigEndian.Uint64(ctrBytes)
}
