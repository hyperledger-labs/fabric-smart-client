/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.state")

type EndorseTransaction interface {
}

type MetaHandler interface {
	StoreMeta(ns *Namespace, s interface{}, namespace string, key string, options *addOutputOptions) error
}

type ViewManager interface {
	InitiateView(view view.View) (interface{}, error)
}

// Command models an operation that involve given business parties
type Command struct {
	// Name of the commands
	Name string
	// Ids contains the identities that the command involves
	Ids Identities
}

type Header struct {
	Commands []*Command
}

type Namespace struct {
	tx              *endorser.Transaction
	ns              string
	codec           Codec
	metaHandlers    []MetaHandler
	certifiedInputs map[string][]byte
}

func NewNamespace(tx *endorser.Transaction, forceSBE bool) *Namespace {
	return &Namespace{
		tx:    tx,
		codec: &JSONCodec{},
		metaHandlers: []MetaHandler{
			&sbeMetaHandler{forceSBE: forceSBE},
			&contractMetaHandler{},
		},
		certifiedInputs: map[string][]byte{},
	}
}

func NewNamespaceForName(tx *endorser.Transaction, ns string, forceSBE bool) *Namespace {
	return &Namespace{
		tx:    tx,
		ns:    ns,
		codec: &JSONCodec{},
		metaHandlers: []MetaHandler{
			&sbeMetaHandler{forceSBE: forceSBE},
			&contractMetaHandler{},
		},
		certifiedInputs: map[string][]byte{},
	}
}

func (n *Namespace) Present() bool {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		return false
	}

	ns := n.namespace()
	for _, s := range rwSet.Namespaces() {
		if s == ns {
			return true
		}
	}
	return false
}

// SetNamespace sets the name of this namespace
func (n *Namespace) SetNamespace(ns string) {
	n.tx.SetProposal(ns, "Version-0.0", "_state")
}

// AddCommand appends a new Command to this namespace
func (n *Namespace) AddCommand(command string, ids ...view.Identity) error {
	tx := &Header{}
	params := n.tx.Parameters()
	flag := false
	if len(params) == 0 {
		tx = &Header{
			Commands: []*Command{},
		}
		flag = true
	} else {
		err := json.Unmarshal(params[0], tx)
		if err != nil {
			return errors.Wrap(err, "failed unmarshalling tx entry")
		}
	}
	tx.Commands = append(tx.Commands, &Command{
		Name: command,
		Ids:  ids,
	})
	raw, err := json.Marshal(tx)
	if err != nil {
		return errors.Wrap(err, "failed marshalling tx entry")
	}
	if flag {
		n.tx.AppendParameter(raw)
	} else {
		if err := n.tx.SetParameterAt(0, raw); err != nil {
			return errors.Wrap(err, "failed setting tx entry")
		}
	}
	return nil
}

// AddInputByLinearID add a reference to the state with the passed id.
// In addition, the function pupulates the passed state with the content of state associated to the passed id and
// stored in the vault.
// Options can be passed to change the behaviour of the function.
func (n *Namespace) AddInputByLinearID(id string, state interface{}, opts ...AddInputOption) error {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		return errors.Wrap(err, "filed getting rw set")
	}

	// retrieve the state from the local vault,
	// if the state is not present, it needs to be loaded from other sources, potentially.
	// For example, if the namespace has an associated chaincode, query the chaincode to retrieve the state
	raw, err := rwSet.GetState(n.namespace(), id)
	if err != nil {
		return errors.Wrapf(err, "failed getting state [%s, %s]", n.namespace(), id)
	}

	mapping, err := n.getFieldMapping(n.namespace(), id, true)
	if err != nil {
		return errors.Wrapf(err, "failed getting mapping [%s, %s]", n.namespace(), id)
	}

	if len(mapping) != 0 && len(mapping["_root_"]) != 0 {
		raw = mapping["_root_"]
		// TODO: check hash
	}

	logger.Debugf("AddInputByLinearID [%ss,%s] [%s]", n.namespace(), id, base64.StdEncoding.EncodeToString(raw))
	err = n.codec.Unmarshal(raw, state)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling state [%s, %s] [%s]", n.namespace(), id, string(raw))
	}

	err = n.unmarshalTags(rwSet, state, mapping)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling tags [%s, %s]", n.namespace(), id)
	}

	// parse opts
	addInputOptions := &addInputOptions{}
	for _, opt := range opts {
		if err := opt(addInputOptions); err != nil {
			return errors.Wrapf(err, "failed parsing opts [%s, %s] [%s]", n.namespace(), id, string(raw))
		}
	}
	if addInputOptions.certification {
		if err := n.certifyInput(id); err != nil {
			return errors.Wrapf(err, "failed certifying input [%s, %s] [%s]", n.namespace(), id, string(raw))
		}
	}

	// add state info
	if err := n.setFieldMapping(n.namespace(), id, mapping); err != nil {
		return errors.Wrap(err, "failed setting meta mapping")
	}
	return nil
}

