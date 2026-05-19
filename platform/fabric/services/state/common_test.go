/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"
	"fmt"
	"sort"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type testRWSet struct {
	state    map[cdriver.Namespace]map[cdriver.PKey]cdriver.RawValue
	metadata map[cdriver.Namespace]map[cdriver.PKey]cdriver.Metadata
	reads    map[cdriver.Namespace][]testRead
	writes   map[cdriver.Namespace][]testWrite

	getStateErr         error
	getStateMetadataErr error
	getReadAtErr        error
	getReadKeyAtErr     error
	getWriteAtErr       error
	setStateErr         error
	setStateMetadataErr error
}

type testRead struct {
	key   cdriver.PKey
	value cdriver.RawValue
}

type testWrite struct {
	key   cdriver.PKey
	value cdriver.RawValue
}

func newTestRWSet() *testRWSet {
	return &testRWSet{
		state:    map[cdriver.Namespace]map[cdriver.PKey]cdriver.RawValue{},
		metadata: map[cdriver.Namespace]map[cdriver.PKey]cdriver.Metadata{},
		reads:    map[cdriver.Namespace][]testRead{},
		writes:   map[cdriver.Namespace][]testWrite{},
	}
}

func (t *testRWSet) IsValid() error {
	return nil
}

func (t *testRWSet) IsClosed() bool {
	return false
}

func (t *testRWSet) Clear(ns cdriver.Namespace) error {
	delete(t.state, ns)
	delete(t.metadata, ns)
	delete(t.reads, ns)
	delete(t.writes, ns)
	return nil
}

func (t *testRWSet) AddReadAt(ns cdriver.Namespace, key string, _ cdriver.RawVersion) error {
	v, _ := t.GetState(ns, key)
	t.reads[ns] = append(t.reads[ns], testRead{key: cdriver.PKey(key), value: cloneRaw(v)})
	return nil
}

func (t *testRWSet) SetState(ns cdriver.Namespace, key cdriver.PKey, value cdriver.RawValue) error {
	if t.setStateErr != nil {
		return t.setStateErr
	}
	if _, ok := t.state[ns]; !ok {
		t.state[ns] = map[cdriver.PKey]cdriver.RawValue{}
	}
	t.state[ns][key] = cloneRaw(value)
	t.writes[ns] = append(t.writes[ns], testWrite{key: key, value: cloneRaw(value)})
	return nil
}

func (t *testRWSet) GetState(ns cdriver.Namespace, key cdriver.PKey, _ ...cdriver.GetStateOpt) (cdriver.RawValue, error) {
	if t.getStateErr != nil {
		return nil, t.getStateErr
	}
	if _, ok := t.state[ns]; !ok {
		return nil, nil
	}
	return cloneRaw(t.state[ns][key]), nil
}

func (t *testRWSet) GetDirectState(ns cdriver.Namespace, key cdriver.PKey) (cdriver.RawValue, error) {
	return t.GetState(ns, key)
}

func (t *testRWSet) DeleteState(ns cdriver.Namespace, key cdriver.PKey) error {
	return t.SetState(ns, key, nil)
}

func (t *testRWSet) GetStateMetadata(ns cdriver.Namespace, key cdriver.PKey, _ ...cdriver.GetStateOpt) (cdriver.Metadata, error) {
	if t.getStateMetadataErr != nil {
		return nil, t.getStateMetadataErr
	}
	if _, ok := t.metadata[ns]; !ok {
		return nil, nil
	}
	return cloneMetadata(t.metadata[ns][key]), nil
}

func (t *testRWSet) SetStateMetadata(ns cdriver.Namespace, key cdriver.PKey, metadata cdriver.Metadata) error {
	if t.setStateMetadataErr != nil {
		return t.setStateMetadataErr
	}
	if _, ok := t.metadata[ns]; !ok {
		t.metadata[ns] = map[cdriver.PKey]cdriver.Metadata{}
	}
	t.metadata[ns][key] = cloneMetadata(metadata)
	return nil
}

func (t *testRWSet) GetReadKeyAt(ns cdriver.Namespace, i int) (cdriver.PKey, error) {
	if t.getReadKeyAtErr != nil {
		return "", t.getReadKeyAtErr
	}
	if i < 0 || i >= len(t.reads[ns]) {
		return "", fmt.Errorf("read index out of range: %d", i)
	}
	return t.reads[ns][i].key, nil
}

func (t *testRWSet) GetReadAt(ns cdriver.Namespace, i int) (cdriver.PKey, cdriver.RawValue, error) {
	if t.getReadAtErr != nil {
		return "", nil, t.getReadAtErr
	}
	if i < 0 || i >= len(t.reads[ns]) {
		return "", nil, fmt.Errorf("read index out of range: %d", i)
	}
	r := t.reads[ns][i]
	return r.key, cloneRaw(r.value), nil
}

