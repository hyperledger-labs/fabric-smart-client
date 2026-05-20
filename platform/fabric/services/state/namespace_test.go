/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type House struct {
	Address   string
	Valuation uint64
	LinearID  string
	Owner     view.Identity
}

func (h *House) SetLinearID(id string) string {
	if len(h.LinearID) == 0 {
		h.LinearID = id
	}
	return h.LinearID
}

func (h *House) Owners() Identities {
	return []view.Identity{h.Owner}
}

type Asset struct {
	ObjectType        string        `json:"objectType"`
	ID                string        `json:"assetID"`
	Owner             view.Identity `json:"owner"`
	PublicDescription string        `json:"publicDescription"`
	PrivateProperties []byte        `state:"hash" json:"privateProperties"`
}

func TestMarshalTagsPointerToStruct(t *testing.T) {
	t.Parallel()
	n := &Namespace{}
	h := &House{
		Address:   "Universe Drive",
		Valuation: 1000,
		LinearID:  "An ID",
		Owner:     []byte("Apple"),
	}
	h2, mapping, err := n.marshalTags(nil, h)
	require.NoError(t, err)
	err = n.unmarshalTags(nil, h2, mapping)
	require.NoError(t, err)
	require.Equal(t, h, h2)

	id, err := n.getStateID(h2)
	require.NoError(t, err)
	require.Equal(t, id, h2.(*House).LinearID)
}

func TestMarshalTagsPointerToStruct2(t *testing.T) {
	t.Parallel()
	n := &Namespace{}
	h := &Asset{
		ObjectType:        "otype",
		ID:                "1234",
		PublicDescription: "public",
		PrivateProperties: []byte("private"),
		Owner:             []byte("Apple"),
	}
	h2, mapping, err := n.marshalTags(nil, h)
	require.NoError(t, err)
	err = n.unmarshalTags(nil, h2, mapping)
	require.NoError(t, err)
	require.Equal(t, h, h2)
}

type testAutoLinearState struct {
	id  string
	err error
}

func (t *testAutoLinearState) GetLinearID() (string, error) {
	return t.id, t.err
}

type testEmbeddingState struct {
	state interface{}
}

func (t *testEmbeddingState) GetState() interface{} {
	return t.state
}

type testMetaHandler struct {
	err error
}

func (t *testMetaHandler) StoreMeta(_ *Namespace, _ interface{}, _, _ string, _ *addOutputOptions) error {
	return t.err
}

type trackingVaultService struct {
	network string
	channel string
}

func (t *trackingVaultService) Vault(network, channel string) (Vault, error) {
	t.network = network
	t.channel = channel
	return nil, nil
}

func TestStateWrapperConstructors(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction("ns")

	require.NotNil(t, NewAcceptView(tx))
	require.NotNil(t, NewCollectEndorsementsView(tx, view.Identity("alice")))
	require.NotNil(t, NewCollectApprovesView(tx, view.Identity("alice")))
	require.NotNil(t, NewEndorseView(tx, view.Identity("alice")))
	require.NotNil(t, NewParallelCollectEndorsementsOnProposalView(tx, view.Identity("alice")))
	require.NotNil(t, NewEndorsementOnProposalResponderView(tx))
	require.NotNil(t, NewFinalityView(tx))
	require.NotNil(t, NewFinalityWithTimeoutView(tx, 3*time.Second))
	require.NotNil(t, NewOrderingAndFinalityView(tx))
	require.NotNil(t, NewOrderingAndFinalityWithTimeoutView(tx, 3*time.Second))
	require.NotNil(t, NewRWSetProcessor(nil))
}

func TestNamespaceLifecycleHelpers(t *testing.T) {
	t.Parallel()

	tx, rwset, driverTx := newTestStateTransaction("assetns")
	require.False(t, tx.Present())

	require.NoError(t, rwset.SetState("assetns", "k1", []byte("v1")))
	require.True(t, tx.Present())

	tx.SetNamespace("newcc")
	require.Equal(t, "newcc", driverTx.chaincode)
	require.Equal(t, "_state", driverTx.function)

	require.NoError(t, tx.AddCommand("create", view.Identity("alice")))
	require.NoError(t, tx.AddCommand("approve", view.Identity("bob")))
	cs := tx.Commands()
	require.Equal(t, 2, cs.Count())
	require.Equal(t, "create", cs.At(0).Name)
	require.Equal(t, "approve", cs.At(1).Name)
	require.True(t, cs.At(0).Ids[0].Equal(view.Identity("alice")))
	require.True(t, cs.At(1).Ids[0].Equal(view.Identity("bob")))

	require.NoError(t, rwset.AddReadAt("assetns", "k1", nil))
	require.NoError(t, rwset.SetState("assetns", "k2", []byte("v2")))
	require.Equal(t, 1, tx.NumInputs())
	require.Equal(t, 2, tx.NumOutputs())

	outs := tx.Outputs()
	require.Equal(t, 2, outs.Count())
	require.Equal(t, ID("k1"), outs.At(0).ID())
	require.Equal(t, ID("k2"), outs.At(1).ID())

	ins := tx.Inputs()
	require.Equal(t, 1, ins.Count())
	require.Equal(t, ID("k1"), ins.At(0).ID())

	rw, err := tx.Namespace.RWSet()
	require.NoError(t, err)
	require.NotNil(t, rw)
}

