/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"github.com/onsi/gomega/gbytes"
)

type silenceBuffer struct {
	buffer *gbytes.Buffer
}

func (c *silenceBuffer) Write(p []byte) (n int, err error) {
	if c.buffer.Closed() {
		// NO-OP
		return len(p), nil
	}

	return c.buffer.Write(p)
}

func NewSilenceBuffer(writer *gbytes.Buffer) *silenceBuffer {
	return &silenceBuffer{
		buffer: writer,
	}
}