func (t *testRWSet) GetWriteAt(ns cdriver.Namespace, i int) (cdriver.PKey, cdriver.RawValue, error) {
	if t.getWriteAtErr != nil {
		return "", nil, t.getWriteAtErr
	}
	if i < 0 || i >= len(t.writes[ns]) {
		return "", nil, fmt.Errorf("write index out of range: %d", i)
	}
	w := t.writes[ns][i]
	return w.key, cloneRaw(w.value), nil
}

func (t *testRWSet) NumReads(ns cdriver.Namespace) int {
	return len(t.reads[ns])
}

func (t *testRWSet) NumWrites(ns cdriver.Namespace) int {
	return len(t.writes[ns])
}

func (t *testRWSet) Namespaces() []cdriver.Namespace {
	set := map[cdriver.Namespace]struct{}{}
	for ns := range t.state {
		set[ns] = struct{}{}
	}
	for ns := range t.reads {
		set[ns] = struct{}{}
	}
	for ns := range t.writes {
		set[ns] = struct{}{}
	}
	namespaces := make([]cdriver.Namespace, 0, len(set))
	for ns := range set {
		namespaces = append(namespaces, ns)
	}
	sort.Slice(namespaces, func(i, j int) bool { return namespaces[i] < namespaces[j] })
	return namespaces
}

func (t *testRWSet) AppendRWSet(_ []byte, _ ...cdriver.Namespace) error {
	return nil
}

func (t *testRWSet) Bytes() ([]byte, error) {
	return []byte("rwset"), nil
}

func (t *testRWSet) Done() {}

func (t *testRWSet) Equals(_ interface{}, _ ...cdriver.Namespace) error {
	return nil
}

type testDriverTransaction struct {
	id               string
	network          string
	channel          string
	function         string
	chaincode        string
	chaincodeVersion string
	params           [][]byte
	transient        fdriver.TransientMap
	rwset            fdriver.RWSet

	getRWSetErr      error
	setParameterErr  error
	bytesRaw         []byte
	bytesErr         error
	proposalResponse []byte
	proposalErr      error
}

func newTestDriverTransaction(rwset fdriver.RWSet) *testDriverTransaction {
	return &testDriverTransaction{
		id:               "tx-id",
		network:          "net",
		channel:          "ch",
		function:         "fn",
		chaincode:        "cc",
		chaincodeVersion: "v1",
		transient:        fdriver.TransientMap{},
		rwset:            rwset,
		bytesRaw:         []byte("tx-bytes"),
		proposalResponse: []byte("proposal-response"),
	}
}

func (t *testDriverTransaction) Creator() view.Identity                  { return view.Identity("creator") }
func (t *testDriverTransaction) Nonce() []byte                           { return []byte("nonce") }
func (t *testDriverTransaction) ID() string                              { return t.id }
func (t *testDriverTransaction) Network() string                         { return t.network }
func (t *testDriverTransaction) Channel() string                         { return t.channel }
func (t *testDriverTransaction) Function() string                        { return t.function }
func (t *testDriverTransaction) Parameters() [][]byte                    { return cloneRawSlices(t.params) }
func (t *testDriverTransaction) Chaincode() string                       { return t.chaincode }
func (t *testDriverTransaction) ChaincodeVersion() string                { return t.chaincodeVersion }
func (t *testDriverTransaction) Results() ([]byte, error)                { return nil, nil }
func (t *testDriverTransaction) From(fdriver.Transaction) error          { return nil }
func (t *testDriverTransaction) SetFromBytes([]byte) error               { return nil }
func (t *testDriverTransaction) SetFromEnvelopeBytes([]byte) error       { return nil }
func (t *testDriverTransaction) Proposal() fdriver.Proposal              { return nil }
func (t *testDriverTransaction) SignedProposal() fdriver.SignedProposal  { return nil }
func (t *testDriverTransaction) ResetTransient()                         { t.transient = fdriver.TransientMap{} }
func (t *testDriverTransaction) SetRWSet() error                         { return nil }
func (t *testDriverTransaction) RWS() fdriver.RWSet                      { return t.rwset }
func (t *testDriverTransaction) Done() error                             { return nil }
func (t *testDriverTransaction) Close()                                  {}
func (t *testDriverTransaction) Raw() ([]byte, error)                    { return []byte("raw"), nil }
func (t *testDriverTransaction) Endorse() error                          { return nil }
func (t *testDriverTransaction) EndorseWithIdentity(view.Identity) error { return nil }
func (t *testDriverTransaction) EndorseWithSigner(view.Identity, fdriver.Signer) error {
	return nil
}
func (t *testDriverTransaction) EndorseProposal() error                                  { return nil }
func (t *testDriverTransaction) EndorseProposalWithIdentity(view.Identity) error         { return nil }
func (t *testDriverTransaction) EndorseProposalResponse() error                          { return nil }
func (t *testDriverTransaction) EndorseProposalResponseWithIdentity(view.Identity) error { return nil }
func (t *testDriverTransaction) AppendProposalResponse(fdriver.ProposalResponse) error   { return nil }
func (t *testDriverTransaction) ProposalHasBeenEndorsedBy(view.Identity) error           { return nil }
func (t *testDriverTransaction) StoreTransient() error                                   { return nil }
func (t *testDriverTransaction) ProposalResponses() ([]fdriver.ProposalResponse, error) {
	return nil, nil
}
func (t *testDriverTransaction) Envelope() (fdriver.Envelope, error) { return nil, nil }

