/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/services/otx"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
)

type Transfer struct {
	Buyer           string
	CarRegistration string
}

type TransferView struct {
	*Transfer
}

func (v *TransferView) Call(context view.Context) (interface{}, error) {
	me := orion.GetDefaultONS(context).IdentityManager().Me()
	buyer := view2.GetIdentityProvider(context).Identity(v.Buyer).String()
	dmv := view2.GetIdentityProvider(context).Identity("dmv").String()

	tx, err := otx.NewTransaction(context, me, orion.GetDefaultONS(context).Name())
	assert.NoError(err, "failed creating orion transaction")
	tx.SetNamespace("cars") // Sets the namespace where the state should be stored

	tx.AddMustSignUser(buyer)

	carRecord := &states.CarRecord{
		Owner:           me,
		CarRegistration: v.CarRegistration,
	}
	carKey := carRecord.Key()

	recordBytes, _, err := tx.Get(carKey)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting car record, key: %s", carKey)
	}
	if recordBytes == nil {
		return nil, errors.Errorf("car record not found, key: %s", carKey)
	}
	carRec := &states.CarRecord{}
	if err = json.Unmarshal(recordBytes, carRec); err != nil {
		return nil, errors.Wrapf(err, "error unmarshaling car record bytes value, key: %s", carKey)
	}

	carRec.Owner = buyer
	recordBytes, err = json.Marshal(carRecord)
	if err != nil {
		return "", errors.Wrapf(err, "error marshaling car record: %s", carRecord)
	}

	err = tx.Put(carKey, recordBytes,
		&types.AccessControl{
			ReadWriteUsers:     otx.UsersMap(dmv, buyer),
			SignPolicyForWrite: types.AccessControl_ALL,
		},
	)
	if err != nil {
		return "", errors.Wrap(err, "error during put")
	}

	env, err := tx.SignAndClose()
	if err != nil {
		return "", errors.Wrap(err, "error during sign and close")
	}

	commSession, err := session.NewJSON(context, v, view2.GetIdentityProvider(context).Identity(v.Buyer))
	assert.NoError(err, "failed getting session with %s", v.Buyer)
	assert.NoError(commSession.Send(env), "failed sending envelope")

	return nil, nil
}

type TransferViewFactory struct{}

func (vf *TransferViewFactory) NewView(in []byte) (view.View, error) {
	v := &TransferView{}
	err := json.Unmarshal(in, &v.Transfer)
	assert.NoError(err)
	return v, nil
}

type BuyerFlow struct {
}

func (f *BuyerFlow) Call(context view.Context) (interface{}, error) {
	me := orion.GetDefaultONS(context).IdentityManager().Me()

	s := session.JSON(context)
	var env proto.Message
	assert.NoError(s.Receive(&env), "failed receiving envelope")

	loadedTx, err := otx.NewLoadedTransaction(context, me, orion.GetDefaultONS(context).Name(), env)
	assert.NoError(err, "failed creating orion loaded transaction")

	// TODO buyer inspect transaction

	env, err = loadedTx.CoSignAndClose()
	if err != nil {
		return "", errors.Wrap(err, "error during co-sign and close")
	}

	commSession, err := session.NewJSON(context, f, view2.GetIdentityProvider(context).Identity("dmv"))
	assert.NoError(err, "failed getting session with dmv")
	assert.NoError(commSession.Send(env), "failed sending envelope")
	assert.Equal(7,8)


	return nil, nil
}

type DMVFlow struct {
}

func (f *DMVFlow) Call(context view.Context) (interface{}, error) {
	me := orion.GetDefaultONS(context).IdentityManager().Me()

	s := session.JSON(context)
	var env proto.Message
	assert.NoError(s.Receive(&env), "failed receiving envelope")

	loadedTx, err := otx.NewLoadedTransaction(context, me, orion.GetDefaultONS(context).Name(), env)
	assert.NoError(err, "failed creating orion loaded transaction")

	// TODO dmv inspect transaction

	if loadedTx.Commit() != nil {
		return "", errors.Wrap(err, "error during commit")
	}

	return nil, nil
}
