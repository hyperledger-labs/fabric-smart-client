/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/services/otx"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MintRequest struct {
	CarRegistration string
}

type MintRequestView struct {
	*MintRequest
}

func (p *MintRequestView) Call(context view.Context) (interface{}, error) {
	request := p.prepareRequest(context)
	p.askApproval(context, request)

	return nil, nil
}

func (p *MintRequestView) prepareRequest(context view.Context) *states.MintRequestRecord {
	me := orion.GetDefaultONS(context).IdentityManager().Me()

	tx, err := otx.NewTransaction(context, me)
	assert.NoError(err, "failed creating orion transaction")

	// Sets the namespace where the state should be stored
	tx.SetNamespace("cars")

	record := &states.MintRequestRecord{
		Dealer:          me,
		CarRegistration: p.CarRegistration,
	}
	key := record.Key()

	recordBytes, _, err := tx.Get(key)
	assert.NoError(err, "error getting MintRequest: %s", key)
	assert.True(len(recordBytes) == 0, recordBytes, "MintRequest already exists: %s", key)

	recordBytes, err = json.Marshal(record)
	assert.NoError(err, "failed marshalling record")
	err = tx.Put(key, recordBytes,
		&types.AccessControl{
			ReadUsers:      otx.UsersMap("dmv"),
			ReadWriteUsers: otx.UsersMap(me),
		},
	)
	assert.NoError(err, "error during data transaction")

	_, err = context.RunView(otx.NewOrderingAndFinalityView(tx))
	assert.NoError(err, "failed ordering transaction")

	return record
}

func (p *MintRequestView) askApproval(context view.Context, request *states.MintRequestRecord) {
	commSession, err := session.NewJSON(context, p, view2.GetIdentityProvider(context).Identity("dmv"))
	assert.NoError(err, "failed getting session to dmf")
	assert.NoError(commSession.Send(request.Key()), "failed sending key")

	// Receive car record key
	var carKey string
	assert.NoError(commSession.Receive(&carKey), "failed receiving car record key")

	me := orion.GetDefaultONS(context).IdentityManager().Me()
	session, err := orion.GetDefaultONS(context).SessionManager().NewSession(me)
	assert.NoError(err, "failed getting orion session")
	qe, err := session.QueryExecutor("cars")
	assert.NoError(err, "failed query executor")

	carRecBytes, _, err := qe.Get(carKey)
	assert.NoError(err, "error getting car record, key: %s", carKey)

	assert.False(len(carRecBytes) == 0, "ListCar: executed, Car key: '%s',  Car record: %s\n", carKey, "not found")

	carRec := &states.CarRecord{}
	assert.NoError(json.Unmarshal(carRecBytes, carRec), "error unmarshaling data transaction value, key: %s", carKey)
	assert.Equal(request.CarRegistration, carRec.CarRegistration, "invalid car registration, got [%s]", carRec.CarRegistration)
	assert.Equal(me, carRec.Owner, "invalid car owner, got [%s]", carRec.Owner)
}

type MintRequestViewFactory struct{}

func (i *MintRequestViewFactory) NewView(in []byte) (view.View, error) {
	f := &MintRequestView{}
	err := json.Unmarshal(in, &f.MintRequest)
	assert.NoError(err)
	return f, nil
}

type MintRequestApprovalFlow struct{}

func (m *MintRequestApprovalFlow) Call(context view.Context) (interface{}, error) {
	// Receive key
	s := session.JSON(context)
	var mintReqRecordKey string
	assert.NoError(s.Receive(&mintReqRecordKey), "failed receiving key")

	// Prepare transaction
	me := orion.GetDefaultONS(context).IdentityManager().Me()

	tx, err := otx.NewTransaction(context, me)
	assert.NoError(err, "failed creating orion transaction")

	// Sets the namespace where the state should be stored
	tx.SetNamespace("cars")

	mintReqRec := &states.MintRequestRecord{}

	recordBytes, _, err := tx.Get(mintReqRecordKey)
	if err != nil {
		return "", errors.Wrapf(err, "error getting MintRequest: %s", mintReqRecordKey)
	}
	if recordBytes == nil {
		return "", errors.Errorf("MintRequest not found: %s", mintReqRecordKey)
	}

	if err = json.Unmarshal(recordBytes, mintReqRec); err != nil {
		return "", errors.Wrapf(err, "error unmarshaling data transaction value, key: %s", mintReqRecordKey)
	}

	if err = m.validateMintRequest(mintReqRecordKey, mintReqRec); err != nil {
		return "", errors.WithMessage(err, "MintRequest validation failed")
	}

	carRecord := &states.CarRecord{
		Owner:           mintReqRec.Dealer,
		CarRegistration: mintReqRec.CarRegistration,
	}
	carKey := carRecord.Key()

	carRecordBytes, _, err := tx.Get(carKey)
	if err != nil {
		return "", errors.Wrapf(err, "error getting Car: %s", carKey)
	}
	if carRecordBytes != nil {
		return "", errors.Errorf("Car already exists: %s", carKey)
	}

	carRecordBytes, err = json.Marshal(carRecord)
	if err != nil {
		return "", errors.Wrapf(err, "error marshaling car record: %s", carRecord)
	}

	err = tx.Put(carKey, carRecordBytes,
		&types.AccessControl{
			ReadUsers:      otx.UsersMap(mintReqRec.Dealer),
			ReadWriteUsers: otx.UsersMap(me),
		},
	)
	if err != nil {
		return "", errors.Wrap(err, "error during data transaction")
	}

	_, err = context.RunView(otx.NewOrderingAndFinalityView(tx))
	assert.NoError(err, "failed ordering transaction")

	// Send car record identifier
	assert.NoError(s.Send(carKey), "failed sending car key")

	return nil, nil
}

// Any validation, including provenance
func (m *MintRequestApprovalFlow) validateMintRequest(mintReqRecordKey string, mintReqRec *states.MintRequestRecord) error {
	reqID := mintReqRecordKey[len(states.MintRequestRecordKeyPrefix):]
	if reqID != mintReqRec.RequestID() {
		return errors.Errorf("MintRequest content compromised: expected: %s != actual: %s", reqID, mintReqRec.RequestID())
	}
	return nil
}
