/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Envelope struct {
	e driver.Envelope
}

func (e *Envelope) Bytes() ([]byte, error) {
	return e.e.Bytes()
}

func (e *Envelope) FromBytes(raw []byte) error {
	return e.e.FromBytes(raw)
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

func (e *Envelope) String() string {
	return e.e.String()
}

type Chaincode struct {
	chaincode     driver.Chaincode
	fns           driver.FabricNetworkService
	EventListener *EventListener
}

// Invoke returns a chaincode invocation proxy struct for the passed function and arguments
func (c *Chaincode) Invoke(function string, args ...interface{}) *ChaincodeInvocation {
	ci := &ChaincodeInvocation{ChaincodeInvocation: c.chaincode.NewInvocation(function, args...)}
	ci.WithInvokerIdentity(c.fns.LocalMembership().DefaultIdentity())
	return ci
}

func (c *Chaincode) Query(function string, args ...interface{}) *ChaincodeQuery {
	ci := &ChaincodeQuery{ChaincodeInvocation: c.chaincode.NewInvocation(function, args...)}
	ci.WithInvokerIdentity(c.fns.LocalMembership().DefaultIdentity())
	return ci
}

func (c *Chaincode) Endorse(function string, args ...interface{}) *ChaincodeEndorse {
	ci := &ChaincodeEndorse{ChaincodeInvocation: c.chaincode.NewInvocation(function, args...)}
	ci.WithInvokerIdentity(c.fns.LocalMembership().DefaultIdentity())
	return ci
}

func (c *Chaincode) Discover() *ChaincodeDiscover {
	return &ChaincodeDiscover{ChaincodeDiscover: c.chaincode.NewDiscover()}
}

func (c *Chaincode) IsAvailable() (bool, error) {
	return c.chaincode.IsAvailable()
}

func (c *Chaincode) IsPrivate() bool {
	return c.chaincode.IsPrivate()
}

// Version returns the version of this chaincode.
// It returns an error if a failure happens during the computation.
func (c *Chaincode) Version() (string, error) {
	return c.chaincode.Version()
}

// DiscoveredPeer contains the information of a discovered peer
type DiscoveredPeer = driver.DiscoveredPeer

// DiscoveredIdentities extract the identities of the discovered peers
func DiscoveredIdentities(d []DiscoveredPeer) []view.Identity {
	// return all identities
	var ids []view.Identity
	for _, p := range d {
		ids = append(ids, p.Identity)
	}
	return ids
}

type ChaincodeDiscover struct {
	driver.ChaincodeDiscover
}

// Call invokes discovery service and returns the discovered peers
func (i *ChaincodeDiscover) Call() ([]DiscoveredPeer, error) {
	return i.ChaincodeDiscover.Call()
}

func (i *ChaincodeDiscover) WithFilterByMSPIDs(mspIDs ...string) *ChaincodeDiscover {
	i.ChaincodeDiscover.WithFilterByMSPIDs(mspIDs...)
	return i
}

type ChaincodeInvocation struct {
	driver.ChaincodeInvocation
}

// Call invokes the chaincode function with the passed arguments, and any additional parameter set with
// the other functions on this struct.
// It no error occurs, it returns the transaction id and the response payload of the chaincode invocation.
func (i *ChaincodeInvocation) Call() (string, []byte, error) {
	return i.ChaincodeInvocation.Submit()
}

func (i *ChaincodeInvocation) WithContext(context context.Context) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithContext(context)
	return i
}

func (i *ChaincodeInvocation) WithTransientEntry(k string, v interface{}) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithTransientEntry(k, v)
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

// WithNumRetries sets the number of times the chaincode operation should be retried before returning a failure
func (i *ChaincodeInvocation) WithNumRetries(numRetries uint) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithNumRetries(numRetries)
	return i
}

// WithRetrySleep sets the time interval between each retry
func (i *ChaincodeInvocation) WithRetrySleep(duration time.Duration) *ChaincodeInvocation {
	i.ChaincodeInvocation.WithRetrySleep(duration)
	return i
}

type ChaincodeQuery struct {
	driver.ChaincodeInvocation
}

func (i *ChaincodeQuery) Call() ([]byte, error) {
	return i.ChaincodeInvocation.Query()
}

