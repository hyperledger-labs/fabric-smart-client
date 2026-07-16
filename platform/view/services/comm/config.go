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
	// DefaultMaxMessageSize is the default max message size limit (10 MiB)
	DefaultMaxMessageSize = 10 * 1024 * 1024
)

type configService interface {
	GetInt(key string) int
	IsSet(key string) bool
}

type config struct {
	incomingMessagesBufferSize int
	streamReaderBufferSize     int
	maxRecvMsgSize             int
	maxSendMsgSize             int
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

	maxRecvMsgSize := DefaultMaxMessageSize
	if cs.IsSet("fsc.p2p.maxRecvMsgSize") {
		maxRecvMsgSize = cs.GetInt("fsc.p2p.maxRecvMsgSize")
	}

	maxSendMsgSize := DefaultMaxMessageSize
	if cs.IsSet("fsc.p2p.maxSendMsgSize") {
		maxSendMsgSize = cs.GetInt("fsc.p2p.maxSendMsgSize")
	}

	if incomingMessagesBufferSize <= 0 {
		incomingMessagesBufferSize = DefaultIncomingMessagesBufferSize
	}
	if streamReaderBufferSize <= 0 {
		streamReaderBufferSize = DefaultStreamReaderBufferSize
	}
	if maxRecvMsgSize < 0 {
		maxRecvMsgSize = DefaultMaxMessageSize
	}
	if maxSendMsgSize < 0 {
		maxSendMsgSize = DefaultMaxMessageSize
	}

	return &config{
		incomingMessagesBufferSize: incomingMessagesBufferSize,
		streamReaderBufferSize:     streamReaderBufferSize,
		maxRecvMsgSize:             maxRecvMsgSize,
		maxSendMsgSize:             maxSendMsgSize,
	}
}
