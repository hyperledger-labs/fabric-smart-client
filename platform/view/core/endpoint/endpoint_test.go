package endpoint

import (
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type mockKVS struct{}

func (k mockKVS) Exists(id string) bool {
	return true
}
func (k mockKVS) Put(id string, state interface{}) error {
	return nil
}
func (k mockKVS) Get(id string, state interface{}) error {
	return nil
}

type mockExtractor struct{}

func (m mockExtractor) ExtractPublicKey(id view.Identity) (any, error) {
	return []byte("id"), nil
}

func TestPKIResolveConcurrency(t *testing.T) {
	svc, err := NewService(nil, nil, mockKVS{})
	assert.NoError(err)

	ext := mockExtractor{}
	resolver := &Resolver{}

	err = svc.AddPublicKeyExtractor(ext)
	assert.NoError(err)

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			svc.pkiResolve(resolver)
		}()
	}
	wg.Wait()
}

func TestGetIdentity(t *testing.T) {
	svc, err := NewService(nil, nil, mockKVS{})
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
