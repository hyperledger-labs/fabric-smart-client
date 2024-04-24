/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"strings"

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
	ons, err := orion.GetDefaultONS(context)
	assert.NoError(err)
	me := ons.IdentityManager().Me()

	tx, err := otx.NewTransaction(context, me, ons.Name())
	assert.NoError(err, "failed creating orion transaction")
	tx.SetNamespace("cars") // Sets the namespace where the state should be stored

	tx.AddMustSignUser(v.Buyer)

	carRecord := &states.CarRecord{
		Owner:           me,
		CarRegistration: v.CarRegistration,
	}
	carKey := carRecord.Key()

	recordBytes, err := tx.Get(carKey)
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

	carRec.Owner = v.Buyer
	recordBytes, err = json.Marshal(carRec)
	if err != nil {
		return nil, errors.Wrapf(err, "error marshaling car record: %s", carRecord)
	}

	err = tx.Put(carKey, recordBytes,
		&types.AccessControl{
			ReadWriteUsers:     otx.UsersMap("dmv", v.Buyer),
			SignPolicyForWrite: types.AccessControl_ALL,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "error during put")
	}

	env, err := tx.SignAndClose()
	if err != nil {
		return nil, errors.Wrap(err, "error during sign and close")
	}

	buyerSession, err := session.NewJSON(context, v, view2.GetIdentityProvider(context).Identity(v.Buyer))
	assert.NoError(err, "failed getting session with %s", v.Buyer)
	assert.NoError(buyerSession.Send(env), "failed sending envelope")

	// receive ack from buyer
	var ack string
	assert.NoError(buyerSession.Receive(&ack), "failed receiving ack from buyer")

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
	ons, err := orion.GetDefaultONS(context)
	assert.NoError(err)
	me := ons.IdentityManager().Me()

	sellerSeesion := session.JSON(context)
	var env []byte
	assert.NoError(sellerSeesion.Receive(&env), "failed receiving envelope")

	loadedTx, err := otx.NewLoadedTransaction(context, me, ons.Name(), "cars", env)
	assert.NoError(err, "failed creating orion loaded transaction")

	if err = buyerValidateTransaction(loadedTx, me, "dmv"); err != nil {
		return nil, errors.Wrap(err, "error during buyer validate transaction")
	}

	env, err = loadedTx.CoSignAndClose()
	if err != nil {
		return nil, errors.Wrap(err, "error during co-sign and close")
	}

	dmvSession, err := session.NewJSON(context, f, view2.GetIdentityProvider(context).Identity("dmv"))
	assert.NoError(err, "failed getting session with dmv")
	assert.NoError(dmvSession.Send(env), "failed sending envelope")

	// receive ack from dmv
	var ack string
	assert.NoError(dmvSession.Receive(&ack), "failed receiving ack from dmv")

	// send ack to seller
	assert.NoError(sellerSeesion.Send("ack"), "failed sending ack to seller")

	return nil, nil
}

func buyerValidateTransaction(loadedTx *otx.LoadedTransaction, buyerID, dmvID string) error {
	newCarRec := &states.CarRecord{}
	var newCarACL *types.AccessControl
	writes := loadedTx.Writes()
	for _, dw := range writes {
		switch {
		case strings.HasPrefix(dw.Key, states.CarRecordKeyPrefix):
			if err := json.Unmarshal(dw.Value, newCarRec); err != nil {
				return err
			}
			newCarACL = dw.Acl.(*types.AccessControl)
		default:
			return errors.Errorf("unexpected write key: %s", dw.Key)
		}
	}
	if newCarRec.Owner != buyerID {
		return errors.Errorf("car new owner %s is not the buyer %s", newCarRec.Owner, buyerID)
	}
	if !newCarACL.ReadWriteUsers[newCarRec.Owner] || !newCarACL.ReadWriteUsers[dmvID] ||
		len(newCarACL.ReadWriteUsers) != 2 || len(newCarACL.ReadUsers) != 0 ||
		newCarACL.SignPolicyForWrite != types.AccessControl_ALL {
		return errors.New("car new ACL is wrong")
	}

	signedUsers := loadedTx.SignedUsers()
	if len(signedUsers) != 1 {
		return errors.Errorf("unexpected length of signed users: %d", len(signedUsers))
	}
	sellerID := signedUsers[0]

	mustSignUsers := loadedTx.MustSignUsers()
	hasBuyer := false
	hasSeller := false
	for _, u := range mustSignUsers {
		if u == buyerID {
			hasBuyer = true
		}
		if u == sellerID {
			hasSeller = true
		}
	}
	if !hasBuyer {
		return errors.New("car buyer is not in must-sign-users")
	}
	if !hasSeller {
		return errors.New("car seller is not in must-sign-users")
	}
	return nil
}