func TestNamespaceAddOutputInputDelete(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("assetns")

	house := &House{
		Address:   "Universe Drive",
		Valuation: 1000,
		Owner:     view.Identity("owner1"),
	}
	require.NoError(t, tx.AddOutput(house))
	require.NotEmpty(t, house.LinearID)

	var out House
	require.NoError(t, tx.GetOutputAt(0, &out))
	require.Equal(t, house.Address, out.Address)
	require.Equal(t, house.Valuation, out.Valuation)
	require.True(t, out.Owner.Equal(house.Owner))

	require.NoError(t, rwset.AddReadAt("assetns", house.LinearID, nil))
	var in House
	require.NoError(t, tx.GetInputAt(0, &in))
	require.Equal(t, house.Address, in.Address)
	require.Equal(t, house.Valuation, in.Valuation)
	require.True(t, in.Owner.Equal(house.Owner))

	var byID House
	require.NoError(t, tx.AddInputByLinearID(house.LinearID, &byID))
	require.Equal(t, house.Address, byID.Address)
	require.Equal(t, house.Valuation, byID.Valuation)
	require.True(t, byID.Owner.Equal(house.Owner))

	require.NoError(t, tx.Delete(house))
	require.Equal(t, 2, tx.NumOutputs())
	last := tx.Outputs().At(1)
	require.True(t, last.IsDelete())
}

func TestNamespaceHashHidingAndFieldMapping(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction("assetns")

	asset := &Asset{
		ObjectType:        "asset",
		ID:                "asset-1",
		Owner:             view.Identity("owner1"),
		PublicDescription: "public",
		PrivateProperties: []byte("secret"),
	}

	require.NoError(t, tx.AddOutput(asset, WithHashHiding()))

	var out Asset
	require.NoError(t, tx.GetOutputAt(0, &out))
	require.Equal(t, asset.ID, out.ID)
	require.Equal(t, asset.PublicDescription, out.PublicDescription)
	require.True(t, out.Owner.Equal(asset.Owner))
	require.Equal(t, []byte("secret"), out.PrivateProperties)
}

func TestNamespaceErrorBranches(t *testing.T) {
	t.Parallel()

	t.Run("add command bad json", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.params = [][]byte{[]byte("{bad-json")}
		err := tx.AddCommand("create")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unmarshalling tx entry")
	})

	t.Run("add command set param error", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		header := &Header{Commands: []*Command{{Name: "existing"}}}
		raw, err := json.Marshal(header)
		require.NoError(t, err)
		driverTx.params = [][]byte{raw}
		driverTx.setParameterErr = errors.New("set param failed")
		err = tx.AddCommand("create")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed setting tx entry")
	})

	t.Run("present rwset error", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.getRWSetErr = errors.New("rwset failed")
		require.False(t, tx.Present())
	})

	t.Run("commands invalid header panic", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.params = [][]byte{[]byte("{bad-json")}
		require.Panics(t, func() {
			_ = tx.Commands()
		})
	})

	t.Run("set meta handler error", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		tx.metaHandlers = []MetaHandler{&testMetaHandler{err: errors.New("meta failed")}}
		err := tx.setMeta(&House{}, "assetns", "k1", &addOutputOptions{})
		require.Error(t, err)
		require.ErrorContains(t, err, "failed storing meta")
	})

	t.Run("get output rwset error", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.getRWSetErr = errors.New("rwset failed")
		err := tx.GetOutputAt(0, &House{})
		require.Error(t, err)
	})

	t.Run("get input rwset error", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.getRWSetErr = errors.New("rwset failed")
		err := tx.GetInputAt(0, &House{})
		require.Error(t, err)
	})

	t.Run("add input by linear id get state error", func(t *testing.T) {
		t.Parallel()
		tx, rwset, _ := newTestStateTransaction("assetns")
		require.NoError(t, rwset.SetState("assetns", "k1", []byte("v1")))
		rwset.getStateErr = errors.New("state failed")
		err := tx.AddInputByLinearID("k1", &House{})
		require.Error(t, err)
	})
}

func TestNamespaceStateIDAndNamespaceResolution(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction("assetns")

	id, err := tx.getStateID(&House{})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	auto := &testAutoLinearState{id: "auto-id"}
	id, err = tx.getStateID(auto)
	require.NoError(t, err)
	require.Equal(t, "auto-id", id)

	autoErr := &testAutoLinearState{err: errors.New("auto failed")}
	_, err = tx.getStateID(autoErr)
	require.Error(t, err)

	embedded := &testEmbeddingState{state: &House{}}
	id, err = tx.getStateID(embedded)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	require.Equal(t, "assetns", tx.namespace())

	tx2, _, driverTx2 := newTestStateTransaction("assetns")
	tx2.ns = ""
	driverTx2.chaincode = "chaincode-a"
	require.Equal(t, "chaincode-a", tx2.namespace())
}

