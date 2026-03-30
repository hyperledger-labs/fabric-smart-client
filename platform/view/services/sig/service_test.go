/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	"context"
	"errors"
	"testing"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/signer_info_store.go -fake-name SignerInfoStore github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.SignerInfoStore
//go:generate counterfeiter -o mock/audit_info_store.go -fake-name AuditInfoStore github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.AuditInfoStore
//go:generate counterfeiter -o mock/signing_identity.go -fake-name SigningIdentity github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.SigningIdentity

func TestNewService(t *testing.T) {
	t.Parallel()

	deserializer := &mock.Deserializer{}
	auditStore := &mock.AuditInfoStore{}
	signerStore := &mock.SignerInfoStore{}

	service := NewService(deserializer, auditStore, signerStore)

	require.NotNil(t, service)
	require.Equal(t, deserializer, service.deserializer)
	require.Equal(t, auditStore, service.auditInfoKVS)
	require.Equal(t, signerStore, service.signerKVS)
	require.NotNil(t, service.signers)
	require.NotNil(t, service.verifiers)
	assert.Empty(t, service.signers)
	assert.Empty(t, service.verifiers)
}

func TestService_RegisterSigner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identity := view.Identity("test-identity")

	tests := []struct {
		name           string
		signer         driver2.Signer
		verifier       driver2.Verifier
		setupStore     func(*mock.SignerInfoStore)
		preRegister    func(*Service)
		expectedErr    string
		expectInCache  bool
		storeCallCount int
	}{
		{
			name:   "nil signer returns error",
			signer: nil,
			setupStore: func(store *mock.SignerInfoStore) {
				// no setup needed
			},
			expectedErr:    "invalid signer",
			storeCallCount: 0,
		},
		{
			name:     "successful registration with verifier",
			signer:   &mock.Signer{},
			verifier: &mock.Verifier{},
			setupStore: func(store *mock.SignerInfoStore) {
				store.PutSignerReturns(nil)
			},
			expectInCache:  true,
			storeCallCount: 1,
		},
		{
			name:   "successful registration without verifier",
			signer: &mock.Signer{},
			setupStore: func(store *mock.SignerInfoStore) {
				store.PutSignerReturns(nil)
			},
			expectInCache:  true,
			storeCallCount: 1,
		},
		{
			name:   "store error rolls back registration",
			signer: &mock.Signer{},
			setupStore: func(store *mock.SignerInfoStore) {
				store.PutSignerReturns(errors.New("store error"))
			},
			expectedErr:    "failed to store entry in kvs",
			expectInCache:  false,
			storeCallCount: 1,
		},
		{
			name:   "already registered signer is ignored",
			signer: &mock.Signer{},
			preRegister: func(s *Service) {
				existingSigner := &mock.Signer{}
				s.signers[identity.UniqueID()] = driver2.SignerEntry{Signer: existingSigner}
			},
			setupStore: func(store *mock.SignerInfoStore) {
				// should not be called
			},
			expectInCache:  true,
			storeCallCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deserializer := &mock.Deserializer{}
			auditStore := &mock.AuditInfoStore{}
			signerStore := &mock.SignerInfoStore{}
			tc.setupStore(signerStore)

			service := NewService(deserializer, auditStore, signerStore)

			if tc.preRegister != nil {
				tc.preRegister(service)
			}

			err := service.RegisterSigner(ctx, identity, tc.signer, tc.verifier)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
			}

			if tc.expectInCache {
				_, exists := service.signers[identity.UniqueID()]
				require.True(t, exists, "signer should be in cache")
			} else {
				_, exists := service.signers[identity.UniqueID()]
				require.False(t, exists, "signer should not be in cache")
			}

			require.Equal(t, tc.storeCallCount, signerStore.PutSignerCallCount())
		})
	}
}

func TestService_RegisterVerifier(t *testing.T) {
	t.Parallel()

	identity := view.Identity("test-identity")

	tests := []struct {
		name          string
		verifier      driver2.Verifier
		preRegister   func(*Service)
		expectedErr   string
		expectInCache bool
	}{
		{
			name:        "nil verifier returns error",
			verifier:    nil,
			expectedErr: "invalid verifier",
		},
		{
			name:          "successful registration",
			verifier:      &mock.Verifier{},
			expectInCache: true,
		},
		{
			name:     "already registered verifier is ignored",
			verifier: &mock.Verifier{},
			preRegister: func(s *Service) {
				existingVerifier := &mock.Verifier{}
				s.verifiers[identity.UniqueID()] = VerifierEntry{Verifier: existingVerifier}
			},
			expectInCache: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := NewService(&mock.Deserializer{}, &mock.AuditInfoStore{}, &mock.SignerInfoStore{})

			if tc.preRegister != nil {
				tc.preRegister(service)
			}

			err := service.RegisterVerifier(identity, tc.verifier)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
			}

			if tc.expectInCache {
				_, exists := service.verifiers[identity.UniqueID()]
				require.True(t, exists, "verifier should be in cache")
			}
		})
	}
}

