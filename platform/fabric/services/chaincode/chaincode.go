/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Endorse interface {
	WithInvokerIdentity(identity view.Identity) Endorse
	WithTransientEntry(k string, v interface{}) error
	WithEndorsersByMSPIDs(ds ...string)
	WithEndorsersFromMyOrg()
	WithTxID(txID fabric.TxID) Endorse
	Call() (*fabric.Envelope, error)
	WithNumRetries(retries uint)
	WithRetrySleep(sleep time.Duration)
}

type Query interface {
	WithInvokerIdentity(identity view.Identity) Query
	WithTransientEntry(k string, v interface{}) error
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

func (s *stdEndorse) WithTransientEntry(k string, v interface{}) error {
	_, err := s.che.WithTransientEntry(k, v)
	return err
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

func (s *stdQuery) WithTransientEntry(k string, v interface{}) error {
	_, err := s.chq.WithTransientEntry(k, v)
	return err
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
