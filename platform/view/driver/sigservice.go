/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

type Identity = driver.VerifyingIdentity

type SigningIdentity = driver.SigningIdentity

// Signer is an interface which wraps the Sign method.
type Signer = driver.Signer

// Verifier is an interface which wraps the Verify method.
type Verifier = driver.Verifier

//go:generate counterfeiter -o mock/sig_service.go -fake-name SigService . SigService

// SigService models a repository of sign and verify keys.
type SigService = driver.SigService

func GetSigService(sp services.Provider) SigService {
	s, err := sp.GetService(reflect.TypeOf((*SigService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(SigService)
}

// AuditRegistry models a repository of identities' audit information
type AuditRegistry = driver.AuditRegistry

func GetAuditRegistry(sp services.Provider) AuditRegistry {
	s, err := sp.GetService(reflect.TypeOf((*AuditRegistry)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(AuditRegistry)
}

type SigRegistry = driver.SigRegistry

func GetSigRegistry(sp services.Provider) SigRegistry {
	s, err := sp.GetService(reflect.TypeOf((*SigRegistry)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(SigRegistry)
}
