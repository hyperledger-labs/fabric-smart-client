/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package protos holds the protobuf messages FSC nodes exchange and store about transactions
// (package fsc.fabric.v1), and the conversion between a TransactionEnvelope and the JSON
// encoding of a transaction that Transaction.Bytes produces.
//
// A transaction's local form is its JSON encoding: it is what nodes hash, store and decode
// with the existing validation. EnvelopeFromJSON turns it into an envelope to send or store,
// and JSONFromEnvelope turns an envelope back into JSON that the transaction managers decode.
//
// An envelope names its platform by driver name (driver.ChannelKey.Platform) and carries the
// transaction as an opaque payload. Each platform owns the format of its payload, a
// TransactionFormat: this package holds the format of Fabric (driver "generic"), and other
// platforms register theirs with RegisterTransactionFormat. The envelope's routing fields are
// filled and read here, so every platform routes the same way.
//
// The package depends on the Fabric protos and platform/fabric/driver only, so storage
// drivers can convert without importing the transaction core.
package protos

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// fabricPlatform is the driver name of Fabric networks, as driver.ChannelKey.Platform holds it.
const fabricPlatform = "generic"

// ErrNoChannelKey is returned by EnvelopeFromJSON for a transaction whose JSON does not tell its
// platform: one that is not wrapped in a Fabric SerializedTransaction and carries no channel key.
var ErrNoChannelKey = errors.New("cannot tell the platform of a transaction without a channel key")

// ErrNoFormat is returned, wrapped with the platform's name, by the conversions for a
// transaction of a platform without a registered TransactionFormat. A binary built without a
// platform's transaction core has no format for it.
var ErrNoFormat = errors.New("no transaction format registered")

// deterministic marshals messages whose bytes are stored or compared, so the same message
// always gives the same bytes.
var deterministic = proto.MarshalOptions{Deterministic: true}

// TransactionFormat converts one platform's JSON transaction to and from its envelope payload.
type TransactionFormat struct {
	Component     string // negotiation component, e.g. "fabric"
	SkipTxIDCheck bool   // true if the platform's txID is not derived from nonce and creator; zero value applies the check
	// ToPayload returns the payload of raw and the key raw carries (nil if none).
	ToPayload func(raw []byte) (payload []byte, key *driver.ChannelKey, err error)
	// ToJSON returns the JSON of env's payload. It must check env.Channel against the channel
	// header of the payload's signed proposal, if any, and fail on a mismatch.
	ToJSON func(env *TransactionEnvelope) ([]byte, error)
}

var (
	// formatsMu guards formats.
	formatsMu sync.RWMutex
	// formats holds the transaction format of each platform by driver name.
	formats = map[string]TransactionFormat{
		fabricPlatform: {Component: "fabric", ToPayload: fabricToPayload, ToJSON: fabricToJSON},
	}
)

// RegisterTransactionFormat makes f the transaction format of the platform whose driver is named
// platform. A platform registers its format before any of its transactions is converted,
// typically in an init function of the package that defines the format. ToPayload and ToJSON
// must be non-nil. It panics if platform is empty, f.ToPayload or f.ToJSON is nil, or platform
// already has a format: two parts of a binary claiming one platform is a programming error.
func RegisterTransactionFormat(platform string, f TransactionFormat) {
	if platform == "" {
		panic("transaction format registered without a platform")
	}
	if f.ToPayload == nil || f.ToJSON == nil {
		panic("transaction format of platform [" + platform + "] registered with a nil ToPayload or ToJSON")
	}
	formatsMu.Lock()
	defer formatsMu.Unlock()
	if _, ok := formats[platform]; ok {
		panic("transaction format of platform [" + platform + "] registered twice")
	}
	formats[platform] = f
}

// TransactionFormatOf returns the transaction format of the platform whose driver is named
// platform, and false if none is registered.
func TransactionFormatOf(platform string) (TransactionFormat, bool) {
	formatsMu.RLock()
	defer formatsMu.RUnlock()
	f, ok := formats[platform]
	return f, ok
}

// isEnvelope reports whether raw, the bytes of a transaction, is a TransactionEnvelope: its
// first byte is 0x0a, the tag of its platform field, which is never empty. The JSON of a
// transaction starts with '{'; JSON with leading whitespace is not recognized, as 0x0a is also
// '\n'. FSC's own producers never emit JSON with leading whitespace.
func isEnvelope(raw []byte) bool {
	return len(raw) > 0 && raw[0] == 0x0a
}

