/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

const (
	// DefaultIncomingMessagesBufferSize is the default buffer size for the incoming messages channel
	DefaultIncomingMessagesBufferSize = 1024
	// DefaultStreamReaderBufferSize is the default buffer size for stream readers
	DefaultStreamReaderBufferSize = 4096
)

type configService interface {
	GetInt(key string) int
	IsSet(key string) bool
}

type config struct {
	incomingMessagesBufferSize int
	streamReaderBufferSize     int
}

func NewConfig(cs configService) *config {
	incomingMessagesBufferSize := DefaultIncomingMessagesBufferSize
	if cs.IsSet("fsc.p2p.incomingMessagesBufferSize") {
		incomingMessagesBufferSize = cs.GetInt("fsc.p2p.incomingMessagesBufferSize")
	}

	streamReaderBufferSize := DefaultStreamReaderBufferSize
	if cs.IsSet("fsc.p2p.streamReaderBufferSize") {
		streamReaderBufferSize = cs.GetInt("fsc.p2p.streamReaderBufferSize")
	}

	return &config{
		incomingMessagesBufferSize: incomingMessagesBufferSize,
		streamReaderBufferSize:     streamReaderBufferSize,
	}
}
