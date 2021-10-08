/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ownable

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/script/api"
)

type InputStateValidator struct{}

func (s InputStateValidator) Validate(tx api.Transaction, index uint32) error {
	ownableState := &State{}

	_, scripts, err := tx.GetInputAt(int(index), ownableState)
	if err != nil {
		return err
	}
	if !IsScriptValid(scripts.Death) {
		return errors.Errorf("invalid death script, expected ownable state death script")
	}
	if len(ownableState.Owners) == 0 {
		return errors.Errorf("the state does not have owners")
	}

	for _, owner := range ownableState.Owners {
		err := tx.HasBeenSignedBy(owner)
		if err != nil {
			return err
		}
	}

	return nil
}

type OutputStateValidator struct{}

func (s OutputStateValidator) Validate(tx api.Transaction, index uint32) error {
	ownableState := &State{}

	scripts, err := tx.GetOutputAt(int(index), ownableState)
	if err != nil {
		return err
	}
	if !IsScriptValid(scripts.Birth) {
		return errors.Errorf("invalid birth script, expected ownable state birth script")
	}
	if len(ownableState.Owners) == 0 {
		return errors.Errorf("[ownable.birth] the state does not have owners")
	}

	for _, owner := range ownableState.Owners {
		err := tx.HasBeenSignedBy(owner)
		if err != nil {
			return err
		}
	}

	return nil
}
