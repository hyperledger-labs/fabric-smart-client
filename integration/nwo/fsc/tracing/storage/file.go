/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package storage

import (
	"fmt"
	"os"

	dmetrics "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fsc.tracing")

type FileStorage struct {
	path string
	f    *os.File
}

func NewFileStorage(filePath string) (*FileStorage, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	return &FileStorage{path: filePath, f: f}, nil
}

func (f *FileStorage) Store(event *dmetrics.Event) {
	logger.Debugf("Storing event from [%s:%s]", event.Host, event.Type)
	// write event to file
	_, err := f.f.WriteString(fmt.Sprintf(
		"%s, %f, %v, %s, %d, %v\n",
		event.Type,
		event.Value,
		event.Keys,
		event.Host,
		event.Raw.Timestamp.UnixNano(),
		event.Raw.Timestamp.String(),
	))
	if err != nil {
		fmt.Println(err)
	}
}

func (f *FileStorage) Close() {
	err := f.f.Close()
	if err != nil {
		fmt.Println(err)
	}
}
