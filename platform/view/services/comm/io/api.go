/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io/streamio"
)

/* Dream interface ideally non-blocking */
// Rename Channel into Conn

// We want a function blocking (timeout):
// NewConn(stub shim.ChaincodeStubInterface, targetPeer string, sessionID string) (Conn, error)

type Conn interface {
	io.Writer
	io.Reader
	Flush() error
}

type MsgConn interface {
	io.Writer
	streamio.MsgReader
	Flush() error
}
