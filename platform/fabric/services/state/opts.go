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
	useRawValue   bool
	rawValue      []byte
	rawVersion    []byte
}

type AddInputOption func(*addInputOptions) error

func WithCertification() AddInputOption {
	return func(o *addInputOptions) error {
		o.certification = true
		return nil
	}
}

// WithRawValue provides a pre-fetched state and its version to be used as input.
// This bypasses local vault lookup while still recording a read dependency.
func WithRawValue(raw []byte, version []byte) AddInputOption {
	return func(o *addInputOptions) error {
		o.useRawValue = true
		o.rawValue = raw
		o.rawVersion = version
		return nil
	}
}