func TestNamespaceGetService(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction("assetns")
	expected := &mockVaultService{}
	tx.Provider = &mockServiceProvider{
		getFn: func(v any) (any, error) {
			return expected, nil
		},
	}

	got, err := tx.GetService("anything")
	require.NoError(t, err)
	require.Same(t, expected, got)
}

func TestContractAndSBEHandlers(t *testing.T) {
	t.Parallel()

	t.Run("contract empty option", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		h := &contractMetaHandler{}
		require.NoError(t, h.StoreMeta(tx.Namespace, &House{}, "assetns", "k1", &addOutputOptions{}))
	})

	t.Run("contract set metadata", func(t *testing.T) {
		t.Parallel()
		tx, rwset, _ := newTestStateTransaction("assetns")
		require.NoError(t, rwset.SetState("assetns", "k1", []byte("v1")))
		h := &contractMetaHandler{}
		require.NoError(t, h.StoreMeta(tx.Namespace, &House{}, "assetns", "k1", &addOutputOptions{contract: "my-contract"}))

		meta, err := rwset.GetStateMetadata("assetns", "k1", cdriver.FromIntermediate)
		require.NoError(t, err)
		require.Equal(t, []byte("my-contract"), meta["contract"])
	})

	t.Run("sbe policy path", func(t *testing.T) {
		t.Parallel()
		tx, rwset, _ := newTestStateTransaction("assetns")
		require.NoError(t, rwset.SetState("assetns", "k1", []byte("v1")))

		owner := &House{Owner: view.Identity("owner1")}
		h := &sbeMetaHandler{forceSBE: true}
		require.NoError(t, h.StoreMeta(tx.Namespace, owner, "assetns", "k1", &addOutputOptions{}))

		meta, err := rwset.GetStateMetadata("assetns", "k1", cdriver.FromIntermediate)
		require.NoError(t, err)
		require.NotEmpty(t, meta[peer.MetaDataKeys_VALIDATION_PARAMETER.String()])
	})

	t.Run("state ep helpers", func(t *testing.T) {
		t.Parallel()
		ep, err := newStateEP(nil)
		require.NoError(t, err)
		ep.addOwner(view.Identity("owner1"))

		policy, err := ep.Policy()
		require.NoError(t, err)
		require.NotEmpty(t, policy)

		ep2, err := newStateEP(policy)
		require.NoError(t, err)
		require.Len(t, ep2.identities, 0)
	})
}

func TestHelperErrorPathsAndStreamMethods(t *testing.T) {
	t.Parallel()

	t.Run("get vault helpers error paths", func(t *testing.T) {
		t.Parallel()
		expected := errors.New("service missing")
		p := &mockServiceProvider{
			getFn: func(v any) (any, error) {
				return nil, expected
			},
		}
		_, err := GetVault(p)
		require.ErrorIs(t, err, expected)
		_, err = GetVaultForChannel(p, "ch")
		require.ErrorIs(t, err, expected)
	})

	t.Run("get vault helpers success paths", func(t *testing.T) {
		t.Parallel()

		vs := &trackingVaultService{}
		nsp := newTestFabricNetworkServiceProvider(
			view.Identity("me"),
			&mockDriverChannel{
				name:     "default-channel",
				metadata: &mockDriverMetadataService{},
			},
		)
		p := &mockServiceProvider{
			getFn: func(v any) (any, error) {
				rt, ok := v.(reflect.Type)
				if !ok {
					return nil, errors.New("unexpected service key")
				}
				switch rt {
				case reflect.TypeOf((*VaultService)(nil)):
					return vs, nil
				case reflect.TypeOf((*fabric.NetworkServiceProvider)(nil)):
					return nsp, nil
				default:
					return nil, errors.New("service missing")
				}
			},
		}

		_, err := GetVault(p)
		require.NoError(t, err)
		require.Equal(t, "fns", vs.network)
		require.Equal(t, "default-channel", vs.channel)

		_, err = GetVaultForChannel(p, "custom-channel")
		require.NoError(t, err)
		require.Equal(t, "fns", vs.network)
		require.Equal(t, "custom-channel", vs.channel)
	})

	t.Run("output state delegates", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.getRWSetErr = errors.New("rwset failed")
		o := &output{namespace: tx.Namespace, index: 0}
		err := o.State(&House{})
		require.Error(t, err)
	})

	t.Run("input state and verify delegates", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.getRWSetErr = errors.New("rwset failed")
		driverTx.transient[CertificationType] = []byte("unknown")
		in := &input{namespace: tx.Namespace, index: 0, key: ID("k1")}
		err := in.State(&House{})
		require.Error(t, err)
		err = in.VerifyCertification()
		require.Error(t, err)
	})
}
