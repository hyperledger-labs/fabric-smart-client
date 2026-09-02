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

// AddOutputOption configures a single call to [Namespace.AddOutput]. An option that
// returns an error aborts the call before anything is written.
type AddOutputOption func(*addOutputOptions) error

// WithContract records contract as the name of the contract governing the output,
// under the "contract" key of the output's state metadata.
func WithContract(contract string) AddOutputOption {
	return func(o *addOutputOptions) error {
		o.contract = contract
		return nil
	}
}

// WithHashHiding stores the SHA-256 digest of the encoded state on the ledger in
// place of the state itself, and keeps the preimage in the transaction's transient
// data. Recovering the state requires that transient data; the ledger alone yields
// only the digest.
//
// This hides the whole state. A single []byte field is hidden the same way by tagging
// it state:"hash", independently of this option; [Namespace.AddOutput] returns an
// error if that tag appears on a field of any other type.
func WithHashHiding() AddOutputOption {
	return func(o *addOutputOptions) error {
		o.hashHiding = true
		return nil
	}
}

// WithStateBasedEndorsement sets the output's validation parameter to a state-based
// endorsement policy requiring every owner of the state to endorse, replacing any
// policy already stored there.
//
// The state must implement [Ownable]; for any other state the option has no effect.
func WithStateBasedEndorsement() AddOutputOption {
	return func(o *addOutputOptions) error {
		o.sbe = true
		return nil
	}
}

type addInputOptions struct {
	certification bool
}

// AddInputOption configures a single call to [Namespace.AddInputByLinearID]. An option
// that returns an error aborts the call.
type AddInputOption func(*addInputOptions) error

// WithCertification certifies the input, so the other parties can check that the value
// read is the committed one. The certification is produced before the call returns, by
// the [Certifier] registered as a service or by [ChaincodeCertifier] if there is none,
// and a failure to produce it fails the call.
func WithCertification() AddInputOption {
	return func(o *addInputOptions) error {
		o.certification = true
		return nil
	}
}
