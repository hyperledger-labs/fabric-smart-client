/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"encoding/binary"
	"math"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	txidNamespace = "txid"
	ctrKey        = "ctr"
	byCtrPrefix   = "C"
	byTxidPrefix  = "T"
)

type SimpleTXIDStore struct {
	persistence driver.Persistence
	ctr         uint64
}

func keyByCtr(ctr uint64) string {
	ctrBytes := make([]byte, binary.MaxVarintLen64)
	binary.BigEndian.PutUint64(ctrBytes, ctr)

	return byCtrPrefix + string(ctrBytes)
}

func keyByTxid(txid string) string {
	return byTxidPrefix + txid
}

func setCtr(persistence driver.Persistence, ctr uint64) error {
	ctrBytes := make([]byte, binary.MaxVarintLen64)
	binary.BigEndian.PutUint64(ctrBytes, ctr)

	err := persistence.SetState(txidNamespace, ctrKey, ctrBytes)
	if err != nil {
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

func NewSimpleTXIDStore(persistence driver.Persistence) (*SimpleTXIDStore, error) {
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

func (s *SimpleTXIDStore) get(txid string) (*ByTxid, error) {
	bytes, err := s.persistence.GetState(txidNamespace, keyByTxid(txid))
	if err != nil {
		return nil, errors.Errorf("error retrieving txid %s [%s]", txid, err.Error())
	}

	if len(bytes) == 0 {
		return nil, nil
	}

	bt := &ByTxid{}
	err = proto.Unmarshal(bytes, bt)
	if err != nil {
		return nil, errors.Errorf("error unmarshalling data for txid %s [%s]", txid, err.Error())
	}

	return bt, nil
}

func (s *SimpleTXIDStore) Get(txid string) (fdriver.ValidationCode, error) {
	bt, err := s.get(txid)
	if err != nil {
		return fdriver.Unknown, err
	}

	if bt == nil {
		return fdriver.Unknown, nil
	}

	return fdriver.ValidationCode(bt.Code), nil
}

func (s *SimpleTXIDStore) Set(txid string, code fdriver.ValidationCode) error {
	// NOTE: we assume that the commit is in progress so no need to update/commit
	// err := s.persistence.BeginUpdate()
	// if err != nil {
	// 	return errors.Errorf("error starting update to set txid %s [%s]", txid, err.Error())
	// }

	// 1: increment ctr in persistence
	err := setCtr(s.persistence, s.ctr+1)
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error storing updated counter for txid %s [%s]", txid, err.Error())
	}

	// 2: store by counter
	byCtrBytes, err := proto.Marshal(&ByNum{
		Txid: txid,
		Code: int32(code),
	})
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error marshalling ByNum for txid %s [%s]", txid, err.Error())
	}
	err = s.persistence.SetState(txidNamespace, keyByCtr(s.ctr), byCtrBytes)
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error storing ByNum for txid %s [%s]", txid, err.Error())
	}

	// 3: store by txid
	byTxidBytes, err := proto.Marshal(&ByTxid{
		Pos:  s.ctr,
		Code: int32(code),
	})
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error marshalling ByTxid for txid %s [%s]", txid, err.Error())
	}
	err = s.persistence.SetState(txidNamespace, keyByTxid(txid), byTxidBytes)
	if err != nil {
		s.persistence.Discard()
		return errors.Errorf("error storing ByTxid for txid %s [%s]", txid, err.Error())
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
	it, err := s.Iterator(&fdriver.SeekEnd{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get txid store iterator")
	}
	defer it.Close()
	next, err := it.Next()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get next from txid store iterator")
	}
	if next == nil {
		return "", nil
	}
	return next.Txid, nil
}

func (s *SimpleTXIDStore) Iterator(pos interface{}) (fdriver.TxidIterator, error) {
	var startKey string
	var endKey string

	switch ppos := pos.(type) {
	case *fdriver.SeekStart:
		startKey = keyByCtr(0)
		endKey = keyByCtr(math.MaxUint64)
	case *fdriver.SeekEnd:
		ctr, err := getCtr(s.persistence)
		if err != nil {
			return nil, err
		}

		startKey = keyByCtr(ctr - 1)
		endKey = keyByCtr(math.MaxUint64)
	case *fdriver.SeekPos:
		bt, err := s.get(ppos.Txid)
		if err != nil {
			return nil, err
		}

		if bt == nil {
			return nil, errors.Errorf("txid %s was not found", ppos.Txid)
		}

		startKey = keyByCtr(bt.Pos)
		endKey = keyByCtr(math.MaxUint64)
	default:
		return nil, errors.Errorf("invalid position %T", pos)
	}

	it, err := s.persistence.GetStateRangeScanIterator(txidNamespace, startKey, endKey)
	if err != nil {
		return nil, err
	}

	return &SimpleTxidIterator{it}, nil
}

type SimpleTxidIterator struct {
	t driver.ResultsIterator
}

func (i *SimpleTxidIterator) Next() (*fdriver.ByNum, error) {
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
		Txid: bn.Txid,
		Code: fdriver.ValidationCode(bn.Code),
	}, nil
}

func (i *SimpleTxidIterator) Close() {
	i.t.Close()
}