// TxJSON has the JSON field names of a Fabric Transaction, which the transactions of other
// platforms share. Formats decode it with DecodeStrict, embedded in their own JSON mirror.
type TxJSON struct {
	TCreator           []byte
	TNonce             []byte
	TTxID              string
	TNetwork           string
	TChannel           string
	TChaincode         string
	TChaincodeVersion  string
	TFunction          string
	TParameters        [][]byte
	RWSet              []byte
	TTransient         map[string][]byte
	TSignedProposal    *pb.SignedProposal
	TProposalResponses []*pb.ProposalResponse
	// TProposal is accepted on input and never carried: receivers rebuild it from the signed
	// proposal.
	TProposal json.RawMessage `json:",omitempty"`
}

// ToMessage returns tx as a Transaction message. TNetwork and TChannel are left out, as the
// envelope carries them. It fails on a transaction without a TTxID.
func (tx *TxJSON) ToMessage() (*Transaction, error) {
	if tx.TTxID == "" {
		return nil, errors.New("transaction JSON carries no transaction ID")
	}
	msg := &Transaction{
		Transient:        tx.TTransient,
		TxId:             tx.TTxID,
		Nonce:            tx.TNonce,
		Creator:          tx.TCreator,
		Chaincode:        tx.TChaincode,
		ChaincodeVersion: tx.TChaincodeVersion,
		Function:         tx.TFunction,
		Parameters:       tx.TParameters,
		Rwset:            tx.RWSet,
	}
	if tx.TSignedProposal != nil {
		sp, err := deterministic.Marshal(tx.TSignedProposal)
		if err != nil {
			return nil, errors.Wrap(err, "marshal signed proposal")
		}
		msg.SignedProposal = sp
	}
	for i, r := range tx.TProposalResponses {
		pr, err := deterministic.Marshal(r)
		if err != nil {
			return nil, errors.Wrapf(err, "marshal proposal response [%d]", i)
		}
		msg.ProposalResponses = append(msg.ProposalResponses, pr)
	}
	return msg, nil
}

// TxJSONFromMessage returns the JSON fields of msg, a transaction carried by env, with
// TNetwork and TChannel taken from env and a non-nil transient map. It fails on a transaction
// without a transaction ID, a signed proposal or proposal response that does not decode, and a
// signed proposal whose channel header names a channel other than env's.
func TxJSONFromMessage(msg *Transaction, env *TransactionEnvelope) (*TxJSON, error) {
	if msg.GetTxId() == "" {
		return nil, errors.New("transaction envelope carries no transaction ID")
	}
	tx := &TxJSON{
		TCreator:          msg.GetCreator(),
		TNonce:            msg.GetNonce(),
		TTxID:             msg.GetTxId(),
		TNetwork:          env.GetNetworkHint(),
		TChannel:          env.GetChannel(),
		TChaincode:        msg.GetChaincode(),
		TChaincodeVersion: msg.GetChaincodeVersion(),
		TFunction:         msg.GetFunction(),
		TParameters:       msg.GetParameters(),
		RWSet:             msg.GetRwset(),
		TTransient:        msg.GetTransient(),
	}
	if tx.TTransient == nil {
		tx.TTransient = map[string][]byte{}
	}
	if len(msg.GetSignedProposal()) > 0 {
		sp := &pb.SignedProposal{}
		if err := proto.Unmarshal(msg.GetSignedProposal(), sp); err != nil {
			return nil, errors.Wrap(err, "unmarshal signed proposal")
		}
		channel, err := proposalChannel(sp)
		if err != nil {
			return nil, err
		}
		if channel != env.GetChannel() {
			return nil, errors.Errorf("envelope channel [%s] does not match the signed proposal's channel [%s]", env.GetChannel(), channel)
		}
		tx.TSignedProposal = sp
	}
	for i, b := range msg.GetProposalResponses() {
		pr := &pb.ProposalResponse{}
		if err := proto.Unmarshal(b, pr); err != nil {
			return nil, errors.Wrapf(err, "unmarshal proposal response [%d]", i)
		}
		tx.TProposalResponses = append(tx.TProposalResponses, pr)
	}
	return tx, nil
}