func TestService_RegisterAuditInfo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identity := view.Identity("test-identity")
	auditInfo := []byte("audit-info")

	tests := []struct {
		name        string
		setupStore  func(*mock.AuditInfoStore)
		expectedErr string
	}{
		{
			name: "successful registration",
			setupStore: func(store *mock.AuditInfoStore) {
				store.PutAuditInfoReturns(nil)
			},
		},
		{
			name: "store error",
			setupStore: func(store *mock.AuditInfoStore) {
				store.PutAuditInfoReturns(errors.New("store error"))
			},
			expectedErr: "store error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			auditStore := &mock.AuditInfoStore{}
			tc.setupStore(auditStore)

			service := NewService(&mock.Deserializer{}, auditStore, &mock.SignerInfoStore{})

			err := service.RegisterAuditInfo(ctx, identity, auditInfo)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, 1, auditStore.PutAuditInfoCallCount())
			gotCtx, gotID, gotInfo := auditStore.PutAuditInfoArgsForCall(0)
			require.Equal(t, ctx, gotCtx)
			require.Equal(t, identity, gotID)
			require.Equal(t, auditInfo, gotInfo)
		})
	}
}

func TestService_GetAuditInfo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identity := view.Identity("test-identity")
	expectedInfo := []byte("audit-info")

	tests := []struct {
		name         string
		setupStore   func(*mock.AuditInfoStore)
		expectedInfo []byte
		expectedErr  string
	}{
		{
			name: "successful retrieval",
			setupStore: func(store *mock.AuditInfoStore) {
				store.GetAuditInfoReturns(expectedInfo, nil)
			},
			expectedInfo: expectedInfo,
		},
		{
			name: "store error",
			setupStore: func(store *mock.AuditInfoStore) {
				store.GetAuditInfoReturns(nil, errors.New("not found"))
			},
			expectedErr: "not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			auditStore := &mock.AuditInfoStore{}
			tc.setupStore(auditStore)

			service := NewService(&mock.Deserializer{}, auditStore, &mock.SignerInfoStore{})

			info, err := service.GetAuditInfo(ctx, identity)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedInfo, info)
			}

			require.Equal(t, 1, auditStore.GetAuditInfoCallCount())
		})
	}
}

func TestService_IsMe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identity := view.Identity("test-identity")

	tests := []struct {
		name        string
		preRegister func(*Service)
		setupStore  func(*mock.SignerInfoStore)
		expected    bool
	}{
		{
			name: "identity in cache",
			preRegister: func(s *Service) {
				s.signers[identity.UniqueID()] = driver2.SignerEntry{Signer: &mock.Signer{}}
			},
			setupStore: func(store *mock.SignerInfoStore) {
				// should not be called
			},
			expected: true,
		},
		{
			name: "identity not in cache, found in store",
			setupStore: func(store *mock.SignerInfoStore) {
				store.FilterExistingSignersReturns([]view.Identity{identity}, nil)
			},
			expected: true,
		},
		{
			name: "identity not found",
			setupStore: func(store *mock.SignerInfoStore) {
				store.FilterExistingSignersReturns([]view.Identity{}, nil)
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			signerStore := &mock.SignerInfoStore{}
			tc.setupStore(signerStore)

			service := NewService(&mock.Deserializer{}, &mock.AuditInfoStore{}, signerStore)

			if tc.preRegister != nil {
				tc.preRegister(service)
			}

			result := service.IsMe(ctx, identity)

			require.Equal(t, tc.expected, result)
		})
	}
}