func (t *testDriverTransaction) FunctionAndParameters() (string, []string) {
	res := make([]string, 0, len(t.params))
	for _, p := range t.params {
		res = append(res, string(p))
	}
	return t.function, res
}

func (t *testDriverTransaction) SetProposal(chaincode, version, function string, params ...string) {
	t.chaincode = chaincode
	t.chaincodeVersion = version
	t.function = function
	t.params = make([][]byte, 0, len(params))
	for _, p := range params {
		t.params = append(t.params, []byte(p))
	}
}

func (t *testDriverTransaction) AppendParameter(p []byte) {
	t.params = append(t.params, cloneRaw(p))
}

func (t *testDriverTransaction) SetParameterAt(i int, p []byte) error {
	if t.setParameterErr != nil {
		return t.setParameterErr
	}
	if i < 0 || i >= len(t.params) {
		return fmt.Errorf("parameter index out of range: %d", i)
	}
	t.params[i] = cloneRaw(p)
	return nil
}

func (t *testDriverTransaction) Transient() fdriver.TransientMap {
	if t.transient == nil {
		t.transient = fdriver.TransientMap{}
	}
	return t.transient
}

func (t *testDriverTransaction) GetRWSet() (fdriver.RWSet, error) {
	if t.getRWSetErr != nil {
		return nil, t.getRWSetErr
	}
	return t.rwset, nil
}

func (t *testDriverTransaction) Bytes() ([]byte, error) {
	if t.bytesErr != nil {
		return nil, t.bytesErr
	}
	return cloneRaw(t.bytesRaw), nil
}

func (t *testDriverTransaction) ProposalResponse() ([]byte, error) {
	if t.proposalErr != nil {
		return nil, t.proposalErr
	}
	return cloneRaw(t.proposalResponse), nil
}

func (t *testDriverTransaction) BytesNoTransient() ([]byte, error) {
	return t.Bytes()
}

