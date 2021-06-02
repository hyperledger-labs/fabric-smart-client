/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

type StateValidator interface {
	Validate(tx Transaction, index uint32) error
}
