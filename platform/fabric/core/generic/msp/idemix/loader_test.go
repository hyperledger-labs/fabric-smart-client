/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func TestIdentityLoader_Load(t *testing.T) { //nolint:paralleltest

	kvss, err := kvs.New(newKVS(), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(), newSignerInfo())

	loader := &idemix.IdentityLoader{
		KVS:           kvss,
		SignerService: sigService,
	}

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathReturns("./testdata/idemix")

	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)
	mockManager.CacheSizeReturns(10)

	mspConfig := config.MSP{
		ID:        "idemix",
		MSPType:   idemix.MSPType,
		MSPID:     "idemix",
		Path:      "./testdata/idemix",
		CacheSize: 5,
	}

	// Successful load
	err = loader.Load(mockManager, mspConfig)
	require.NoError(t, err)
	require.Equal(t, 1, mockManager.AddMSPCallCount())
	require.Equal(t, 1, mockManager.AddDeserializerCallCount())
	id, mspType, enrollmentID, idGetter := mockManager.AddMSPArgsForCall(0)
	require.Equal(t, "idemix", id)
	require.Equal(t, idemix.MSPType, mspType)
	require.NotEmpty(t, enrollmentID)
	require.NotNil(t, idGetter)

	// Error path: invalid config path
	mockConfigErr := &mock.Config{}
	mockConfigErr.TranslatePathReturns("./invalid/path")
	mockManagerErr := &mock.Manager{}
	mockManagerErr.ConfigReturns(mockConfigErr)

	err = loader.Load(mockManagerErr, mspConfig)
	require.ErrorContains(t, err, "failed reading idemix msp configuration")
}
