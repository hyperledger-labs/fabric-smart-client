/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"context"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type RWSet struct {
	cdriver.RWSet
	NamespacesList []cdriver.Namespace
	DoneCalls      int
}

func (f *RWSet) Namespaces() []cdriver.Namespace {
	return append([]cdriver.Namespace(nil), f.NamespacesList...)
}

func (f *RWSet) Done() {
	f.DoneCalls++
}

type ProcessTransaction struct {
	IDValue      string
	NetworkValue string
	ChannelValue string
	Function     string
	Args         []string
}

func (f *ProcessTransaction) Network() string {
	return f.NetworkValue
}

func (f *ProcessTransaction) Channel() string {
	return f.ChannelValue
}

func (f *ProcessTransaction) ID() string {
	return f.IDValue
}

func (f *ProcessTransaction) FunctionAndParameters() (string, []string) {
	return f.Function, append([]string(nil), f.Args...)
}

type Transaction struct {
	fdriver.Transaction
	RWSetValue fdriver.RWSet
	RWSetErr   error
}

func (f *Transaction) GetRWSet() (fdriver.RWSet, error) {
	return f.RWSetValue, f.RWSetErr
}

type SignerSerializer struct {
	Serialized   []byte
	SerializeErr error
	Signature    []byte
	SignErr      error
}

func (f *SignerSerializer) Serialize() ([]byte, error) {
	return f.Serialized, f.SerializeErr
}

func (f *SignerSerializer) Sign([]byte) ([]byte, error) {
	return f.Signature, f.SignErr
}

type EnvelopeService struct {
	fdriver.EnvelopeService
	ExistsValue bool
	Envelope    []byte
	LoadErr     error
}

func (f *EnvelopeService) Exists(context.Context, string) bool {
	return f.ExistsValue
}

func (f *EnvelopeService) LoadEnvelope(context.Context, string) ([]byte, error) {
	return f.Envelope, f.LoadErr
}

type EndorserTransactionService struct {
	fdriver.EndorserTransactionService
	ExistsValue bool
	Transaction []byte
	LoadErr     error
}

func (f *EndorserTransactionService) Exists(context.Context, string) bool {
	return f.ExistsValue
}

func (f *EndorserTransactionService) LoadTransaction(context.Context, string) ([]byte, error) {
	return f.Transaction, f.LoadErr
}

type Channel struct {
	fdriver.Channel
	EnvelopeServiceValue    fdriver.EnvelopeService
	TransactionServiceValue fdriver.EndorserTransactionService
	RWSetLoaderValue        fdriver.RWSetLoader
}

func (f *Channel) EnvelopeService() fdriver.EnvelopeService {
	return f.EnvelopeServiceValue
}

func (f *Channel) TransactionService() fdriver.EndorserTransactionService {
	return f.TransactionServiceValue
}

func (f *Channel) RWSetLoader() fdriver.RWSetLoader {
	return f.RWSetLoaderValue
}

type ChannelProvider struct {
	ChannelValue fdriver.Channel
	ChannelErr   error
}

func (f *ChannelProvider) Channel(string) (fdriver.Channel, error) {
	return f.ChannelValue, f.ChannelErr
}
