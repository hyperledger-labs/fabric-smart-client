/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	"errors"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig/mock"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/deserializer.go -fake-name Deserializer github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.SigDeserializer
//go:generate counterfeiter -o mock/verifier.go -fake-name Verifier github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.Verifier
//go:generate counterfeiter -o mock/signer.go -fake-name Signer github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.Signer

func TestNewMultiplexDeserializer(t *testing.T) {
	t.Parallel()

	md := NewMultiplexDeserializer()

	require.NotNil(t, md)
	require.Empty(t, md.deserializers)
}

func TestMultiplexDeserializer_AddDeserializer(t *testing.T) {
	t.Parallel()

	md := NewMultiplexDeserializer()
	mock1 := &mock.Deserializer{}
	mock2 := &mock.Deserializer{}

	md.AddDeserializer(mock1)
	require.Len(t, md.deserializers, 1)

	md.AddDeserializer(mock2)
	require.Len(t, md.deserializers, 2)
}

func TestMultiplexDeserializer_DeserializeVerifier(t *testing.T) {
	t.Parallel()

	testRaw := []byte("test-raw-data")

	tests := []struct {
		name               string
		setupDeserializers func() []Deserializer
		expectedErr        string
		expectVerifier     bool
		expectedCallCounts []int
	}{
		{
			name: "success on first deserializer",
			setupDeserializers: func() []Deserializer {
				verifier := &mock.Verifier{}
				des := &mock.Deserializer{}
				des.DeserializeVerifierReturns(verifier, nil)
				return []Deserializer{des}
			},
			expectVerifier:     true,
			expectedCallCounts: []int{1},
		},
		{
			name: "first fails, second succeeds",
			setupDeserializers: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.DeserializeVerifierReturns(nil, errors.New("first error"))

				verifier := &mock.Verifier{}
				des2 := &mock.Deserializer{}
				des2.DeserializeVerifierReturns(verifier, nil)

				return []Deserializer{des1, des2}
			},
			expectVerifier:     true,
			expectedCallCounts: []int{1, 1},
		},
		{
			name: "all deserializers fail",
			setupDeserializers: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.DeserializeVerifierReturns(nil, errors.New("error 1"))

				des2 := &mock.Deserializer{}
				des2.DeserializeVerifierReturns(nil, errors.New("error 2"))

				return []Deserializer{des1, des2}
			},
			expectedErr:        "failed deserialization",
			expectedCallCounts: []int{1, 1},
		},
		{
			name: "no deserializers",
			setupDeserializers: func() []Deserializer {
				return []Deserializer{}
			},
			expectedErr:        "failed deserialization",
			expectedCallCounts: []int{},
		},
		{
			name: "multiple deserializers, third succeeds",
			setupDeserializers: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.DeserializeVerifierReturns(nil, errors.New("error 1"))

				des2 := &mock.Deserializer{}
				des2.DeserializeVerifierReturns(nil, errors.New("error 2"))

				verifier := &mock.Verifier{}
				des3 := &mock.Deserializer{}
				des3.DeserializeVerifierReturns(verifier, nil)

				des4 := &mock.Deserializer{}
				// des4 should not be called

				return []Deserializer{des1, des2, des3, des4}
			},
			expectVerifier:     true,
			expectedCallCounts: []int{1, 1, 1, 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			md := NewMultiplexDeserializer()
			deserializers := tc.setupDeserializers()
			for _, des := range deserializers {
				md.AddDeserializer(des)
			}

			verifier, err := md.DeserializeVerifier(testRaw)

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

			for i, des := range deserializers {
				mockDes := des.(*mock.Deserializer)
				require.Equal(t, tc.expectedCallCounts[i], mockDes.DeserializeVerifierCallCount(),
					"deserializer %d call count mismatch", i)
				if mockDes.DeserializeVerifierCallCount() > 0 {
					require.Equal(t, testRaw, mockDes.DeserializeVerifierArgsForCall(0))
				}
			}
		})
	}
}