func (i *ChaincodeQuery) WithContext(context context.Context) *ChaincodeQuery {
	i.ChaincodeInvocation.WithContext(context)
	return i
}

func (i *ChaincodeQuery) WithTransientEntry(k string, v interface{}) *ChaincodeQuery {
	i.ChaincodeInvocation.WithTransientEntry(k, v)
	return i
}

// WithDiscoveredEndorsersByEndpoints sets the endpoints to be used to filter the result of
// discovery. Discovery is used to identify the chaincode's endorsers, if not set otherwise.
func (i *ChaincodeQuery) WithDiscoveredEndorsersByEndpoints(endpoints ...string) *ChaincodeQuery {
	i.ChaincodeInvocation.WithDiscoveredEndorsersByEndpoints(endpoints...)
	return i
}

func (i *ChaincodeQuery) WithEndorsersByMSPIDs(mspIDs ...string) *ChaincodeQuery {
	i.ChaincodeInvocation.WithEndorsersByMSPIDs(mspIDs...)
	return i
}

func (i *ChaincodeQuery) WithEndorsersFromMyOrg() *ChaincodeQuery {
	i.ChaincodeInvocation.WithEndorsersFromMyOrg()
	return i
}

func (i *ChaincodeQuery) WithInvokerIdentity(id view.Identity) *ChaincodeQuery {
	i.ChaincodeInvocation.WithSignerIdentity(id)
	return i
}

func (i *ChaincodeQuery) WithTxID(id TxID) *ChaincodeQuery {
	i.ChaincodeInvocation.WithTxID(driver.TxID{
		Nonce:   id.Nonce,
		Creator: id.Creator,
	})
	return i
}

// WithMatchEndorsementPolicy enforces that the query is perfomed against a set of peers that satisfy the
// endorsement policy of the chaincode
func (i *ChaincodeQuery) WithMatchEndorsementPolicy() *ChaincodeQuery {
	i.ChaincodeInvocation.WithMatchEndorsementPolicy()
	return i
}

// WithNumRetries sets the number of times the chaincode operation should be retried before returning a failure
func (i *ChaincodeQuery) WithNumRetries(numRetries uint) *ChaincodeQuery {
	i.ChaincodeInvocation.WithNumRetries(numRetries)
	return i
}

// WithRetrySleep sets the time interval between each retry
func (i *ChaincodeQuery) WithRetrySleep(duration time.Duration) *ChaincodeQuery {
	i.ChaincodeInvocation.WithRetrySleep(duration)
	return i
}

type ChaincodeEndorse struct {
	ChaincodeInvocation driver.ChaincodeInvocation
}

func (i *ChaincodeEndorse) Call() (*Envelope, error) {
	env, err := i.ChaincodeInvocation.Endorse()
	if err != nil {
		return nil, err
	}
	return &Envelope{e: env}, nil
}

func (i *ChaincodeEndorse) WithContext(context context.Context) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithContext(context)
	return i
}

func (i *ChaincodeEndorse) WithTransientEntry(k string, v interface{}) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithTransientEntry(k, v)
	return i
}

func (i *ChaincodeEndorse) WithEndorsersByMSPIDs(mspIDs ...string) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithEndorsersByMSPIDs(mspIDs...)
	return i
}

func (i *ChaincodeEndorse) WithEndorsersFromMyOrg() *ChaincodeEndorse {
	i.ChaincodeInvocation.WithEndorsersFromMyOrg()
	return i
}

func (i *ChaincodeEndorse) WithInvokerIdentity(id view.Identity) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithSignerIdentity(id)
	return i
}

func (i *ChaincodeEndorse) WithTxID(id TxID) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithTxID(driver.TxID{
		Nonce:   id.Nonce,
		Creator: id.Creator,
	})
	return i
}

func (i *ChaincodeEndorse) WithImplicitCollections(mspIDs ...string) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithImplicitCollections(mspIDs...)
	return i
}

// WithNumRetries sets the number of times the chaincode operation should be retried before returning a failure
func (i *ChaincodeEndorse) WithNumRetries(numRetries uint) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithNumRetries(numRetries)
	return i
}

// WithRetrySleep sets the time interval between each retry
func (i *ChaincodeEndorse) WithRetrySleep(duration time.Duration) *ChaincodeEndorse {
	i.ChaincodeInvocation.WithRetrySleep(duration)
	return i
}
