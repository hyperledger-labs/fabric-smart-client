/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Envelope struct {
	e driver.Envelope
}

func (e *Envelope) Bytes() ([]byte, error) {
	return e.e.Bytes()
}

func (e *Envelope) Results() []byte {
	return e.e.Results()
}

func (e *Envelope) TxID() string {
	return e.e.TxID()
}

func (e *Envelope) Nonce() []byte {
	return e.e.Nonce()
}

func (e *Envelope) Creator() []byte {
	return e.e.Creator()
}

func (e *Envelope) MarshalJSON() ([]byte, error) {
	raw, err := e.e.Bytes()
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

func (e *Envelope) UnmarshalJSON(raw []byte) error {
	var r []byte
	err := json.Unmarshal(raw, &r)
	if err != nil {
		return err
	}
	return e.e.FromBytes(r)
}

type Chaincode struct {
	chaincode driver.Chaincode
	fns       driver.FabricNetworkService
}

func (c *Chaincode) Invoke(function string, args ...interface{}) *ChaincodeInvocation {
	ci := &ChaincodeInvocation{ChaincodeInvocation: c.chaincode.NewInvocation(driver.ChaincodeInvoke, function, args...)}
	ci.WithInvokerIdentity(c.fns.LocalMembership().DefaultIdentity())
	return ci
}

func (c *Chaincode) Query(function string, args ...interface{}) *ChaincodeInvocation {
	ci := &ChaincodeInvocation{ChaincodeInvocation: c.chaincode.NewInvocation(driver.ChaincodeQuery, function, args...)}
	ci.WithInvokerIdentity(c.fns.LocalMembership().DefaultIdentity())
	return ci
}

func (c *Chaincode) Endorse(function string, args ...interface{}) *ChaincodeEndorse {
	ci := &ChaincodeEndorse{ci: c.chaincode.NewInvocation(driver.ChaincodeEndorse, function, args...)}
	ci.WithInvokerIdentity(c.fns.LocalMembership().DefaultIdentity())
	return ci
}

func (c *Chaincode) Discover() *ChaincodeDiscover {
	return &ChaincodeDiscover{ChaincodeDiscover: c.chaincode.NewDiscover()}
}

type ChaincodeDiscover struct {
	driver.ChaincodeDiscover
}

func (i *ChaincodeDiscover) Call() ([]view.Identity, error) {
	return i.ChaincodeDiscover.Call()
}

func (i *ChaincodeDiscover) WithFilterByMSPIDs(mspIDs ...string) *ChaincodeDiscover {
	i.ChaincodeDiscover.WithFilterByMSPIDs(mspIDs...)
	return i
}

type ChaincodeInvocation struct {
	driver.ChaincodeInvocation
}

func (i *ChaincodeInvocation) Call() (interface{}, error) {
	return i.ChaincodeInvocation.Call()
}

func (i *ChaincodeInvocation) WithTransientEntry(k string, v interface{}) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithTransientEntry(k, v)
	return i
}

func (i *ChaincodeInvocation) WithEndorsers(ids ...view.Identity) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithEndorsers(ids...)
	return i
}

func (i *ChaincodeInvocation) WithEndorsersByMSPIDs(mspIDs ...string) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithEndorsersByMSPIDs(mspIDs...)
	return i
}

func (i *ChaincodeInvocation) WithEndorsersFromMyOrg() *ChaincodeInvocation {
	i.ChaincodeInvocation.WithEndorsersFromMyOrg()
	return i
}

func (i *ChaincodeInvocation) WithInvokerIdentity(id view.Identity) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithSignerIdentity(id)
	return i
}

type ChaincodeEndorse struct {
	ci driver.ChaincodeInvocation
}

func (i *ChaincodeEndorse) Call() (*Envelope, error) {
	envBoxed, err := i.ci.Call()
	if err != nil {
		return nil, err
	}
	env, ok := envBoxed.(driver.Envelope)
	if !ok {
		panic("programming error")
	}
	return &Envelope{e: env}, nil
}

func (i *ChaincodeEndorse) WithTransientEntry(k string, v interface{}) *ChaincodeEndorse {
	i.ci.WithTransientEntry(k, v)
	return i
}

func (i *ChaincodeEndorse) WithEndorsers(ids ...view.Identity) *ChaincodeEndorse {
	i.ci.WithEndorsers(ids...)
	return i
}

func (i *ChaincodeEndorse) WithEndorsersByMSPIDs(mspIDs ...string) *ChaincodeEndorse {
	i.ci.WithEndorsersByMSPIDs(mspIDs...)
	return i
}

func (i *ChaincodeEndorse) WithEndorsersFromMyOrg() *ChaincodeEndorse {
	i.ci.WithEndorsersFromMyOrg()
	return i
}

func (i *ChaincodeEndorse) WithInvokerIdentity(id view.Identity) *ChaincodeEndorse {
	i.ci.WithSignerIdentity(id)
	return i
}

func (i *ChaincodeEndorse) WithTxID(id TxID) *ChaincodeEndorse {
	i.ci.WithTxID(driver.TxID{
		Nonce:   id.Nonce,
		Creator: id.Creator,
	})
	return i
}