func TestService_AreMe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id1 := view.Identity("identity-1")
	id2 := view.Identity("identity-2")
	id3 := view.Identity("identity-3")

	tests := []struct {
		name           string
		identities     []view.Identity
		preRegister    func(*Service)
		setupStore     func(*mock.SignerInfoStore)
		expectedCount  int
		expectedHashes []string
	}{
		{
			name:       "all identities in cache",
			identities: []view.Identity{id1, id2},
			preRegister: func(s *Service) {
				s.signers[id1.UniqueID()] = driver2.SignerEntry{Signer: &mock.Signer{}}
				s.signers[id2.UniqueID()] = driver2.SignerEntry{Signer: &mock.Signer{}}
			},
			setupStore: func(store *mock.SignerInfoStore) {
				// should not be called
			},
			expectedCount:  2,
			expectedHashes: []string{id1.UniqueID(), id2.UniqueID()},
		},
		{
			name:       "mixed: some in cache, some in store",
			identities: []view.Identity{id1, id2, id3},
			preRegister: func(s *Service) {
				s.signers[id1.UniqueID()] = driver2.SignerEntry{Signer: &mock.Signer{}}
			},
			setupStore: func(store *mock.SignerInfoStore) {
				store.FilterExistingSignersReturns([]view.Identity{id2}, nil)
			},
			expectedCount:  2,
			expectedHashes: []string{id1.UniqueID(), id2.UniqueID()},
		},
		{
			name:       "none found",
			identities: []view.Identity{id1, id2},
			setupStore: func(store *mock.SignerInfoStore) {
				store.FilterExistingSignersReturns([]view.Identity{}, nil)
			},
			expectedCount: 0,
		},
		{
			name:       "store error returns cache results only",
			identities: []view.Identity{id1, id2},
			preRegister: func(s *Service) {
				s.signers[id1.UniqueID()] = driver2.SignerEntry{Signer: &mock.Signer{}}
			},
			setupStore: func(store *mock.SignerInfoStore) {
				store.FilterExistingSignersReturns(nil, errors.New("store error"))
			},
			expectedCount:  1,
			expectedHashes: []string{id1.UniqueID()},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			signerStore := &mock.SignerInfoStore{}
			tc.setupStore(signerStore)

			service := NewService(&mock.Deserializer{}, &mock.AuditInfoStore{}, signerStore)

			if tc.preRegister != nil {
				tc.preRegister(service)
			}

			result := service.AreMe(ctx, tc.identities...)

			require.Len(t, result, tc.expectedCount)
			for _, hash := range tc.expectedHashes {
				assert.Contains(t, result, hash)
			}
		})
	}
}

func TestService_Info(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identity := view.Identity("test-identity")
	auditInfo := []byte("audit-info")
	expectedInfo := "identity-info"

	tests := []struct {
		name              string
		setupAuditStore   func(*mock.AuditInfoStore)
		setupDeserializer func(*mock.Deserializer)
		expectedContains  string
	}{
		{
			name: "successful info retrieval",
			setupAuditStore: func(store *mock.AuditInfoStore) {
				store.GetAuditInfoReturns(auditInfo, nil)
			},
			setupDeserializer: func(des *mock.Deserializer) {
				des.InfoReturns(expectedInfo, nil)
			},
			expectedContains: expectedInfo,
		},
		{
			name: "audit info not found",
			setupAuditStore: func(store *mock.AuditInfoStore) {
				store.GetAuditInfoReturns(nil, errors.New("not found"))
			},
			setupDeserializer: func(des *mock.Deserializer) {
				// should not be called
			},
			expectedContains: "unable to identify identity",
		},
		{
			name: "deserializer fails",
			setupAuditStore: func(store *mock.AuditInfoStore) {
				store.GetAuditInfoReturns(auditInfo, nil)
			},
			setupDeserializer: func(des *mock.Deserializer) {
				des.InfoReturns("", errors.New("deserialization failed"))
			},
			expectedContains: "unable to identify identity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			auditStore := &mock.AuditInfoStore{}
			tc.setupAuditStore(auditStore)

			deserializer := &mock.Deserializer{}
			tc.setupDeserializer(deserializer)

			service := NewService(deserializer, auditStore, &mock.SignerInfoStore{})

			info := service.Info(ctx, identity)

			require.Contains(t, info, tc.expectedContains)
		})
	}
}

func TestService_GetSigner(t *testing.T) {
	t.Parallel()

	identity := view.Identity("test-identity")

	tests := []struct {
		name              string
		preRegister       func(*Service)
		setupDeserializer func(*mock.Deserializer)
		expectedErr       string
		expectSigner      bool
		deserializeCount  int
	}{
		{
			name: "signer in cache",
			preRegister: func(s *Service) {
				s.signers[identity.UniqueID()] = driver2.SignerEntry{Signer: &mock.Signer{}}
			},
			setupDeserializer: func(des *mock.Deserializer) {
				// should not be called
			},
			expectSigner:     true,
			deserializeCount: 0,
		},
		{
			name: "signer not in cache, successful deserialization",
			setupDeserializer: func(des *mock.Deserializer) {
				signer := &mock.Signer{}
				des.DeserializeSignerReturnsOnCall(0, signer, nil)
			},
			expectSigner:     true,
			deserializeCount: 1,
		},
		{
			name: "deserialization fails",
			setupDeserializer: func(des *mock.Deserializer) {
				des.DeserializeSignerReturns(nil, errors.New("deserialization failed"))
			},
			expectedErr:      "failed deserializing identity for signer",
			deserializeCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deserializer := &mock.Deserializer{}
			tc.setupDeserializer(deserializer)

			service := NewService(deserializer, &mock.AuditInfoStore{}, &mock.SignerInfoStore{})

			if tc.preRegister != nil {
				tc.preRegister(service)
			}

			signer, err := service.GetSigner(identity)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
				require.Nil(t, signer)
			} else {
				require.NoError(t, err)
				if tc.expectSigner {
					require.NotNil(t, signer)
				}
			}

			require.Equal(t, tc.deserializeCount, deserializer.DeserializeSignerCallCount())
		})
	}
}

