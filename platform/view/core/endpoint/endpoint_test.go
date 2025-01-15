/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type mockKVS struct{}

func (k mockKVS) GetBinding(ephemeral view.Identity) (view.Identity, error) { return nil, nil }
func (k mockKVS) PutBinding(ephemeral, longTerm view.Identity) error        { return nil }

type mockExtractor struct{}

func (m mockExtractor) ExtractPublicKey(id view.Identity) (any, error) {
	return []byte("id"), nil
}

func TestPKIResolveConcurrency(t *testing.T) {
	svc, err := NewService(mockKVS{})
	assert.NoError(err)

	ext := mockExtractor{}
	resolver := &Resolver{}

	err = svc.AddPublicKeyExtractor(ext)
	assert.NoError(err)

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			svc.pkiResolve(resolver)
		}()
	}
	wg.Wait()
}

func TestGetIdentity(t *testing.T) {
	svc, err := NewService(mockKVS{})
	assert.NoError(err)

	ext := mockExtractor{}
	_, err = svc.AddResolver("getbyname", "", map[string]string{}, []string{}, []byte("id"))
	assert.NoError(err)

	err = svc.AddPublicKeyExtractor(ext)
	assert.NoError(err)

	resultID, err := svc.GetIdentity("getbyname", []byte{})
	assert.NoError(err)
	assert.Equal([]byte("id"), []byte(resultID))

	resultID, err = svc.GetIdentity("notfound", []byte("no"))
	assert.Error(err, "identity not found")
	assert.Equal([]byte(nil), []byte(resultID))
}
