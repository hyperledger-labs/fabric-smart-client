/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func (c *channel) NewRWSet(txid string) (driver.RWSet, error) {
	return c.vault.NewRWSet(txid)
}

func (c *channel) GetRWSet(txid string, rwset []byte) (driver.RWSet, error) {
	return c.vault.GetRWSet(txid, rwset)
}

func (c *channel) GetEphemeralRWSet(rwset []byte) (driver.RWSet, error) {
	return c.vault.InspectRWSet(rwset)
}

func (c *channel) NewQueryExecutor() (driver.QueryExecutor, error) {
	return c.vault.NewQueryExecutor()
}

func (c *channel) GetBlockByNumber(number uint64) (driver.Block, error) {
	res, err := c.Chaincode("qscc").NewInvocation(driver.ChaincodeQuery, GetBlockByNumber, c.name, number).WithSignerIdentity(
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
