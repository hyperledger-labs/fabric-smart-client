/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/stretchr/testify/assert"
)

// mockQueryService implements a mock QueryService for testing
type mockQueryService struct {
	states map[string]*driver.VaultValue
	err    error
}

func newMockQueryService() *mockQueryService {
	return &mockQueryService{
		states: make(map[string]*driver.VaultValue),
	}
}

func (m *mockQueryService) addState(namespace, key string, raw []byte, version []byte) {
	compositeKey := namespace + ":" + key
	m.states[compositeKey] = &driver.VaultValue{
		Raw:     raw,
		Version: version,
	}
}

func (m *mockQueryService) GetState(ns driver.Namespace, key driver.PKey) (*driver.VaultValue, error) {
	if m.err != nil {
		return nil, m.err
	}
	compositeKey := string(ns) + ":" + string(key)
	return m.states[compositeKey], nil
}

func (m *mockQueryService) GetStates(input map[driver.Namespace][]driver.PKey) (map[driver.Namespace]map[driver.PKey]driver.VaultValue, error) {
	if m.err != nil {
		return nil, m.err
	}

	result := make(map[driver.Namespace]map[driver.PKey]driver.VaultValue)
	for ns, keys := range input {
		result[ns] = make(map[driver.PKey]driver.VaultValue)
		for _, key := range keys {
			compositeKey := string(ns) + ":" + string(key)
			if val, ok := m.states[compositeKey]; ok {
				result[ns][key] = *val
			}
		}
	}
	return result, nil
}

// TestRemoteInput represents a sample state for testing
type TestRemoteInput struct {
	ID     string `json:"id"`
	Value  int    `json:"value"`
	Linear string `json:"linear_id"`
}

func (t *TestRemoteInput) SetLinearID(id string) string {
	if len(t.Linear) == 0 {
		t.Linear = id
	}
	return t.Linear
}

func TestRemoteInputStruct(t *testing.T) {
	input := &RemoteInput{
		LinearID: "test-id-123",
		State:    &TestRemoteInput{},
	}

	assert.Equal(t, "test-id-123", input.LinearID)
	assert.NotNil(t, input.State)
}

func TestRemoteInputSlice(t *testing.T) {
	inputs := []RemoteInput{
		{LinearID: "id1", State: &TestRemoteInput{}},
		{LinearID: "id2", State: &TestRemoteInput{}},
		{LinearID: "id3", State: &TestRemoteInput{}},
	}

	assert.Len(t, inputs, 3)
	assert.Equal(t, "id1", inputs[0].LinearID)
	assert.Equal(t, "id2", inputs[1].LinearID)
	assert.Equal(t, "id3", inputs[2].LinearID)
}

func TestMockQueryServiceGetState(t *testing.T) {
	qs := newMockQueryService()
	qs.addState("testns", "key1", []byte(`{"id":"1","value":100}`), []byte{1, 0, 0})

	val, err := qs.GetState("testns", "key1")
	assert.NoError(t, err)
	assert.NotNil(t, val)
	assert.Equal(t, []byte(`{"id":"1","value":100}`), val.Raw)
	assert.Equal(t, []byte{1, 0, 0}, val.Version)
}

func TestMockQueryServiceGetStateNotFound(t *testing.T) {
	qs := newMockQueryService()

	val, err := qs.GetState("testns", "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, val)
}

func TestMockQueryServiceGetStates(t *testing.T) {
	qs := newMockQueryService()
	qs.addState("testns", "key1", []byte(`{"id":"1"}`), []byte{1})
	qs.addState("testns", "key2", []byte(`{"id":"2"}`), []byte{2})
	qs.addState("testns", "key3", []byte(`{"id":"3"}`), []byte{3})

	input := map[driver.Namespace][]driver.PKey{
		"testns": {"key1", "key2", "key3"},
	}

	result, err := qs.GetStates(input)
	assert.NoError(t, err)
	assert.Len(t, result["testns"], 3)

	assert.Equal(t, []byte(`{"id":"1"}`), result["testns"]["key1"].Raw)
	assert.Equal(t, []byte(`{"id":"2"}`), result["testns"]["key2"].Raw)
	assert.Equal(t, []byte(`{"id":"3"}`), result["testns"]["key3"].Raw)
}

func TestMockQueryServiceGetStatesPartial(t *testing.T) {
	qs := newMockQueryService()
	qs.addState("testns", "key1", []byte(`{"id":"1"}`), []byte{1})
	// key2 does not exist

	input := map[driver.Namespace][]driver.PKey{
		"testns": {"key1", "key2"},
	}

	result, err := qs.GetStates(input)
	assert.NoError(t, err)
	assert.Len(t, result["testns"], 1)
	assert.Contains(t, result["testns"], driver.PKey("key1"))
	assert.NotContains(t, result["testns"], driver.PKey("key2"))
}