func TestMultiplexDeserializer_DeserializeSigner(t *testing.T) {
	t.Parallel()

	testRaw := []byte("test-raw-data")

	tests := []struct {
		name               string
		setupDeserializers func() []Deserializer
		expectedErr        string
		expectSigner       bool
		expectedCallCounts []int
	}{
		{
			name: "success on first deserializer",
			setupDeserializers: func() []Deserializer {
				signer := &mock.Signer{}
				des := &mock.Deserializer{}
				des.DeserializeSignerReturns(signer, nil)
				return []Deserializer{des}
			},
			expectSigner:       true,
			expectedCallCounts: []int{1},
		},
		{
			name: "first fails, second succeeds",
			setupDeserializers: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.DeserializeSignerReturns(nil, errors.New("first error"))

				signer := &mock.Signer{}
				des2 := &mock.Deserializer{}
				des2.DeserializeSignerReturns(signer, nil)

				return []Deserializer{des1, des2}
			},
			expectSigner:       true,
			expectedCallCounts: []int{1, 1},
		},
		{
			name: "all deserializers fail",
			setupDeserializers: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.DeserializeSignerReturns(nil, errors.New("error 1"))

				des2 := &mock.Deserializer{}
				des2.DeserializeSignerReturns(nil, errors.New("error 2"))

				return []Deserializer{des1, des2}
			},
			expectedErr:        "failed signer deserialization",
			expectedCallCounts: []int{1, 1},
		},
		{
			name: "no deserializers",
			setupDeserializers: func() []Deserializer {
				return []Deserializer{}
			},
			expectedErr:        "failed signer deserialization",
			expectedCallCounts: []int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			md := NewMultiplexDeserializer()
			deserializers := tc.setupDeserializers()
			for _, des := range deserializers {
				md.AddDeserializer(des)
			}

			signer, err := md.DeserializeSigner(testRaw)

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

			// Verify call counts
			for i, des := range deserializers {
				mockDes := des.(*mock.Deserializer)
				require.Equal(t, tc.expectedCallCounts[i], mockDes.DeserializeSignerCallCount(),
					"deserializer %d call count mismatch", i)
				if mockDes.DeserializeSignerCallCount() > 0 {
					require.Equal(t, testRaw, mockDes.DeserializeSignerArgsForCall(0))
				}
			}
		})
	}
}

func TestMultiplexDeserializer_Info(t *testing.T) {
	t.Parallel()

	testRaw := []byte("test-raw-data")
	testAuditInfo := []byte("test-audit-info")

	tests := []struct {
		name               string
		setupDeserializers func() []Deserializer
		expectedErr        string
		expectedInfo       string
		expectedCallCounts []int
	}{
		{
			name: "success on first deserializer",
			setupDeserializers: func() []Deserializer {
				des := &mock.Deserializer{}
				des.InfoReturns("identity-info", nil)
				return []Deserializer{des}
			},
			expectedInfo:       "identity-info",
			expectedCallCounts: []int{1},
		},
		{
			name: "first fails, second succeeds",
			setupDeserializers: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.InfoReturns("", errors.New("first error"))

				des2 := &mock.Deserializer{}
				des2.InfoReturns("second-identity-info", nil)

				return []Deserializer{des1, des2}
			},
			expectedInfo:       "second-identity-info",
			expectedCallCounts: []int{1, 1},
		},
		{
			name: "all deserializers fail",
			setupDeserializers: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.InfoReturns("", errors.New("error 1"))

				des2 := &mock.Deserializer{}
				des2.InfoReturns("", errors.New("error 2"))

				return []Deserializer{des1, des2}
			},
			expectedErr:        "failed info deserialization",
			expectedCallCounts: []int{1, 1},
		},
		{
			name: "no deserializers",
			setupDeserializers: func() []Deserializer {
				return []Deserializer{}
			},
			expectedErr:        "failed info deserialization",
			expectedCallCounts: []int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			md := NewMultiplexDeserializer()
			deserializers := tc.setupDeserializers()
			for _, des := range deserializers {
				md.AddDeserializer(des)
			}

			info, err := md.Info(testRaw, testAuditInfo)

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
				require.Empty(t, info)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedInfo, info)
			}

			for i, des := range deserializers {
				mockDes := des.(*mock.Deserializer)
				require.Equal(t, tc.expectedCallCounts[i], mockDes.InfoCallCount(),
					"deserializer %d call count mismatch", i)
				if mockDes.InfoCallCount() > 0 {
					raw, audit := mockDes.InfoArgsForCall(0)
					require.Equal(t, testRaw, raw)
					require.Equal(t, testAuditInfo, audit)
				}
			}
		})
	}
}

