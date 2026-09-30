/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/keys"
)

// Marshaller is the custom marshaller for fabricx that handles serialization and deserialization
// of read-write sets to/from the FabricX protobuf format. It is stateless and safe for concurrent
// use: the namespace version information required for marshalling is passed to Marshal per call
// rather than stored on the marshaller.
type Marshaller struct{}

// NewMarshaller creates a new Marshaller instance.
func NewMarshaller() *Marshaller {
	return &Marshaller{}
}

// Marshal serializes a ReadWriteSet into FabricX protobuf format for the given transaction ID.
// nsInfo maps each namespace to its version information and must contain an entry for every
// namespace present in the RWSet. Returns an error if a namespace is missing from nsInfo or if
// marshalling fails. Taking nsInfo as a parameter keeps the marshaller stateless and race-free.
func (*Marshaller) Marshal(txID string, rws *vault.ReadWriteSet, nsInfo map[driver.Namespace]driver.RawVersion) ([]byte, error) {
	logger.Debugf("Marshal rws into fabricx proto [txID=%v]", txID)
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

	// namespaceFor returns the entry for ns, creating it on first use. Every serialized
	// namespace needs a non-nil version: a nil one would silently encode as version 0.
	namespaceFor := func(ns driver.Namespace) (*namespaceType, error) {
		if namespace, exists := namespaceSet[ns]; exists {
			return namespace, nil
		}
		nsVersion, exists := nsInfo[ns]
		if !exists {
			return nil, errors.Errorf("nsInfo does not contain entry for ns = [%s]", ns)
		}
		if nsVersion == nil {
			return nil, errors.Errorf("nsVersion is nil for ns = [%s]", ns)
		}
		namespace := newNamespace(ns, nsVersion)
		namespaceSet[ns] = namespace
		return namespace, nil
	}

	// writes ...
	for ns, keyMap := range rws.Writes {
		namespace, err := namespaceFor(ns)
		if err != nil {
			return nil, err
		}

		for key, val := range keyMap {
			namespace.writeSet[key] = &applicationpb.Write{Key: []byte(key), Value: val}
			logger.Debugf("blind write [%s:%s][%x]", namespace.ns, key, val)
		}
	}

	// reads
	for ns, keyMap := range rws.Reads {
		namespace, err := namespaceFor(ns)
		if err != nil {
			return nil, err
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

	namespaces := make([]*applicationpb.TxNamespace, 0, len(namespaceSet))
	for _, ns := range sortedKeys(namespaceSet) {
		namespace := namespaceSet[ns]

		readsOnly := make([]*applicationpb.Read, 0, len(namespace.readSet))
		for _, key := range sortedKeys(namespace.readSet) {
			readsOnly = append(readsOnly, namespace.readSet[key])
		}

		blindWrites := make([]*applicationpb.Write, 0, len(namespace.writeSet))
		for _, key := range sortedKeys(namespace.writeSet) {
			blindWrites = append(blindWrites, namespace.writeSet[key])
		}

		readWrites := make([]*applicationpb.ReadWrite, 0, len(namespace.readWriteSet))
		for _, key := range sortedKeys(namespace.readWriteSet) {
			readWrites = append(readWrites, namespace.readWriteSet[key])
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

// sortedKeys returns the keys of m in ascending order, so that marshalling
// output does not depend on Go's randomized map iteration order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// RWSetFromBytes deserializes a ReadWriteSet from FabricX protobuf format.
// If namespaces are specified, only those namespaces will be included in the result.
// It also returns the namespace versions carried by the serialized transaction, so a
// reconstructed RWSet re-serializes to the versions it arrived with.
// Returns an error if deserialization fails; see Append for what counts as malformed.
func (m *Marshaller) RWSetFromBytes(raw []byte, namespaces ...string) (*vault.ReadWriteSet, map[driver.Namespace]driver.RawVersion, error) {
	rws := vault.EmptyRWSet()
	nsVersions, err := m.Append(&rws, raw, namespaces...)
	if err != nil {
		return nil, nil, err
	}
	return &rws, nsVersions, nil
}

// Append deserializes FabricX protobuf data and appends it to an existing ReadWriteSet.
// If namespaces are specified, only those namespaces are processed and every other
// namespace in the payload is skipped; with none specified the whole payload is appended.
//
// It returns the NsVersion each processed namespace was serialized with. That version is
// part of what the endorsement signature covers, so a party that reconstructs a
// transaction and re-serializes it must reuse the incoming version rather than resolve a
// current one.
//
// Empty input is the serialization of a transaction without namespaces and yields no
// reads or writes. Input that decodes to fields the applicationpb.Tx schema does not
// define, at any depth, is rejected. Protobuf would otherwise skip such fields, so the
// bytes of an unrelated message would decode as a plausible but wrong transaction, and
// the ReadWriteSet cannot represent them, so re-serializing it would not reproduce the
// endorsed bytes. A foreign message whose wire form happens to match the Tx schema is
// indistinguishable from a transaction and is accepted.
//
// The payload is validated before destination is touched, so a failed Append leaves
// destination unchanged. Returns an error if deserialization fails, if the input carries
// unknown fields, or if the payload carries an invalid namespace ID.
func (*Marshaller) Append(destination *vault.ReadWriteSet, raw []byte, namespaces ...string) (map[driver.Namespace]driver.RawVersion, error) {
	var txIn applicationpb.Tx
	err := proto.Unmarshal(raw, &txIn)
	if err == nil {
		err = checkNoUnknownFields(txIn.ProtoReflect())
	}
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal tx from [len=%d][%s]", len(raw), logging.SHA256Base64(raw))
	}

	if logger.IsEnabledFor(zap.DebugLevel) {
		str, _ := json.MarshalIndent(&txIn, "", "\t")
		logger.Debugf("Unmarshalled fabricx tx: %s", string(str))
	}

	// An empty filter means "every namespace"; otherwise only the listed ones are kept.
	var wanted map[driver.Namespace]struct{}
	if len(namespaces) > 0 {
		wanted = make(map[driver.Namespace]struct{}, len(namespaces))
		for _, ns := range namespaces {
			wanted[ns] = struct{}{}
		}
	}

	txNamespaces := make([]*applicationpb.TxNamespace, 0, len(txIn.GetNamespaces()))
	for _, txNs := range txIn.GetNamespaces() {
		if wanted != nil {
			if _, ok := wanted[txNs.GetNsId()]; !ok {
				continue
			}
		}
		if err := keys.ValidateNs(txNs.GetNsId()); err != nil {
			return nil, err
		}
		txNamespaces = append(txNamespaces, txNs)
	}

	nsVersions := make(map[driver.Namespace]driver.RawVersion, len(txNamespaces))
	for _, txNs := range txNamespaces {
		nsVersions[txNs.GetNsId()] = MarshalVersion(txNs.GetNsVersion())

		for _, read := range txNs.GetReadsOnly() {
			destination.ReadSet.Add(txNs.GetNsId(), string(read.GetKey()), optionalVersion(read.Version))
		}

		for _, write := range txNs.GetBlindWrites() {
			if err := destination.WriteSet.Add(txNs.GetNsId(), string(write.GetKey()), write.GetValue()); err != nil {
				return nil, errors.Wrapf(err, "adding blindwrite [%s]", write.GetKey())
			}
		}

		for _, readWrite := range txNs.GetReadWrites() {
			// A nil version means the key is expected not to exist yet — usually the case when
			// a value is created for the first time. That is still a read dependency and has to
			// enter the read set: a ReadWrite whose read half is dropped re-serializes as a
			// BlindWrite, which is an unconditional write with a different meaning to the
			// committer, and different bytes for the endorsement digest.
			destination.ReadSet.Add(txNs.GetNsId(), string(readWrite.GetKey()), optionalVersion(readWrite.Version))

			if err := destination.WriteSet.Add(txNs.GetNsId(), string(readWrite.GetKey()), readWrite.GetValue()); err != nil {
				return nil, errors.Wrapf(err, "adding readwrite [%s]", readWrite.GetKey())
			}
		}
	}

	return nsVersions, nil
}

// MarshalVersion encodes a uint64 version number into a protobuf varint byte slice.
// This is used for encoding version information in the FabricX format.
func MarshalVersion(v uint64) []byte {
	return protowire.AppendVarint(nil, v)
}

// optionalVersion encodes an optional protobuf version field. An absent version stays
// absent rather than collapsing to version 0: Marshal emits no version for a nil
// RawVersion and version 0 for an encoded zero, and those are different bytes on the wire.
func optionalVersion(v *uint64) driver.RawVersion {
	if v == nil {
		return nil
	}
	return MarshalVersion(*v)
}

// UnmarshalVersion decodes a protobuf varint byte slice into a uint64 version number.
// Returns 0 if the input is invalid or empty.
func UnmarshalVersion(raw []byte) uint64 {
	v, _ := protowire.ConsumeVarint(raw)
	return v
}

// checkNoUnknownFields returns an error if m, or any message nested in it, carries fields
// its schema does not define. Map fields are not traversed; applicationpb.Tx has none.
func checkNoUnknownFields(m protoreflect.Message) error {
	if len(m.GetUnknown()) > 0 {
		return errors.Errorf("unknown fields in %s", m.Descriptor().FullName())
	}
	var err error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap() || fd.Message() == nil:
		case fd.IsList():
			for i := 0; i < v.List().Len() && err == nil; i++ {
				err = checkNoUnknownFields(v.List().Get(i).Message())
			}
		default:
			err = checkNoUnknownFields(v.Message())
		}
		return err == nil
	})
	return err
}