func cloneRaw(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func cloneRawSlices(in [][]byte) [][]byte {
	out := make([][]byte, 0, len(in))
	for _, v := range in {
		out = append(out, cloneRaw(v))
	}
	return out
}

func cloneMetadata(in cdriver.Metadata) cdriver.Metadata {
	if in == nil {
		return nil
	}
	out := cdriver.Metadata{}
	for k, v := range in {
		out[k] = cloneRaw(v)
	}
	return out
}

func newTestEndorserTransactionWithRWSet(rwset *testRWSet) *endorser.Transaction {
	fabricTx := fabric.NewTransaction(nil, newTestDriverTransaction(rwset))
	return &endorser.Transaction{Transaction: fabricTx}
}

func newTestStateTransaction(ns string) (*Transaction, *testRWSet, *testDriverTransaction) {
	rwset := newTestRWSet()
	driverTx := newTestDriverTransaction(rwset)
	endorserTx := &endorser.Transaction{Transaction: fabric.NewTransaction(nil, driverTx)}
	stateTx := &Transaction{
		Transaction: endorserTx,
		Namespace:   NewNamespaceForName(endorserTx, ns, false),
	}
	return stateTx, rwset, driverTx
}

type testBindingStore struct {
	bindings map[string]view.Identity
}

func newTestBindingStore() *testBindingStore {
	return &testBindingStore{bindings: map[string]view.Identity{}}
}

func (t *testBindingStore) GetLongTerm(_ context.Context, ephemeral view.Identity) (view.Identity, error) {
	return t.bindings[string(ephemeral)], nil
}

func (t *testBindingStore) HaveSameBinding(_ context.Context, this, that view.Identity) (bool, error) {
	return t.bindings[string(this)].Equal(t.bindings[string(that)]), nil
}

func (t *testBindingStore) PutBindings(_ context.Context, longTerm view.Identity, ephemeral ...view.Identity) error {
	for _, e := range ephemeral {
		t.bindings[string(e)] = longTerm
	}
	return nil
}

type testLocalMembership struct {
	defaultIdentity view.Identity
	byLabel         map[string]view.Identity
}

func (t *testLocalMembership) DefaultIdentity() view.Identity {
	return t.defaultIdentity
}

func (t *testLocalMembership) AnonymousIdentity() (view.Identity, error) {
	return t.defaultIdentity, nil
}

func (t *testLocalMembership) IsMe(_ context.Context, id view.Identity) bool {
	return t.defaultIdentity.Equal(id)
}

func (t *testLocalMembership) DefaultSigningIdentity() fdriver.SigningIdentity {
	return nil
}

func (t *testLocalMembership) RegisterX509MSP(string, string, string) error {
	return nil
}

func (t *testLocalMembership) RegisterIdemixMSP(string, string, string) error {
	return nil
}

func (t *testLocalMembership) GetIdentityByID(id string) (view.Identity, error) {
	if t.byLabel != nil {
		if v, ok := t.byLabel[id]; ok {
			return v, nil
		}
	}
	return nil, fmt.Errorf("identity [%s] not found", id)
}

func (t *testLocalMembership) GetIdentityInfoByLabel(string, string) *fdriver.IdentityInfo {
	return nil
}

func (t *testLocalMembership) GetIdentityInfoByIdentity(string, view.Identity) *fdriver.IdentityInfo {
	return nil
}

func (t *testLocalMembership) Refresh() error {
	return nil
}

type testIdentityProvider struct {
	identityByLabel map[string]view.Identity
	defaultIdentity view.Identity
}

func (t *testIdentityProvider) Identity(label string) (view.Identity, error) {
	if t.identityByLabel != nil {
		if v, ok := t.identityByLabel[label]; ok {
			return v, nil
		}
	}
	return t.defaultIdentity, nil
}

type testFabricDriverFNS struct {
	name             string
	localMembership  fdriver.LocalMembership
	identityProvider fdriver.IdentityProvider
	channel          fdriver.Channel
	channelErr       error
}

func (t *testFabricDriverFNS) Name() string {
	if t.name == "" {
		return "fns"
	}
	return t.name
}

func (t *testFabricDriverFNS) OrderingService() fdriver.Ordering {
	return nil
}

func (t *testFabricDriverFNS) TransactionManager() fdriver.TransactionManager {
	return nil
}

func (t *testFabricDriverFNS) ProcessorManager() fdriver.ProcessorManager {
	return nil
}

func (t *testFabricDriverFNS) LocalMembership() fdriver.LocalMembership {
	return t.localMembership
}

func (t *testFabricDriverFNS) IdentityProvider() fdriver.IdentityProvider {
	return t.identityProvider
}

func (t *testFabricDriverFNS) Channel(string) (fdriver.Channel, error) {
	if t.channelErr != nil {
		return nil, t.channelErr
	}
	return t.channel, nil
}

func (t *testFabricDriverFNS) Ledger(string) (fdriver.Ledger, error) {
	return nil, nil
}

func (t *testFabricDriverFNS) Committer(string) (fdriver.Committer, error) {
	return nil, nil
}

func (t *testFabricDriverFNS) SignerService() fdriver.SignerService {
	return nil
}

func (t *testFabricDriverFNS) ConfigService() fdriver.ConfigService {
	return nil
}

type testFabricDriverFNSProvider struct {
	fns fdriver.FabricNetworkService
	err error
}

func (t *testFabricDriverFNSProvider) Names() []string {
	return []string{""}
}

func (t *testFabricDriverFNSProvider) DefaultName() string {
	return ""
}

func (t *testFabricDriverFNSProvider) FabricNetworkService(string) (fdriver.FabricNetworkService, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.fns, nil
}

func newTestFabricNetworkServiceProvider(defaultIdentity view.Identity, channel fdriver.Channel) *fabric.NetworkServiceProvider {
	localMembership := &testLocalMembership{
		defaultIdentity: defaultIdentity,
		byLabel: map[string]view.Identity{
			"alice": defaultIdentity,
			"bob":   view.Identity("bob-id"),
		},
	}
	identityProvider := &testIdentityProvider{
		defaultIdentity: defaultIdentity,
		identityByLabel: map[string]view.Identity{
			"alice": defaultIdentity,
			"bob":   view.Identity("bob-id"),
		},
	}
	fns := &testFabricDriverFNS{
		name:             "fns",
		localMembership:  localMembership,
		identityProvider: identityProvider,
		channel:          channel,
	}
	return fabric.NewNetworkServiceProvider(&testFabricDriverFNSProvider{fns: fns}, nil)
}