func TestMultiplexDeserializer_ThreadSafety(t *testing.T) {
	t.Parallel()

	md := NewMultiplexDeserializer()

	// Add initial deserializers
	for i := 0; i < 5; i++ {
		des := &mock.Deserializer{}
		des.DeserializeVerifierReturns(&mock.Verifier{}, nil)
		des.DeserializeSignerReturns(&mock.Signer{}, nil)
		des.InfoReturns("info", nil)
		md.AddDeserializer(des)
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrently add deserializers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			des := &mock.Deserializer{}
			des.DeserializeVerifierReturns(&mock.Verifier{}, nil)
			md.AddDeserializer(des)
		}()
	}

	// Concurrently call DeserializeVerifier
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = md.DeserializeVerifier([]byte("test"))
		}()
	}

	// Concurrently call DeserializeSigner
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = md.DeserializeSigner([]byte("test"))
		}()
	}

	// Concurrently call Info
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = md.Info([]byte("test"), []byte("audit"))
		}()
	}

	wg.Wait()

	// Verify that all deserializers were added
	require.GreaterOrEqual(t, len(md.deserializers), 5)
}

func TestMultiplexDeserializer_ThreadSafeCopyDeserializers(t *testing.T) {
	t.Parallel()

	md := NewMultiplexDeserializer()

	// Add some deserializers
	des1 := &mock.Deserializer{}
	des2 := &mock.Deserializer{}
	md.AddDeserializer(des1)
	md.AddDeserializer(des2)

	// Get a copy
	copy1 := md.threadSafeCopyDeserializers()
	require.Len(t, copy1, 2)

	// Add another deserializer
	des3 := &mock.Deserializer{}
	md.AddDeserializer(des3)

	// Get another copy
	copy2 := md.threadSafeCopyDeserializers()
	require.Len(t, copy2, 3)

	require.Len(t, copy1, 2)

	// Verify the copies are independent
	copy1[0] = nil
	require.NotNil(t, md.deserializers[0], "modifying copy should not affect original")
}

func TestNewDeserializer(t *testing.T) {
	t.Parallel()

	des, err := NewDeserializer()

	require.NoError(t, err)
	require.NotNil(t, des)

	// Verify it's a MultiplexDeserializer
	md, ok := des.(*MultiplexDeserializer)
	require.True(t, ok, "NewDeserializer should return a MultiplexDeserializer")

	// Verify it has at least one deserializer (x509.Deserializer)
	require.NotEmpty(t, md.deserializers, "NewDeserializer should add x509.Deserializer by default")
}

func TestMultiplexDeserializer_ReturnsFirstSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		testFunc func(*MultiplexDeserializer) (interface{}, error)
		setup    func() []Deserializer
	}{
		{
			name: "DeserializeVerifier returns first success",
			testFunc: func(md *MultiplexDeserializer) (interface{}, error) {
				return md.DeserializeVerifier([]byte("test"))
			},
			setup: func() []Deserializer {
				verifier1 := &mock.Verifier{}
				des1 := &mock.Deserializer{}
				des1.DeserializeVerifierReturns(verifier1, nil)

				verifier2 := &mock.Verifier{}
				des2 := &mock.Deserializer{}
				des2.DeserializeVerifierReturns(verifier2, nil)

				return []Deserializer{des1, des2}
			},
		},
		{
			name: "DeserializeSigner returns first success",
			testFunc: func(md *MultiplexDeserializer) (interface{}, error) {
				return md.DeserializeSigner([]byte("test"))
			},
			setup: func() []Deserializer {
				signer1 := &mock.Signer{}
				des1 := &mock.Deserializer{}
				des1.DeserializeSignerReturns(signer1, nil)

				signer2 := &mock.Signer{}
				des2 := &mock.Deserializer{}
				des2.DeserializeSignerReturns(signer2, nil)

				return []Deserializer{des1, des2}
			},
		},
		{
			name: "Info returns first success",
			testFunc: func(md *MultiplexDeserializer) (interface{}, error) {
				return md.Info([]byte("test"), []byte("audit"))
			},
			setup: func() []Deserializer {
				des1 := &mock.Deserializer{}
				des1.InfoReturns("first-info", nil)

				des2 := &mock.Deserializer{}
				des2.InfoReturns("second-info", nil)

				return []Deserializer{des1, des2}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			md := NewMultiplexDeserializer()
			deserializers := tc.setup()
			for _, des := range deserializers {
				md.AddDeserializer(des)
			}

			result, err := tc.testFunc(md)

			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify only the first deserializer was called
			mockDes1 := deserializers[0].(*mock.Deserializer)
			mockDes2 := deserializers[1].(*mock.Deserializer)

			// First deserializer should be called
			require.Greater(t, mockDes1.DeserializeVerifierCallCount()+
				mockDes1.DeserializeSignerCallCount()+
				mockDes1.InfoCallCount(), 0)

			// Second deserializer should not be called
			require.Equal(t, 0, mockDes2.DeserializeVerifierCallCount()+
				mockDes2.DeserializeSignerCallCount()+
				mockDes2.InfoCallCount())
		})
	}
}