// AddOutput adds the passed state following the passed options.
// This corresponds to a write entry in the RWSet
func (n *Namespace) AddOutput(st interface{}, opts ...AddOutputOption) error {
	var err error
	rwSet, err := n.tx.RWSet()
	if err != nil {
		return errors.Wrap(err, "filed getting rw set")
	}

	// Get ID
	id, err := n.getStateID(st)
	if err != nil {
		return err
	}

	// parse opts
	options := &addOutputOptions{}
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return errors.Wrapf(err, "failed parsing opts [%s, %s]", n.namespace(), id)
		}
	}

	st, mapping, err := n.marshalTags(rwSet, st)
	if err != nil {
		return errors.Wrap(err, "failed parsing tags")
	}

	// Encode
	raw, err := n.codec.Marshal(st)
	if err != nil {
		return err
	}
	logger.Debugf("AddOutput [%s,%s][%b] [%s]", n.namespace(), id, options.hashHiding, base64.StdEncoding.EncodeToString(raw))

	switch {
	case options.hashHiding:
		hash := sha256.New()
		hash.Write(raw)
		rawHashed := hash.Sum(nil)

		// store preimage in mapping
		if len(mapping) == 0 {
			mapping = map[string][]byte{}
		}
		mapping["_root_"] = raw

		// store hash
		err = rwSet.SetState(n.namespace(), id, rawHashed)
		if err != nil {
			return err
		}
	default:
		err = rwSet.SetState(n.namespace(), id, raw)
		if err != nil {
			return err
		}
	}

	// Store mapping
	if err := n.setFieldMapping(n.namespace(), id, mapping); err != nil {
		return errors.Wrap(err, "failed setting meta mapping")
	}

	// set meta
	if err = n.setMeta(st, n.namespace(), id, options); err != nil {
		return errors.Wrap(err, "failed setting metadata")
	}

	return nil
}

// GetOutputAt populates the passed state with the content of the output in the passed position
func (n *Namespace) GetOutputAt(index int, state interface{}) error {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		return errors.Wrap(err, "filed getting rw set")
	}
	k, raw, err := rwSet.GetWriteAt(n.namespace(), index)
	if err != nil {
		return errors.Wrapf(err, "failed getting state [%s, %d]", n.namespace(), index)
	}

	mapping, err := n.getFieldMapping(n.namespace(), k, true)
	if err != nil {
		return errors.Wrapf(err, "failed getting mapping [%s, %d] [%s]", n.namespace(), index, string(raw))
	}

	if len(mapping) != 0 && len(mapping["_root_"]) != 0 {
		raw = mapping["_root_"]
		// TODO: check hash
	}

	logger.Debugf("GetOutputAt [%s,%d] [%s]", n.namespace(), index, string(raw))
	err = n.codec.Unmarshal(raw, state)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling state [%s, %d] [%s]", n.namespace(), index, string(raw))
	}

	err = n.unmarshalTags(rwSet, state, mapping)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling tags [%s, %d] [%s]", n.namespace(), index, string(raw))
	}

	return nil
}

// GetInputAt populates the passed state with the content of the input in the passed position.
// The content of the input is loaded from the vault.
func (n *Namespace) GetInputAt(index int, state interface{}) error {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		return errors.Wrap(err, "filed getting rw set")
	}

	flag := true
	k, raw, err := rwSet.GetReadAt(n.namespace(), index)
	if err != nil {
		// check if the state is certified
		k, err = rwSet.GetReadKeyAt(n.namespace(), index)
		if err != nil {
			return errors.Wrapf(err, "failed getting state [%s, %d]", n.namespace(), index)
		}

		v, ok := n.certifiedInputs[k]
		if !ok {
			return errors.Wrapf(err, "failed getting state [%s, %d]", n.namespace(), index)
		}
		raw = v
		flag = false
	}

	mapping, err := n.getFieldMapping(n.namespace(), k, flag)
	if err != nil {
		return errors.Wrapf(err, "failed getting mapping [%s, %d] [%s]", n.namespace(), index, string(raw))
	}

	if len(mapping) != 0 && len(mapping["_root_"]) != 0 {
		raw = mapping["_root_"]
		// TODO: check hash
	}

	logger.Debugf("GetInputAt [%s,%d] [%s]", n.namespace(), index, string(raw))
	err = n.codec.Unmarshal(raw, state)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling state [%s, %d] [%s]", n.namespace(), index, string(raw))
	}

	err = n.unmarshalTags(rwSet, state, mapping)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling tags [%s, %d] [%s]", n.namespace(), index, string(raw))
	}

	return nil
}

