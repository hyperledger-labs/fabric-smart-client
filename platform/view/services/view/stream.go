/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

//go:generate counterfeiter -o mock/stream.go -fake-name Stream . Stream

// Stream models a communication stream.
type Stream interface {
	// Recv receives a message from the stream.
	Recv(m interface{}) error
	// Send sends a message to the stream.
	Send(m interface{}) error
}

// GetStream returns the stream from the service provider.
// It panics if no stream is found.
func GetStream(sp services.Provider) Stream {
	scsBoxed, err := GetStreamIfExists(sp)
	if err != nil {
		panic(err)
	}
	return scsBoxed
}

// GetStreamIfExists returns the stream from the service provider if it exists.
func GetStreamIfExists(sp services.Provider) (Stream, error) {
	scsBoxed, err := sp.GetService(reflect.TypeOf((*Stream)(nil)))
	if err != nil {
		return nil, err
	}
	return scsBoxed.(Stream), nil
}
