/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type transientStore struct {
	store  map[string][]byte
	setErr error
}

func (t *transientStore) SetTransient(key string, raw []byte) error {
	if t.setErr != nil {
		return t.setErr
	}
	if t.store == nil {
		t.store = map[string][]byte{}
	}
	t.store[key] = append([]byte(nil), raw...)
	return nil
}

func (t *transientStore) GetTransient(key string) []byte {
	return append([]byte(nil), t.store[key]...)
}

func TestCertificationTypeFunctions(t *testing.T) {
	t.Parallel()

	ts := &transientStore{store: map[string][]byte{}}

	err := SetCertificationType(ts, ChaincodeCertification, nil)
	require.NoError(t, err)
	require.Equal(t, []byte(ChaincodeCertification), ts.store[CertificationType])

	typ, value, err := GetCertificationType(ts)
	require.NoError(t, err)
	require.Equal(t, ChaincodeCertification, typ)
	require.Nil(t, value)

	emptyType, emptyValue, err := GetCertificationType(&transientStore{store: map[string][]byte{}})
	require.NoError(t, err)
	require.Empty(t, emptyType)
	require.Nil(t, emptyValue)

	err = SetCertificationType(ts, "unknown", nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "not recognized")

	tsUnknown := &transientStore{store: map[string][]byte{CertificationType: []byte("unknown")}}
	_, _, err = GetCertificationType(tsUnknown)
	require.Error(t, err)
	require.ErrorContains(t, err, "not recognized")
}

func TestCertificationPayloadFunctions(t *testing.T) {
	t.Parallel()

	ts := &transientStore{store: map[string][]byte{}}
	key := "asset1"
	value := []byte("cert-bytes")

	err := SetCertification(ts, key, value)
	require.NoError(t, err)

	got, err := GetCertification(ts, key)
	require.NoError(t, err)
	require.Equal(t, value, got)

	tsErr := &transientStore{setErr: errors.New("set transient failed")}
	err = SetCertification(tsErr, key, value)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed appending certification type")
}

func TestCertificationKey(t *testing.T) {
	t.Parallel()

	composite, err := CreateCompositeKey("asset", []string{"owner", "id1"})
	require.NoError(t, err)

	key, err := certificationKey(composite)
	require.NoError(t, err)
	require.Equal(t, "\x00Certification\x00asset\x00owner\x00id1\x00", key)
}

func TestCertificationViewFactory(t *testing.T) {
	t.Parallel()

	f := &CertificationViewFactory{}
	v, err := f.NewView([]byte(`{"Channel":"mych","Namespace":"myns","Key":"k1"}`))
	require.NoError(t, err)

	cv, ok := v.(*CertificationView)
	require.True(t, ok)
	require.Equal(t, "mych", cv.Channel)
	require.Equal(t, "myns", cv.Namespace)
	require.Equal(t, "k1", cv.Key)

	_, err = f.NewView([]byte("{bad-json"))
	require.Error(t, err)
	require.ErrorContains(t, err, "failed unmarshalling input")
}

func TestRecipientSerializationHelpers(t *testing.T) {
	t.Parallel()

	rd := &RecipientData{Identity: []byte("alice")}
	raw, err := rd.Bytes()
	require.NoError(t, err)
	rd2 := &RecipientData{}
	require.NoError(t, rd2.FromBytes(raw))
	require.Equal(t, rd.Identity, rd2.Identity)
	require.Error(t, rd2.FromBytes([]byte("{bad-json")))

	ex := &ExchangeRecipientRequest{
		Channel:       "ch1",
		WalletID:      []byte("wallet"),
		RecipientData: rd,
	}
	exRaw, err := ex.Bytes()
	require.NoError(t, err)
	ex2 := &ExchangeRecipientRequest{}
	require.NoError(t, ex2.FromBytes(exRaw))
	require.Equal(t, ex.Channel, ex2.Channel)
	require.Equal(t, ex.WalletID, ex2.WalletID)
	require.NotNil(t, ex2.RecipientData)
	require.Equal(t, rd.Identity, ex2.RecipientData.Identity)
	require.Error(t, ex2.FromBytes([]byte("{bad-json")))

	rr := &RecipientRequest{Network: "network1"}
	rrRaw, err := rr.Bytes()
	require.NoError(t, err)
	rr2 := &RecipientRequest{}
	require.NoError(t, rr2.FromBytes(rrRaw))
	require.Equal(t, rr.Network, rr2.Network)
	require.Error(t, rr2.FromBytes([]byte("{bad-json")))
}