func (n *Namespace) Delete(state interface{}) error {
	var err error
	rwSet, err := n.tx.RWSet()
	if err != nil {
		return errors.Wrap(err, "filed getting rw set")
	}

	// Get ID
	id, err := n.getStateID(state)
	if err != nil {
		return err
	}

	// Delete
	logger.Debugf("Delete [%s,%s]", n.namespace(), id)
	err = rwSet.SetState(n.namespace(), id, nil)
	if err != nil {
		return err
	}
	return nil
}

// NumInputs returns the number of inputs (or reads in the RWSet) contained in this namespace
func (n *Namespace) NumInputs() int {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		panic(errors.Wrap(err, "filed getting rw set").Error())
	}

	return rwSet.NumReads(n.namespace())
}

// NumOutputs returns the number of outputs (or writes in the RWSet) contained in this namespace
func (n *Namespace) NumOutputs() int {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		panic(errors.Wrap(err, "filed getting rw set").Error())
	}

	return rwSet.NumWrites(n.namespace())
}

// Commands returns a stream containing the commands in this namespace
func (n *Namespace) Commands() *commandStream {
	params := n.tx.Parameters()
	if len(params) == 0 {
		return &commandStream{
			namespace: n,
			commands:  nil,
		}
	}

	tx := &Header{}
	if err := json.Unmarshal(params[0], tx); err != nil {
		panic(errors.Wrap(err, "failed unmarshalling header entry").Error())
	}
	return &commandStream{
		namespace: n,
		commands:  tx.Commands,
	}
}

// Outputs returns a stream containing the outputs in this namespace
func (n *Namespace) Outputs() *outputStream {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		panic(errors.Wrap(err, "filed getting rw set").Error())
	}

	var outputs []*output
	for i := 0; i < rwSet.NumWrites(n.namespace()); i++ {
		k, v, err := rwSet.GetWriteAt(n.namespace(), i)
		if err != nil {
			panic(errors.Wrapf(err, "filed getting [%d] write", i).Error())
		}
		outputs = append(outputs, &output{
			namespace: n,
			key:       ID(k),
			index:     i,
			delete:    len(v) == 0,
		})
	}

	return &outputStream{namespace: n, outputs: outputs}
}

// Inputs returns a stream containing the inputs in this namespace
func (n *Namespace) Inputs() *inputStream {
	rwSet, err := n.tx.RWSet()
	if err != nil {
		panic(errors.Wrap(err, "filed getting rw set").Error())
	}

	var inputs []*input
	for i := 0; i < rwSet.NumReads(n.namespace()); i++ {
		k, err := rwSet.GetReadKeyAt(n.namespace(), i)
		if err != nil {
			panic(errors.Wrapf(err, "filed getting [%d] read key", i).Error())
		}
		inputs = append(inputs, &input{
			namespace: n,
			index:     i,
			key:       ID(k),
		})
	}
	return &inputStream{n, inputs}
}

// TODO: remove
func (n *Namespace) RWSet() (*fabric.RWSet, error) {
	return n.tx.RWSet()
}

func (n *Namespace) GetService(v interface{}) (interface{}, error) {
	return n.tx.GetService(v)
}

func (n *Namespace) getStateID(s interface{}) (string, error) {
	logger.Debugf("getStateID %v...", s)
	defer logger.Debugf("getStateID...done")
	var key string
	var err error
	switch d := s.(type) {
	case AutoLinearState:
		logger.Debugf("AutoLinearState...")
		key, err = d.GetLinearID()
		if err != nil {
			return "", err
		}
	case LinearState:
		logger.Debugf("LinearState...")
		key = GenerateUUID()
		key = d.SetLinearID(key)
	case EmbeddingState:
		logger.Debugf("EmbeddingState...")
		return n.getStateID(d.GetState())
	default:
		logger.Debugf("default...")
		key = base64.StdEncoding.EncodeToString(GenerateBytesUUID())
	}
	return key, nil
}

func (n *Namespace) setMeta(s interface{}, namespace string, key string, options *addOutputOptions) error {
	for _, handler := range n.metaHandlers {
		if err := handler.StoreMeta(n, s, namespace, key, options); err != nil {
			return errors.Wrapf(err, "failed storing meta for [%s:%s]", namespace, key)
		}
	}
	return nil
}

func (n *Namespace) namespace() string {
	if len(n.ns) == 0 {
		chaincode, _ := n.tx.Chaincode()
		return chaincode
	}
	return n.ns
}