// EnvelopeKey returns the channel key env carries, or nil if env has no genesis hash.
func EnvelopeKey(env *TransactionEnvelope) *driver.ChannelKey {
	if len(env.GetChannelGenesisHash()) == 0 {
		return nil
	}
	return &driver.ChannelKey{Platform: env.GetPlatform(), Channel: env.GetChannel(), GenesisHash: env.GetChannelGenesisHash()}
}

// wrapperJSON has the JSON field names of the Fabric SerializedTransaction, which wraps the
// JSON of a Fabric transaction in Raw.
type wrapperJSON struct {
	Type driver.TransactionType
	Raw  []byte
	Key  *driver.ChannelKey `json:",omitempty"`
}

// fabricToPayload is the ToPayload of the Fabric format: raw is a SerializedTransaction of an
// endorser transaction, and the payload is a Transaction.
func fabricToPayload(raw []byte) ([]byte, *driver.ChannelKey, error) {
	var outer wrapperJSON
	if err := DecodeStrict(raw, &outer); err != nil {
		return nil, nil, errors.Wrap(err, "decode transaction JSON")
	}
	if outer.Type != driver.EndorserTransaction {
		return nil, nil, errors.Errorf("transaction type [%d] cannot be sent as an envelope", outer.Type)
	}
	var tx TxJSON
	if err := DecodeStrict(outer.Raw, &tx); err != nil {
		return nil, nil, errors.Wrap(err, "decode wrapped transaction JSON")
	}
	msg, err := tx.ToMessage()
	if err != nil {
		return nil, nil, err
	}
	payload, err := deterministic.Marshal(msg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal transaction")
	}
	return payload, outer.Key, nil
}

// fabricToJSON is the ToJSON of the Fabric format: it returns a SerializedTransaction wrapping
// the transaction.
func fabricToJSON(env *TransactionEnvelope) ([]byte, error) {
	msg := &Transaction{}
	if err := proto.Unmarshal(env.GetPayload(), msg); err != nil {
		return nil, errors.Wrap(err, "unmarshal transaction")
	}
	tx, err := TxJSONFromMessage(msg, env)
	if err != nil {
		return nil, err
	}
	inner, err := marshalJSON(tx)
	if err != nil {
		return nil, err
	}
	return marshalJSON(&wrapperJSON{Type: driver.EndorserTransaction, Raw: inner, Key: EnvelopeKey(env)})
}

// EnvelopeFromJSON converts raw, the JSON of a transaction, into a marshaled
// TransactionEnvelope. JSON that wraps the transaction in Raw is a Fabric SerializedTransaction;
// any other JSON is a transaction of the platform its channel key names, and fails with
// ErrNoChannelKey if it carries none. The payload is what the platform's TransactionFormat
// gives; the proposal (TProposal) is left out, as receivers rebuild it from the signed proposal.
//
// The conversion refuses JSON whose information the envelope would not keep, so that a value
// that is not a transaction is never turned into one: a field the transaction JSON does not
// have, trailing data after the object, a wrapper that carries transaction fields next to Raw,
// a key inside a wrapped Fabric transaction, and a transaction without a TTxID. It also fails
// on a Fabric transaction type other than driver.EndorserTransaction, which the envelope cannot
// carry, on a wrapper whose key names a platform other than Fabric, on a platform without a
// registered format (wrapping ErrNoFormat), and on a key whose channel differs from the
// transaction's.
func EnvelopeFromJSON(raw []byte) ([]byte, error) {
	return EnvelopeFromJSONFor(raw, "")
}

