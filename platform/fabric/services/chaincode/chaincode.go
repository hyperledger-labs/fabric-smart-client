/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Endorse interface {
	WithInvokerIdentity(identity view.Identity) Endorse
	WithTransientEntry(k string, v interface{})
	WithEndorsersByMSPIDs(ds ...string)
	WithEndorsersFromMyOrg()
	WithTxID(txID fabric.TxID) Endorse
	Call() (*fabric.Envelope, error)
	WithNumRetries(retries uint)
	WithRetrySleep(sleep time.Duration)
}

type Query interface {
	WithInvokerIdentity(identity view.Identity) Query
	WithTransientEntry(k string, v interface{})
	WithEndorsersByMSPIDs(ds ...string)
	WithEndorsersFromMyOrg()
	WithMatchEndorsementPolicy()
	Call() ([]byte, error)
	WithNumRetries(retries uint)
	WithRetrySleep(sleep time.Duration)
}

type Chaincode interface {
	IsPrivate() bool
	Endorse(function string, args ...interface{}) Endorse
	Query(function string, args ...interface{}) Query
}

type stdEndorse struct {
	che *fabric.ChaincodeEndorse
}

func (s *stdEndorse) WithTxID(txID fabric.TxID) Endorse {
	s.che.WithTxID(txID)
	return s
}

func (s *stdEndorse) Call() (*fabric.Envelope, error) {
	return s.che.Call()
}

func (s *stdEndorse) WithInvokerIdentity(identity view.Identity) Endorse {
	s.che.WithInvokerIdentity(identity)
	return s
}

func (s *stdEndorse) WithTransientEntry(k string, v interface{}) {
	s.che.WithTransientEntry(k, v)
}

func (s *stdEndorse) WithEndorsersByMSPIDs(ds ...string) {
	s.che.WithEndorsersByMSPIDs(ds...)
}

func (s *stdEndorse) WithEndorsersFromMyOrg() {
	s.che.WithEndorsersFromMyOrg()
}

func (s *stdEndorse) WithNumRetries(numRetries uint) {
	s.che.WithNumRetries(numRetries)
}

func (s *stdEndorse) WithRetrySleep(duration time.Duration) {
	s.che.WithRetrySleep(duration)
}

type stdQuery struct {
	chq *fabric.ChaincodeQuery
}

func (s *stdQuery) Call() ([]byte, error) {
	return s.chq.Call()
}

func (s *stdQuery) WithInvokerIdentity(identity view.Identity) Query {
	s.chq.WithInvokerIdentity(identity)
	return s
}

func (s *stdQuery) WithTransientEntry(k string, v interface{}) {
	s.chq.WithTransientEntry(k, v)
}

func (s *stdQuery) WithEndorsersByMSPIDs(ds ...string) {
	s.chq.WithEndorsersByMSPIDs(ds...)
}

func (s *stdQuery) WithEndorsersFromMyOrg() {
	s.chq.WithEndorsersFromMyOrg()
}

func (s *stdQuery) WithMatchEndorsementPolicy() {
	s.chq.WithMatchEndorsementPolicy()
}

func (s *stdQuery) WithNumRetries(numRetries uint) {
	s.chq.WithNumRetries(numRetries)
}

func (s *stdQuery) WithRetrySleep(duration time.Duration) {
	s.chq.WithRetrySleep(duration)
}

type stdChaincode struct {
	ch *fabric.Chaincode
}

func (s *stdChaincode) IsPrivate() bool {
	return s.ch.IsPrivate()
}

func (s *stdChaincode) Endorse(function string, args ...interface{}) Endorse {
	return &stdEndorse{che: s.ch.Endorse(function, args...)}
}

func (s *stdChaincode) Query(function string, args ...interface{}) Query {
	return &stdQuery{chq: s.ch.Query(function, args...)}
}

type fpcEndorse struct {
	che *fpc.ChaincodeEndorse
}

func (s *fpcEndorse) Call() (*fabric.Envelope, error) {
	return s.che.Call()
}

func (s *fpcEndorse) WithTxID(txID fabric.TxID) Endorse {
	s.che.WithTxID(txID)
	return s
}

func (s *fpcEndorse) WithInvokerIdentity(identity view.Identity) Endorse {
	s.che.WithSignerIdentity(identity)
	return s
}

func (s *fpcEndorse) WithTransientEntry(k string, v interface{}) {
	s.che.WithTransientEntry(k, v)
}

func (s *fpcEndorse) WithEndorsersByMSPIDs(ds ...string) {
	s.che.WithEndorsersByMSPIDs(ds...)
}

func (s *fpcEndorse) WithEndorsersFromMyOrg() {
	s.che.WithEndorsersFromMyOrg()
}

func (s *fpcEndorse) WithNumRetries(numRetries uint) {
}

func (s *fpcEndorse) WithRetrySleep(duration time.Duration) {
}

type fpcQuery struct {
	chq *fpc.ChaincodeQuery
}

func (s *fpcQuery) Call() ([]byte, error) {
	return s.chq.Call()
}

func (s *fpcQuery) WithInvokerIdentity(identity view.Identity) Query {
	s.chq.WithSignerIdentity(identity)
	return s
}

func (s *fpcQuery) WithTransientEntry(k string, v interface{}) {
	s.chq.WithTransientEntry(k, v)
}

func (s *fpcQuery) WithEndorsersByMSPIDs(ds ...string) {
	s.chq.WithEndorsersByMSPIDs(ds...)
}

func (s *fpcQuery) WithEndorsersFromMyOrg() {
	s.chq.WithEndorsersFromMyOrg()
}

func (s *fpcQuery) WithMatchEndorsementPolicy() {
	s.chq.WithMatchEndorsementPolicy()
}

func (s *fpcQuery) WithNumRetries(numRetries uint) {
}

func (s *fpcQuery) WithRetrySleep(duration time.Duration) {
}

type fpcChaincode struct {
	ch *fpc.Chaincode
}

func (s *fpcChaincode) IsPrivate() bool {
	return s.ch.IsPrivate()
}

func (s *fpcChaincode) Endorse(function string, args ...interface{}) Endorse {
	return &fpcEndorse{che: s.ch.Endorse(function, args...)}
}

func (s *fpcChaincode) Query(function string, args ...interface{}) Query {
	return &fpcQuery{chq: s.ch.Query(function, args...)}
}
