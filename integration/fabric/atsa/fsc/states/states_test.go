/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package states

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
)

func TestJson(t *testing.T) {
	asset := &Asset{
		ObjectType:        "coin",
		ID:                "1234",
		Owner:             []byte("Alice"),
		PublicDescription: "Coin",
		PrivateProperties: []byte("Hello World!!!"),
	}

	o, err := json.MarshalIndent(asset, "", " ")
	assert.NoError(err)
	fmt.Println(string(o))
	hash := sha256.New()
	hash.Write([]byte("Hello World!!!"))
	rawHashed := hash.Sum(nil)
	asset.PrivateProperties = []byte(base64.StdEncoding.EncodeToString(rawHashed))
	o, err = json.MarshalIndent(asset, "", " ")
	assert.NoError(err)
	fmt.Println(string(o))
}
