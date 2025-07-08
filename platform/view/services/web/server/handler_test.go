/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	mocks2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/mocks"
	"github.com/stretchr/testify/require"
)

type Fruit struct {
	Name     string
	Quantity int
}

type FruitBasket struct {
	Fruits []string
}

func TestHttpHandler(t *testing.T) {
	h := web2.NewHttpHandler()

	rh := &mocks2.FakeRequestHandler{}
	rh.HandleRequestStub = func(ctx *web2.ReqContext) (interface{}, int) {
		query := ctx.Query.(*Fruit)

		var res FruitBasket
		for i := 0; i < query.Quantity; i++ {
			res.Fruits = append(res.Fruits, query.Name)
		}

		require.Equal(t, ctx.Vars["Fruit"], "pineapple")

		return res, 200
	}

	rh.ParsePayloadStub = func(payload []byte) (interface{}, error) {
		var f Fruit
		err := json.Unmarshal(payload, &f)
		require.NoError(t, err)
		return &f, nil
	}

	h.RegisterURI("/test/{Fruit}", "PUT", rh)

	resp := httptest.NewRecorder()
	pineappleRequest := bytes.NewBufferString(`{"Name": "pineapple", "Quantity": 3}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/test/pineapple", pineappleRequest)
	h.ServeHTTP(resp, req)

	expectedPineappleResponse := FruitBasket{Fruits: []string{"pineapple", "pineapple", "pineapple"}}
	var actualResponse FruitBasket
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &actualResponse))
	require.Equal(t, expectedPineappleResponse, actualResponse)
}
