/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package script

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/script/api"
	"github.com/pkg/errors"
)

type MultiplexStateValidator struct {
	InputStateValidators  map[string]api.StateValidator
	OutputStateValidators map[string]api.StateValidator
}

func (m MultiplexStateValidator) Validate(tx api.Transaction, index uint32) error {
	for index := range tx.Inputs() {
		scripts, err := tx.GetInputScriptsAt(index)
		if err != nil {
			return err
		}

		sv, ok := m.InputStateValidators[scripts.Death.Type]
		if !ok {
			return errors.Errorf("not input validator for type %s", scripts.Death.Type)
		}

		err = sv.Validate(tx, uint32(index))
		if err != nil {
			return err
		}
	}

	for index := range tx.Outputs() {
		scripts, err := tx.GetOutputScriptsAt(index)
		if err != nil {
			return err
		}

		sv, ok := m.OutputStateValidators[scripts.Birth.Type]
		if !ok {
			return errors.Errorf("not input validator for type %s", scripts.Birth.Type)
		}

		err = sv.Validate(tx, uint32(index))
		if err != nil {
			return err
		}
	}
	return nil
}
