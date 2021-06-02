/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
)

func (c *channel) NewRWSet(txid string) (api.RWSet, error) {
	return c.vault.NewRWSet(txid)
}

func (c *channel) GetRWSet(txid string, rwset []byte) (api.RWSet, error) {
	return c.vault.GetRWSet(txid, rwset)
}

func (c *channel) GetEphemeralRWSet(rwset []byte) (api.RWSet, error) {
	return c.vault.InspectRWSet(rwset)
}

func (c *channel) NewQueryExecutor() (api.QueryExecutor, error) {
	return c.vault.NewQueryExecutor()
}

func (c *channel) GetBlockByNumber(number uint64) (api.Block, error) {
	res, err := c.Chaincode("qscc").NewInvocation(api.ChaincodeQuery, GetBlockByNumber, c.name, number).WithSignerIdentity(
		c.network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.network.Peers()...).Call()
	if err != nil {
		return nil, err
	}

	b, err := protoutil.UnmarshalBlock(res.([]byte))
	if err != nil {
		return nil, err
	}
	return &Block{Block: b}, nil
}

type Block struct {
	*common.Block
}

func (b *Block) DataAt(i int) []byte {
	return b.Data.Data[i]
}