func TestService_GetVerifier(t *testing.T) {
	t.Parallel()

	identity := view.Identity("test-identity")

	tests := []struct {
		name              string
		preRegister       func(*Service)
		setupDeserializer func(*mock.Deserializer)
		expectedErr       string
		expectVerifier    bool
		deserializeCount  int
	}{
		{
			name: "verifier in cache",
			preRegister: func(s *Service) {
				s.verifiers[identity.UniqueID()] = VerifierEntry{Verifier: &mock.Verifier{}}
			},
			setupDeserializer: func(des *mock.Deserializer) {
				// should not be called
			},
			expectVerifier:   true,
			deserializeCount: 0,
		},
		{
			name: "verifier not in cache, successful deserialization",
			setupDeserializer: func(des *mock.Deserializer) {
				verifier := &mock.Verifier{}
				des.DeserializeVerifierReturnsOnCall(0, verifier, nil)
			},
			expectVerifier:   true,
			deserializeCount: 1,
		},
		{
			name: "deserialization fails",
			setupDeserializer: func(des *mock.Deserializer) {
				des.DeserializeVerifierReturns(nil, errors.New("deserialization failed"))
			},
			expectedErr:      "failed deserializing identity for verifier",
			deserializeCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deserializer := &mock.Deserializer{}
			tc.setupDeserializer(deserializer)

			service := NewService(deserializer, &mock.AuditInfoStore{}, &mock.SignerInfoStore{})

			if tc.preRegister != nil {
				tc.preRegister(service)
			}

			verifier, err := service.GetVerifier(identity)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
				require.Nil(t, verifier)
			} else {
				require.NoError(t, err)
				if tc.expectVerifier {
					require.NotNil(t, verifier)
				}
			}

			require.Equal(t, tc.deserializeCount, deserializer.DeserializeVerifierCallCount())
		})
	}
}

func TestService_GetSigningIdentity(t *testing.T) {
	t.Parallel()

	identity := view.Identity("test-identity")

	tests := []struct {
		name        string
		setupSigner func() driver2.Signer
		preRegister func(*Service, driver2.Signer)
		expectedErr string
		expectSI    bool
	}{
		{
			name: "signer implements SigningIdentity",
			setupSigner: func() driver2.Signer {
				return &mock.SigningIdentity{}
			},
			preRegister: func(s *Service, signer driver2.Signer) {
				s.signers[identity.UniqueID()] = driver2.SignerEntry{Signer: signer}
			},
			expectSI: true,
		},
		{
			name: "signer does not implement SigningIdentity",
			setupSigner: func() driver2.Signer {
				return &mock.Signer{}
			},
			preRegister: func(s *Service, signer driver2.Signer) {
				s.signers[identity.UniqueID()] = driver2.SignerEntry{Signer: signer}
			},
			expectSI: true,
		},
		{
			name: "signer not found",
			setupSigner: func() driver2.Signer {
				return nil
			},
			preRegister: func(s *Service, signer driver2.Signer) {
				// don't register
			},
			expectedErr: "cannot find signer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := NewService(nil, &mock.AuditInfoStore{}, &mock.SignerInfoStore{})
			signer := tc.setupSigner()
			tc.preRegister(service, signer)

			si, err := service.GetSigningIdentity(identity)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
				require.Nil(t, si)
			} else {
				require.NoError(t, err)
				if tc.expectSI {
					require.NotNil(t, si)
				}
			}
		})
	}
}

func TestSigningIdentityWrapper(t *testing.T) {
	t.Parallel()

	identity := view.Identity("test-identity")
	message := []byte("test-message")
	signature := []byte("test-signature")

	signer := &mock.Signer{}
	signer.SignReturns(signature, nil)

	si := &si{
		id:     identity,
		signer: signer,
	}

	t.Run("Sign", func(t *testing.T) {
		sig, err := si.Sign(message)
		require.NoError(t, err)
		require.Equal(t, signature, sig)
		require.Equal(t, 1, signer.SignCallCount())
		require.Equal(t, message, signer.SignArgsForCall(0))
	})

	t.Run("Serialize", func(t *testing.T) {
		serialized, err := si.Serialize()
		require.NoError(t, err)
		require.Equal(t, identity, view.Identity(serialized))
	})

	t.Run("GetPublicVersion", func(t *testing.T) {
		pub := si.GetPublicVersion()
		require.Equal(t, si, pub)
	})

	t.Run("Verify panics", func(t *testing.T) {
		require.Panics(t, func() {
			_ = si.Verify(message, signature)
		})
	})
}
