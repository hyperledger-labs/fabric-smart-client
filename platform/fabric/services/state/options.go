/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

// ServiceOptions defines the options for the state service
type ServiceOptions struct {
	// Network is the name of the network to use
	Network string
	// Channel is the name of the channel to use
	Channel string
	// Identity is the name of the identity to use
	Identity string
}

// CompileServiceOptions compiles the service options
func CompileServiceOptions(opts ...ServiceOption) (*ServiceOptions, error) {
	txOptions := &ServiceOptions{}
	for _, opt := range opts {
		if err := opt(txOptions); err != nil {
			return nil, err
		}
	}
	return txOptions, nil
}

// ServiceOption models an option
type ServiceOption func(*ServiceOptions) error

// WithNetwork is a ServiceOption to set the network
func WithNetwork(network string) ServiceOption {
	return func(o *ServiceOptions) error {
		o.Network = network
		return nil
	}
}

// WithChannel is a ServiceOption to set the channel
func WithChannel(channel string) ServiceOption {
	return func(o *ServiceOptions) error {
		o.Channel = channel
		return nil
	}
}

// WithIdentity is a ServiceOption to set the identity
func WithIdentity(identity string) ServiceOption {
	return func(o *ServiceOptions) error {
		o.Identity = identity
		return nil
	}
}