func TestNamespaceVerifyInputCertificationAtBranches(t *testing.T) {
	t.Parallel()

	t.Run("missing certification type", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		err := tx.VerifyInputCertificationAt(0, "k1")
		require.Error(t, err)
		require.ErrorContains(t, err, "no certification type found")
	})

	t.Run("unknown certification type", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.transient[CertificationType] = []byte("unknown")
		err := tx.VerifyInputCertificationAt(0, "k1")
		require.Error(t, err)
		require.ErrorContains(t, err, "not recognized")
	})

	t.Run("rwset retrieval failure", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.transient[CertificationType] = []byte(ChaincodeCertification)
		driverTx.getRWSetErr = errors.New("rwset failed")
		err := tx.VerifyInputCertificationAt(0, "k1")
		require.Error(t, err)
		require.ErrorContains(t, err, "filed getting rw set")
	})

	t.Run("read key retrieval failure", func(t *testing.T) {
		t.Parallel()
		tx, rwset, driverTx := newTestStateTransaction("assetns")
		driverTx.transient[CertificationType] = []byte(ChaincodeCertification)
		rwset.getReadKeyAtErr = errors.New("read key failed")
		err := tx.VerifyInputCertificationAt(0, "k1")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting state")
	})

	t.Run("missing certification payload", func(t *testing.T) {
		t.Parallel()
		tx, rwset, driverTx := newTestStateTransaction("assetns")
		driverTx.transient[CertificationType] = []byte(ChaincodeCertification)
		tx.Provider = &mockServiceProvider{
			getFn: func(v any) (any, error) {
				return nil, errors.New("service missing")
			},
		}
		key, err := CreateCompositeKey("asset", []string{"1"})
		require.NoError(t, err)
		require.NoError(t, rwset.AddReadAt("assetns", key, nil))

		err = tx.VerifyInputCertificationAt(0, key)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting channel")
	})

	t.Run("channel lookup failure", func(t *testing.T) {
		t.Parallel()
		tx, rwset, driverTx := newTestStateTransaction("assetns")
		driverTx.transient[CertificationType] = []byte(ChaincodeCertification)
		tx.Provider = &mockServiceProvider{
			getFn: func(v any) (any, error) {
				return nil, errors.New("service missing")
			},
		}
		key, err := CreateCompositeKey("asset", []string{"1"})
		require.NoError(t, err)
		require.NoError(t, rwset.AddReadAt("assetns", key, nil))
		require.NoError(t, SetCertification(tx, key, []byte("fake-envelope")))

		err = tx.VerifyInputCertificationAt(0, key)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting channel")
	})
}

func TestNamespaceCertifyInputBranches(t *testing.T) {
	t.Parallel()

	t.Run("missing certification type", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		err := tx.certifyInput("k1")
		require.Error(t, err)
		require.ErrorContains(t, err, "no certification type found")
	})

	t.Run("unknown certification type", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.transient[CertificationType] = []byte("unknown")
		err := tx.certifyInput("k1")
		require.Error(t, err)
		require.ErrorContains(t, err, "not recognized")
	})

	t.Run("channel lookup failure", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction("assetns")
		driverTx.transient[CertificationType] = []byte(ChaincodeCertification)
		tx.Provider = &mockServiceProvider{
			getFn: func(v any) (any, error) {
				return nil, errors.New("service missing")
			},
		}

		err := tx.certifyInput("k1")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting channel")
	})
}

type testCertificationVault struct {
	cert      []byte
	certErr   error
	nsCalled  cdriver.Namespace
	keyCalled cdriver.PKey
}

func (t *testCertificationVault) GetState(context.Context, cdriver.Namespace, cdriver.PKey, interface{}) error {
	return nil
}

func (t *testCertificationVault) GetStateCertification(_ context.Context, namespace cdriver.Namespace, key cdriver.PKey) ([]byte, error) {
	t.nsCalled = namespace
	t.keyCalled = key
	if t.certErr != nil {
		return nil, t.certErr
	}
	return append([]byte(nil), t.cert...), nil
}

type testCertificationVaultService struct {
	vault         Vault
	err           error
	networkCalled string
	channelCalled string
}

func (t *testCertificationVaultService) Vault(network, channel string) (Vault, error) {
	t.networkCalled = network
	t.channelCalled = channel
	if t.err != nil {
		return nil, t.err
	}
	return t.vault, nil
}

func TestCertificationViewCall(t *testing.T) {
	t.Parallel()

	newContext := func(vs VaultService) *mockViewContext {
		nsp := newTestFabricNetworkServiceProvider(
			view.Identity("me"),
			&mockDriverChannel{name: "default-channel", metadata: &mockDriverMetadataService{}},
		)
		return &mockViewContext{
			getServiceFn: func(v interface{}) (interface{}, error) {
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
	}

	t.Run("vault lookup failure", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			getServiceFn: func(interface{}) (interface{}, error) {
				return nil, errors.New("service missing")
			},
		}
		v := &CertificationView{CertificationRequest: &CertificationRequest{
			Channel:   "ch",
			Namespace: "ns",
			Key:       "k1",
		}}
		_, err := v.Call(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "service missing")
	})

	t.Run("certification retrieval failure", func(t *testing.T) {
		t.Parallel()
		vs := &testCertificationVaultService{
			vault: &testCertificationVault{certErr: errors.New("cert failed")},
		}
		ctx := newContext(vs)
		v := &CertificationView{CertificationRequest: &CertificationRequest{
			Channel:   "custom-channel",
			Namespace: "assetns",
			Key:       "asset1",
		}}
		_, err := v.Call(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting certification")
		require.Equal(t, "fns", vs.networkCalled)
		require.Equal(t, "custom-channel", vs.channelCalled)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		vault := &testCertificationVault{cert: []byte("cert-bytes")}
		vs := &testCertificationVaultService{vault: vault}
		ctx := newContext(vs)
		v := &CertificationView{CertificationRequest: &CertificationRequest{
			Channel:   "custom-channel",
			Namespace: "assetns",
			Key:       "asset1",
		}}
		out, err := v.Call(ctx)
		require.NoError(t, err)
		require.Equal(t, []byte("cert-bytes"), out)
		require.Equal(t, cdriver.Namespace("assetns"), vault.nsCalled)
		require.Equal(t, cdriver.PKey("asset1"), vault.keyCalled)
		require.Equal(t, "fns", vs.networkCalled)
		require.Equal(t, "custom-channel", vs.channelCalled)
	})
}
