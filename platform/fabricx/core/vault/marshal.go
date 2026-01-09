/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"go.uber.org/zap"
)

// Marshaller is the custom marshaller for fabricx.
type Marshaller struct {
	NsInfo map[driver.Namespace]driver.RawVersion
}

func NewMarshaller() *Marshaller {
	return &Marshaller{}
}

func (m *Marshaller) Marshal(txID string, rws *vault.ReadWriteSet) ([]byte, error) {
	logger.Debugf("Marshal rws into fabricx proto [txID=%v]", txID)
	return m.marshal(txID, rws, m.NsInfo)
}

func (m *Marshaller) marshal(txID string, rws *vault.ReadWriteSet, nsInfo map[driver.Namespace]driver.RawVersion) ([]byte, error) {
	if logger.IsEnabledFor(zap.DebugLevel) {
		str, _ := json.MarshalIndent(rws, "", "\t")
		logger.Debugf("Marshal vault.ReadWriteSet %s", string(str))
	}

	type namespaceType struct {
		ns           driver.Namespace
		nsVersion    driver.RawVersion
		readSet      map[string]*protoblocktx.Read
		writeSet     map[string]*protoblocktx.Write
		readWriteSet map[string]*protoblocktx.ReadWrite
	}

	newNamespace := func(ns driver.Namespace, nsVersion driver.RawVersion) *namespaceType {
		return &namespaceType{
			ns:           ns,
			nsVersion:    nsVersion,
			readSet:      make(map[string]*protoblocktx.Read),
			writeSet:     make(map[string]*protoblocktx.Write),
			readWriteSet: make(map[string]*protoblocktx.ReadWrite),
		}
	}

	namespaceSet := make(map[driver.Namespace]*namespaceType)

	// writes ...
	for ns, keyMap := range rws.Writes {
		// check that namespace exists as in _meta
		nsVersion, exists := nsInfo[ns]
		if !exists {
			return nil, errors.Errorf("nsInfo does not contain entry for ns = [%s]", ns)
		}

		if nsVersion == nil {
			return nil, errors.Errorf("nsVersion is nil for ns = [%s]", ns)
		}

		// create namespace if not already exists
		namespace, exists := namespaceSet[ns]
		if !exists {
			namespace = newNamespace(ns, nsVersion)
			namespaceSet[ns] = namespace
		}

		for key, val := range keyMap {
			namespace.writeSet[key] = &protoblocktx.Write{Key: []byte(key), Value: val}
			logger.Debugf("blind write [%s:%s][%x]", namespace.ns, key, val)
		}
	}

	// reads
	for ns, keyMap := range rws.Reads {
		// check that namespace exists as in _meta
		nsVersion, exists := nsInfo[ns]
		if !exists {
			return nil, errors.Errorf("ns = [%s] does not exist in nsInfo", ns)
		}

		// create namespace if not already exists
		namespace, exists := namespaceSet[ns]
		if !exists {
			namespace = newNamespace(ns, nsVersion)
			namespaceSet[ns] = namespace
		}

		for key, ver := range keyMap {
			// note that the version might be nil; this is the case when an entry is read but does not exist
			var vPtr *uint64
			if ver != nil {
				v := Unmarshal(ver)
				vPtr = &v
			}

			// let's check if our read is a read-write or read-only
			if w, exists := namespace.writeSet[key]; exists {
				namespace.readWriteSet[key] = &protoblocktx.ReadWrite{Key: []byte(key), Version: vPtr, Value: w.GetValue()}
				logger.Debugf("blind write was a read write [%s:%s][%x][%v]", namespace.ns, key, w.GetValue(), printVer(vPtr))
				delete(namespace.writeSet, key)
			} else {
				namespace.readSet[key] = &protoblocktx.Read{Key: []byte(key), Version: vPtr}
				logger.Debugf("read [%s:%s][%v]", namespace.ns, key, printVer(vPtr))
			}
		}
	}

	namespaces := make([]*protoblocktx.TxNamespace, 0)
	for _, namespace := range namespaceSet {
		readsOnly := make([]*protoblocktx.Read, 0, len(namespace.readSet))
		for _, read := range namespace.readSet {
			readsOnly = append(readsOnly, read)
		}

		blindWrites := make([]*protoblocktx.Write, 0, len(namespace.writeSet))
		for _, write := range namespace.writeSet {
			blindWrites = append(blindWrites, write)
		}

		readWrites := make([]*protoblocktx.ReadWrite, 0, len(namespace.readWriteSet))
		for _, readWrite := range namespace.readWriteSet {
			readWrites = append(readWrites, readWrite)
		}

		namespaces = append(namespaces, &protoblocktx.TxNamespace{
			NsId:        namespace.ns,
			NsVersion:   Unmarshal(namespace.nsVersion),
			ReadsOnly:   readsOnly,
			ReadWrites:  readWrites,
			BlindWrites: blindWrites,
		})
	}

	txIn := &protoblocktx.Tx{Namespaces: namespaces}
	if logger.IsEnabledFor(zap.DebugLevel) {
		str, _ := json.MarshalIndent(txIn, "", "\t")
		logger.Debugf("Unmarshalled fabricx tx: %s", string(str))
	}

	return proto.Marshal(txIn)
}

// printVer is a helper function to print versions; it is safe to call on nil
func printVer(v *uint64) string {
	if v != nil {
		return fmt.Sprintf("%d", *v)
	}
	return "nil"
}

func (m *Marshaller) RWSetFromBytes(raw []byte, namespaces ...string) (*vault.ReadWriteSet, error) {
	rws := vault.EmptyRWSet()
	if err := m.Append(&rws, raw, namespaces...); err != nil {
		return nil, err
	}
	return &rws, nil
}

func (m *Marshaller) Append(destination *vault.ReadWriteSet, raw []byte, namespaces ...string) error {
	var txIn protoblocktx.Tx
	if err := proto.Unmarshal(raw, &txIn); err != nil {
		return errors.Wrapf(err, "unmarshal tx from [len=%d][%s]", len(raw), logging.SHA256Base64(raw))
	}

	if logger.IsEnabledFor(zap.DebugLevel) {
		str, _ := json.MarshalIndent(&txIn, "", "\t")
		logger.Debugf("Unmarshalled fabricx tx: %s", string(str))
	}

	for _, txNs := range txIn.GetNamespaces() {
		for _, read := range txNs.GetReadsOnly() {
			destination.ReadSet.Add(txNs.GetNsId(), string(read.GetKey()), Marshal(read.GetVersion()))
		}

		for _, write := range txNs.GetBlindWrites() {
			if err := destination.WriteSet.Add(txNs.GetNsId(), string(write.GetKey()), write.GetValue()); err != nil {
				// TODO: ... should we really just stop here or revert all changes ... ?
				return errors.Wrapf(err, "adding blindwrite [%s]", write.GetKey())
			}
		}

		for _, readWrite := range txNs.GetReadWrites() {
			// only add to the readset if it has a version, this is usually the case when a value is created for the first time
			if readWrite.Version != nil {
				destination.ReadSet.Add(txNs.GetNsId(), string(readWrite.GetKey()), Marshal(readWrite.GetVersion()))
			}

			if err := destination.WriteSet.Add(txNs.GetNsId(), string(readWrite.GetKey()), readWrite.GetValue()); err != nil {
				// TODO: ... should we really just stop here or revert all changes ... ?
				return errors.Wrapf(err, "adding readwrite [%s]", readWrite.GetKey())
			}
		}
	}

	return nil
}
