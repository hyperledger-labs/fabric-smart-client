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
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protowire"
)

// Marshaller is the custom marshaller for fabricx that handles serialization and deserialization
// of read-write sets to/from the FabricX protobuf format.
// It maintains namespace version information (NsInfo) required for proper marshalling.
type Marshaller struct {
	// NsInfo maps namespace names to their version information.
	// This must be populated before calling Marshal.
	NsInfo map[driver.Namespace]driver.RawVersion
}

// NewMarshaller creates a new Marshaller instance with an empty namespace info map.
func NewMarshaller() *Marshaller {
	return &Marshaller{}
}

// Marshal serializes a ReadWriteSet into FabricX protobuf format for the given transaction ID.
// It uses the namespace version information stored in m.NsInfo to properly encode namespace versions.
// Returns an error if any namespace in the RWSet is not found in NsInfo or if marshalling fails.
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
		readSet      map[string]*applicationpb.Read
		writeSet     map[string]*applicationpb.Write
		readWriteSet map[string]*applicationpb.ReadWrite
	}

	newNamespace := func(ns driver.Namespace, nsVersion driver.RawVersion) *namespaceType {
		return &namespaceType{
			ns:           ns,
			nsVersion:    nsVersion,
			readSet:      make(map[string]*applicationpb.Read),
			writeSet:     make(map[string]*applicationpb.Write),
			readWriteSet: make(map[string]*applicationpb.ReadWrite),
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
			namespace.writeSet[key] = &applicationpb.Write{Key: []byte(key), Value: val}
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
				v := UnmarshalVersion(ver)
				vPtr = &v
			}

			// let's check if our read is a read-write or read-only
			if w, exists := namespace.writeSet[key]; exists {
				namespace.readWriteSet[key] = &applicationpb.ReadWrite{Key: []byte(key), Version: vPtr, Value: w.GetValue()}
				logger.Debugf("blind write was a read write [%s:%s][%x][%v]", namespace.ns, key, w.GetValue(), printVer(vPtr))
				delete(namespace.writeSet, key)
			} else {
				namespace.readSet[key] = &applicationpb.Read{Key: []byte(key), Version: vPtr}
				logger.Debugf("read [%s:%s][%v]", namespace.ns, key, printVer(vPtr))
			}
		}
	}

	namespaces := make([]*applicationpb.TxNamespace, 0)
	for _, namespace := range namespaceSet {
		readsOnly := make([]*applicationpb.Read, 0, len(namespace.readSet))
		for _, read := range namespace.readSet {
			readsOnly = append(readsOnly, read)
		}

		blindWrites := make([]*applicationpb.Write, 0, len(namespace.writeSet))
		for _, write := range namespace.writeSet {
			blindWrites = append(blindWrites, write)
		}

		readWrites := make([]*applicationpb.ReadWrite, 0, len(namespace.readWriteSet))
		for _, readWrite := range namespace.readWriteSet {
			readWrites = append(readWrites, readWrite)
		}

		namespaces = append(namespaces, &applicationpb.TxNamespace{
			NsId:        namespace.ns,
			NsVersion:   UnmarshalVersion(namespace.nsVersion),
			ReadsOnly:   readsOnly,
			ReadWrites:  readWrites,
			BlindWrites: blindWrites,
		})
	}

	txIn := &applicationpb.Tx{Namespaces: namespaces}
	if logger.IsEnabledFor(zap.DebugLevel) {
		str, _ := json.MarshalIndent(txIn, "", "\t")
		logger.Debugf("Unmarshalled fabricx tx: %s", string(str))
	}

	return proto.Marshal(txIn)
}

// printVer is a helper function to print version pointers safely.
// It returns "nil" for nil pointers and the numeric value otherwise.
func printVer(v *uint64) string {
	if v != nil {
		return fmt.Sprintf("%d", *v)
	}
	return "nil"
}

// RWSetFromBytes deserializes a ReadWriteSet from FabricX protobuf format.
// If namespaces are specified, only those namespaces will be included in the result.
// Returns an error if deserialization fails.
func (m *Marshaller) RWSetFromBytes(raw []byte, namespaces ...string) (*vault.ReadWriteSet, error) {
	rws := vault.EmptyRWSet()
	if err := m.Append(&rws, raw, namespaces...); err != nil {
		return nil, err
	}
	return &rws, nil
}

// Append deserializes FabricX protobuf data and appends it to an existing ReadWriteSet.
// If namespaces are specified, only those namespaces will be processed.
// Returns an error if deserialization fails or if adding reads/writes fails.
func (m *Marshaller) Append(destination *vault.ReadWriteSet, raw []byte, namespaces ...string) error {
	var txIn applicationpb.Tx
	if err := proto.Unmarshal(raw, &txIn); err != nil {
		return errors.Wrapf(err, "unmarshal tx from [len=%d][%s]", len(raw), logging.SHA256Base64(raw))
	}

	if logger.IsEnabledFor(zap.DebugLevel) {
		str, _ := json.MarshalIndent(&txIn, "", "\t")
		logger.Debugf("Unmarshalled fabricx tx: %s", string(str))
	}

	for _, txNs := range txIn.GetNamespaces() {
		for _, read := range txNs.GetReadsOnly() {
			destination.ReadSet.Add(txNs.GetNsId(), string(read.GetKey()), MarshalVersion(read.GetVersion()))
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
				destination.ReadSet.Add(txNs.GetNsId(), string(readWrite.GetKey()), MarshalVersion(readWrite.GetVersion()))
			}

			if err := destination.WriteSet.Add(txNs.GetNsId(), string(readWrite.GetKey()), readWrite.GetValue()); err != nil {
				// TODO: ... should we really just stop here or revert all changes ... ?
				return errors.Wrapf(err, "adding readwrite [%s]", readWrite.GetKey())
			}
		}
	}

	return nil
}

// MarshalVersion encodes a uint64 version number into a protobuf varint byte slice.
// This is used for encoding version information in the FabricX format.
func MarshalVersion(v uint64) []byte {
	return protowire.AppendVarint(nil, v)
}

// UnmarshalVersion decodes a protobuf varint byte slice into a uint64 version number.
// Returns 0 if the input is invalid or empty.
func UnmarshalVersion(raw []byte) uint64 {
	v, _ := protowire.ConsumeVarint(raw)
	return v
}
