/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package states

import (
	"encoding/base64"
	"fmt"

	"github.com/hyperledger-labs/orion-server/pkg/crypto"
)

const (
	CarRecordKeyPrefix             = "car~"
	MintRequestRecordKeyPrefix     = "mint-request~"
	TransferToRecordKeyPrefix      = "transfer-to~"
	TransferReceiveRecordKeyPrefix = "transfer-receive~"
)

type MintRequestRecord struct {
	Dealer          string
	CarRegistration string
}

func (r *MintRequestRecord) Key() string {
	return MintRequestRecordKeyPrefix + r.RequestID()
}

func (r *MintRequestRecord) RequestID() string {
	str := r.Dealer + "_" + r.CarRegistration
	sha256Hash, _ := crypto.ComputeSHA256Hash([]byte(str))
	return base64.URLEncoding.EncodeToString(sha256Hash)
}

type CarRecord struct {
	Owner           string
	CarRegistration string
}

func (r *CarRecord) String() string {
	return fmt.Sprintf("{CarRegistration: %s, Owner: %s}", r.CarRegistration, r.Owner)
}

func (r *CarRecord) Key() string {
	return CarRecordKeyPrefix + r.CarRegistration
}

type TransferToRecord struct {
	Owner           string
	Buyer           string
	CarRegistration string
}

func (r *TransferToRecord) RequestID() string {
	str := r.Owner + "_" + r.Buyer + "_" + r.CarRegistration
	sha256Hash, _ := crypto.ComputeSHA256Hash([]byte(str))
	return base64.URLEncoding.EncodeToString(sha256Hash)
}

type TransferReceiveRecord struct {
	Buyer               string
	CarRegistration     string
	TransferToRecordKey string
}

func (r *TransferToRecord) Key() string {
	return TransferToRecordKeyPrefix + r.RequestID()
}

func (r *TransferReceiveRecord) RequestID() string {
	str := r.Buyer + "_" + r.CarRegistration + "_" + r.TransferToRecordKey
	sha256Hash, _ := crypto.ComputeSHA256Hash([]byte(str))
	return base64.URLEncoding.EncodeToString(sha256Hash)
}

func (r *TransferReceiveRecord) Key() string {
	return TransferReceiveRecordKeyPrefix + r.RequestID()
}
