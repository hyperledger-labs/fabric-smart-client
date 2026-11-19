/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"github.com/onsi/gomega/gbytes"
)

// silenceBuffer feeds a process's output to the buffer Run watches for the
// start check, and drops it once Run has closed that buffer.
type silenceBuffer struct {
	buffer *gbytes.Buffer
}

// Write never fails. Run closes the buffer while the process keeps writing,
// and an error here would reach os/exec, which then closes the pipe and the
// process dies of SIGPIPE on its next write.
func (c *silenceBuffer) Write(p []byte) (int, error) {
	// gbytes.Buffer.Write fails only once the buffer is closed.
	_, _ = c.buffer.Write(p)

	return len(p), nil
}