type DMVFlow struct {
}

func (f *DMVFlow) Call(context view.Context) (interface{}, error) {
	ons, err := orion.GetDefaultONS(context)
	assert.NoError(err)
	me := ons.IdentityManager().Me()

	buyerSession := session.JSON(context)
	var env []byte
	assert.NoError(buyerSession.Receive(&env), "failed receiving envelope")

	loadedTx, err := otx.NewLoadedTransaction(context, me, ons.Name(), "cars", env)
	assert.NoError(err, "failed creating orion loaded transaction")

	tx, err := otx.NewTransaction(context, me, ons.Name())
	assert.NoError(err, "failed creating orion transaction")
	tx.SetNamespace("cars") // Sets the namespace where the state should be stored

	if err = dmvValidateTransaction(tx, loadedTx, me); err != nil {
		return nil, errors.Wrap(err, "error during buyer validate transaction")
	}

	if err = loadedTx.Commit(); err != nil {
		return nil, errors.Wrap(err, "error during commit")
	}

	assert.NoError(buyerSession.Send("ack"), "failed sending ack")

	return nil, nil
}

func dmvValidateTransaction(tx *otx.Transaction, loadedTx *otx.LoadedTransaction, dmvID string) error {
	carRec := &states.CarRecord{}
	reads := loadedTx.Reads()
	for _, dr := range reads {
		recordBytes, err := tx.Get(dr.Key)
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(dr.Key, states.CarRecordKeyPrefix):
			if err = json.Unmarshal(recordBytes, carRec); err != nil {
				return err
			}
		default:
			return errors.Errorf("unexpected read key: %s", dr.Key)
		}
	}

	newCarRec := &states.CarRecord{}
	var newCarACL *types.AccessControl
	writes := loadedTx.Writes()
	for _, dw := range writes {
		switch {
		case strings.HasPrefix(dw.Key, states.CarRecordKeyPrefix):
			if err := json.Unmarshal(dw.Value, newCarRec); err != nil {
				return err
			}
			newCarACL = dw.Acl.(*types.AccessControl)
		default:
			return errors.Errorf("unexpected write key: %s", dw.Key)
		}
	}

	mustSignUsers := loadedTx.MustSignUsers()
	signedUsers := loadedTx.SignedUsers()

	hasSeller := false
	hasBuyer := false
	for _, u := range mustSignUsers {
		if u == carRec.Owner {
			hasSeller = true
		}
		if u == newCarRec.Owner {
			hasBuyer = true
		}
	}
	if !hasBuyer {
		return errors.New("car buyer is not in must-sign-users")
	}
	if !hasSeller {
		return errors.New("car seller is not in must-sign-users")
	}

	hasSeller = false
	hasBuyer = false
	for _, u := range signedUsers {
		if u == carRec.Owner {
			hasSeller = true
		}
		if u == newCarRec.Owner {
			hasBuyer = true
		}
	}
	if !hasBuyer {
		return errors.New("car buyer is not in signed-users")
	}
	if !hasSeller {
		return errors.New("car seller is not in signed-users")
	}

	if newCarRec.CarRegistration != carRec.CarRegistration {
		return errors.New("car registration changed")
	}

	if !newCarACL.ReadWriteUsers[newCarRec.Owner] || !newCarACL.ReadWriteUsers[dmvID] ||
		len(newCarACL.ReadWriteUsers) != 2 || len(newCarACL.ReadUsers) != 0 ||
		newCarACL.SignPolicyForWrite != types.AccessControl_ALL {
		return errors.New("car new ACL is wrong")
	}

	return nil
}
