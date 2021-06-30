/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

type addOutputOptions struct {
	contract   string
	hashHiding bool
	sbe        bool
}

type AddOutputOption func(*addOutputOptions) error

func WithContract(contract string) AddOutputOption {
	return func(o *addOutputOptions) error {
		o.contract = contract
		return nil
	}
}

func WithHashHiding() AddOutputOption {
	return func(o *addOutputOptions) error {
		o.hashHiding = true
		return nil
	}
}

func WithStateBasedEndorsement() AddOutputOption {
	return func(o *addOutputOptions) error {
		o.sbe = true
		return nil
	}
}

type addInputOptions struct {
	certification bool
}

type AddInputOption func(*addInputOptions) error

func WithCertification() AddInputOption {
	return func(o *addInputOptions) error {
		o.certification = true
		return nil
	}
}