// EnvelopeFromJSONFor converts raw as EnvelopeFromJSON does, for a network whose driver is named
// platform: raw that does not tell its platform is a transaction of platform, and raw that names
// another platform is an error. Senders use it, as they know the driver of the transaction even
// when its channel key cannot be read yet. An empty platform converts as EnvelopeFromJSON.
func EnvelopeFromJSONFor(raw []byte, want string) ([]byte, error) {
	// probe tells the platform; the format decodes raw strictly.
	var probe struct {
		Raw json.RawMessage
		Key *driver.ChannelKey
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, errors.Wrap(err, "decode transaction JSON")
	}
	platform, txRaw := want, raw
	switch {
	case probe.Raw != nil:
		if probe.Key != nil && probe.Key.Platform != fabricPlatform {
			return nil, errors.Errorf("wrapped transaction carries a channel key of platform [%s]", probe.Key.Platform)
		}
		// A fresh slice: decoding into txRaw would reuse raw's backing array.
		var inner []byte
		if err := json.Unmarshal(probe.Raw, &inner); err != nil {
			return nil, errors.Wrap(err, "decode wrapped transaction JSON")
		}
		platform, txRaw = fabricPlatform, inner
	case probe.Key != nil:
		if probe.Key.Platform == "" {
			return nil, errors.New("channel key names no platform")
		}
		platform = probe.Key.Platform
	case want == "":
		return nil, ErrNoChannelKey
	}
	if want != "" && platform != want {
		return nil, errors.Errorf("transaction of platform [%s] on a [%s] network", platform, want)
	}
	f, ok := TransactionFormatOf(platform)
	if !ok {
		return nil, errors.Wrapf(ErrNoFormat, "unsupported platform [%s]", platform)
	}
	payload, key, err := f.ToPayload(raw)
	if err != nil {
		return nil, err
	}
	// The format accepted raw, so txRaw is the JSON object of the transaction.
	var tx struct{ TChannel, TNetwork string }
	if err := json.Unmarshal(txRaw, &tx); err != nil {
		return nil, errors.Wrap(err, "decode transaction JSON")
	}
	var genesisHash []byte
	if key != nil {
		if key.Channel != tx.TChannel {
			return nil, errors.Errorf("channel key names channel [%s], transaction channel [%s]", key.Channel, tx.TChannel)
		}
		genesisHash = key.GenesisHash
	}
	env, err := deterministic.Marshal(&TransactionEnvelope{
		Platform:           platform,
		ChannelGenesisHash: genesisHash,
		Channel:            tx.TChannel,
		NetworkHint:        tx.TNetwork,
		Payload:            payload,
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal transaction envelope")
	}
	return env, nil
}

// JSONFromEnvelope converts raw, a marshaled TransactionEnvelope, into the JSON that the
// transaction manager of its platform decodes, as the platform's TransactionFormat gives it.
// The JSON always carries a non-nil transient map, and a channel key only when the envelope
// has a genesis hash. The envelope's channel fills both the key and the transaction channel,
// so JSONFromEnvelope fails when it differs from the channel header of the signed proposal. It
// also fails on an empty platform or one without a registered format (wrapping ErrNoFormat), a
// missing transaction ID, and a payload, signed proposal or proposal response that does not decode.
func JSONFromEnvelope(raw []byte) ([]byte, error) {
	env := &TransactionEnvelope{}
	if err := proto.Unmarshal(raw, env); err != nil {
		return nil, errors.Wrap(err, "unmarshal transaction envelope")
	}
	return jsonOf(env)
}

// JSONFor returns raw, the bytes of a transaction, as JSON for a network whose driver is named
// want: an envelope is converted with JSONFromEnvelope, any other bytes are returned unchanged
// for the JSON decoder to accept or reject. It fails on an envelope that does not convert, and,
// unless want is "", on an envelope of a platform other than want.
func JSONFor(raw []byte, want string) ([]byte, error) {
	if !isEnvelope(raw) {
		return raw, nil
	}
	env := &TransactionEnvelope{}
	if err := proto.Unmarshal(raw, env); err != nil {
		return nil, errors.Wrap(err, "unmarshal transaction envelope")
	}
	if want != "" && env.GetPlatform() != want {
		return nil, errors.Errorf("transaction for platform [%s] on a [%s] network", env.GetPlatform(), want)
	}
	return jsonOf(env)
}

// jsonOf converts env as JSONFromEnvelope describes.
func jsonOf(env *TransactionEnvelope) ([]byte, error) {
	if env.GetPlatform() == "" {
		return nil, errors.New("transaction envelope names no platform")
	}
	f, ok := TransactionFormatOf(env.GetPlatform())
	if !ok {
		return nil, errors.Wrapf(ErrNoFormat, "unsupported platform [%s] in transaction envelope", env.GetPlatform())
	}
	return f.ToJSON(env)
}

// SameTransactionJSON reports whether a and b, the JSON of a transaction, hold the same
// transaction as an envelope keeps it. Both are decoded strictly into the transaction's JSON
// mirror: a Fabric SerializedTransaction wrapping a TxJSON, or a TxJSON with a top-level channel
// key, the shape of every other registered platform. JSON that does not decode, such as the JSON
// of a format with further fields, is never the same. The differences listed in
// ignoreRoundTripChanges are not compared. Callers use it to check that JSONFromEnvelope gives
// back the transaction EnvelopeFromJSON was given before they replace the JSON by the envelope.
func SameTransactionJSON(a, b []byte) bool {
	ma, err := decodeMirror(a)
	if err != nil {
		return false
	}
	mb, err := decodeMirror(b)
	if err != nil {
		return false
	}
	ignoreRoundTripChanges(ma)
	ignoreRoundTripChanges(mb)
	// The mirror holds protobuf messages, whose internal state reflect.DeepEqual would compare;
	// their JSON holds their fields only.
	ja, err := json.Marshal(ma)
	if err != nil {
		return false
	}
	jb, err := json.Marshal(mb)
	return err == nil && bytes.Equal(ja, jb)
}

// txMirror is what the JSON of a transaction holds apart from its channel key: the transaction,
// and for a Fabric SerializedTransaction its type.
type txMirror struct {
	Wrapped bool
	Type    driver.TransactionType
	Tx      TxJSON
}

// decodeMirror decodes raw strictly as EnvelopeFromJSON tells its shape: JSON with Raw is a
// Fabric SerializedTransaction, any other JSON a TxJSON with a top-level key. The key is
// decoded, so that a malformed one fails, and then dropped (see ignoreRoundTripChanges).
func decodeMirror(raw []byte) (*txMirror, error) {
	var probe struct{ Raw json.RawMessage }
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	m := &txMirror{Wrapped: probe.Raw != nil}
	if m.Wrapped {
		var w wrapperJSON
		if err := DecodeStrict(raw, &w); err != nil {
			return nil, err
		}
		m.Type = w.Type
		return m, DecodeStrict(w.Raw, &m.Tx)
	}
	var flat struct {
		TxJSON
		Key *driver.ChannelKey
	}
	if err := DecodeStrict(raw, &flat); err != nil {
		return nil, err
	}
	m.Tx = flat.TxJSON
	return m, nil
}

// ignoreRoundTripChanges clears in m what a round trip through an envelope changes by design:
//   - TProposal, which the envelope does not carry: receivers rebuild it from the signed proposal;
//   - nil against empty for RWSet, TParameters and TTransient, which protobuf does not tell apart;
//   - the channel key, which the envelope's routing fields carry and its payload does not: the
//     key reads back from the routing fields, and as no key when there is no genesis hash, so
//     txMirror does not hold it.
func ignoreRoundTripChanges(m *txMirror) {
	m.Tx.TProposal = nil
	if len(m.Tx.RWSet) == 0 {
		m.Tx.RWSet = nil
	}
	if len(m.Tx.TParameters) == 0 {
		m.Tx.TParameters = nil
	}
	if len(m.Tx.TTransient) == 0 {
		m.Tx.TTransient = nil
	}
}

// proposalChannel returns the channel ID in the channel header of the signed proposal sp.
func proposalChannel(sp *pb.SignedProposal) (string, error) {
	prop := &pb.Proposal{}
	if err := proto.Unmarshal(sp.GetProposalBytes(), prop); err != nil {
		return "", errors.Wrap(err, "unmarshal signed proposal: proposal")
	}
	hdr := &cb.Header{}
	if err := proto.Unmarshal(prop.GetHeader(), hdr); err != nil {
		return "", errors.Wrap(err, "unmarshal signed proposal: header")
	}
	chdr := &cb.ChannelHeader{}
	if err := proto.Unmarshal(hdr.GetChannelHeader(), chdr); err != nil {
		return "", errors.Wrap(err, "unmarshal signed proposal: channel header")
	}
	return chdr.GetChannelId(), nil
}

// DecodeStrict decodes raw, a single JSON value, into v, refusing fields v does not have and
// trailing data.
func DecodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after the transaction JSON")
	}
	return nil
}

func marshalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "marshal transaction JSON")
	}
	return raw, nil
}
