/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"encoding/binary"
	"math"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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

type SimpleTXIDStore struct {
	persistence driver.Persistence
	ctr         uint64
}

func NewTXIDStore(persistence driver.Persistence) (*SimpleTXIDStore, error) {
	ctrBytes, err := persistence.GetState(txidNamespace, ctrKey)
	if err != nil {
		return nil, errors.Errorf("error retrieving txid counter [%s]", err.Error())
	}

	if ctrBytes == nil {
		err = persistence.BeginUpdate()
		if err != nil {
			return nil, errors.Errorf("error starting update to store counter [%s]", err.Error())
		}

		err = setCtr(persistence, 0)
		if err != nil {
			persistence.Discard()
			return nil, err
		}

		err = persistence.Commit()
		if err != nil {
			return nil, errors.Errorf("error committing update to store counter [%s]", err.Error())
		}

		ctrBytes = make([]byte, binary.MaxVarintLen64)
	}

	return &SimpleTXIDStore{
		persistence: persistence,
		ctr:         getCtrFromBytes(ctrBytes),
	}, nil
}

func (s *SimpleTXIDStore) get(txID string) (*ByTxid, error) {
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

func (s *SimpleTXIDStore) Get(txID string) (fdriver.ValidationCode, string, error) {
	bt, err := s.get(txID)
	if err != nil {
		return fdriver.Unknown, "", err
	}

	if bt == nil {
		return fdriver.Unknown, "", nil
	}

	return fdriver.ValidationCode(bt.Code), bt.Message, nil
}

func (s *SimpleTXIDStore) Set(txID string, code fdriver.ValidationCode, message string) error {
	// NOTE: we assume that the commit is in progress so no need to update/commit
	// err := s.persistence.BeginUpdate()
	// if err != nil {
	// 	return errors.Errorf("error starting update to set txid %s [%s]", txid, err.Error())
	// }

	// 1: increment ctr in persistence
	err := setCtr(s.persistence, s.ctr+1)
	if err != nil && !errors2.HasCause(err, driver.UniqueKeyViolation) {
		s.persistence.Discard()
		return errors.Errorf("error storing updated counter for txid %s and code %d [%s]", txID, code, err.Error())
	}

	// 2: store by counter
	byCtrBytes, err := proto.Marshal(&ByNum{
		Txid:    txID,
		Code:    int32(code),
		Message: message,
	})
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error marshalling ByNum for txID %s [%s]", txID, err.Error())
	}
	err = s.persistence.SetState(txidNamespace, keyByCtr(s.ctr), byCtrBytes)
	if err != nil && !errors2.HasCause(err, driver.UniqueKeyViolation) {
		s.persistence.Discard()
		return errors.Errorf("error storing ByNum for txid %s [%s]", txID, err.Error())
	}

	// 3: store by txid
	byTxidBytes, err := proto.Marshal(&ByTxid{
		Pos:     s.ctr,
		Code:    int32(code),
		Message: message,
	})
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error marshalling ByTxid for txid %s [%s]", txID, err.Error())
	}
	err = s.persistence.SetState(txidNamespace, keyByTxID(txID), byTxidBytes)
	if err != nil && !errors2.HasCause(err, driver.UniqueKeyViolation) {
		s.persistence.Discard()
		return errors.Errorf("error storing ByTxid for txid %s [%s]", txID, err.Error())
	}

	if code == fdriver.Valid {
		err = s.persistence.SetState(txidNamespace, lastTX, []byte(txID))
		if err != nil && !errors2.HasCause(err, driver.UniqueKeyViolation) {
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

func (s *SimpleTXIDStore) GetLastTxID() (string, error) {
	v, err := s.persistence.GetState(txidNamespace, lastTX)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get last TxID")
	}
	if len(v) == 0 {
		return "", nil
	}
	return string(v), nil
}

func (s *SimpleTXIDStore) Iterator(pos interface{}) (fdriver.TxidIterator, error) {
	if ppos, ok := pos.(fdriver.SeekSet); ok {
		it, err := s.persistence.GetStateSetIterator(txidNamespace, ppos.TxIDs...)
		if err != nil {
			return nil, err
		}
		return &SimpleTxIDIterator{it}, nil
	}
	startKey, err := s.getStartKey(pos)
	if err != nil {
		return nil, err
	}

	it, err := s.persistence.GetStateRangeScanIterator(txidNamespace, keyByCtr(startKey), keyByCtr(math.MaxUint64))
	if err != nil {
		return nil, err
	}

	return &SimpleTxIDIterator{it}, nil
}

func (s *SimpleTXIDStore) getStartKey(pos interface{}) (uint64, error) {
	switch ppos := pos.(type) {
	case *fdriver.SeekStart:
		return 0, nil
	case *fdriver.SeekEnd:
		ctr, err := getCtr(s.persistence)
		if err != nil {
			return 0, err
		}
		return ctr - 1, nil
	case *fdriver.SeekPos:
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

type SimpleTxIDIterator struct {
	t driver.ResultsIterator
}

func (i *SimpleTxIDIterator) Next() (*fdriver.ByNum, error) {
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

	return &fdriver.ByNum{
		TxID:    bn.Txid,
		Code:    fdriver.ValidationCode(bn.Code),
		Message: bn.Message,
	}, nil
}

func (i *SimpleTxIDIterator) Close() {
	i.t.Close()
}

func keyByCtr(ctr uint64) string {
	ctrBytes := new([binary.MaxVarintLen64]byte)
	binary.BigEndian.PutUint64(ctrBytes[:], ctr)

	return byCtrPrefix + string(ctrBytes[:])
}

func keyByTxID(txID string) string {
	return byTxidPrefix + txID
}

func setCtr(persistence driver.Persistence, ctr uint64) error {
	ctrBytes := new([binary.MaxVarintLen64]byte)
	binary.BigEndian.PutUint64(ctrBytes[:], ctr)

	err := persistence.SetState(txidNamespace, ctrKey, ctrBytes[:])
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
