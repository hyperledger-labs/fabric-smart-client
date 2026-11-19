/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"runtime"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

// TestSilenceBufferWriteRacesClose closes the buffer while a writer is busy, as
// Run does once the start check matches. A write that fails then would make
// os/exec close the process's pipe, and the process would die of SIGPIPE.
func TestSilenceBufferWriteRacesClose(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	for range 200 {
		b := gbytes.NewBuffer()
		w := &silenceBuffer{buffer: b}
		stop := make(chan struct{})
		errs := make(chan error, 1)
		go func() {
			defer close(errs)
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := w.Write([]byte("x")); err != nil {
					errs <- err

					return
				}
			}
		}()
		for len(b.Contents()) < 64 {
			runtime.Gosched()
		}
		g.Expect(b.Close()).To(gomega.Succeed())
		close(stop)
		g.Expect(<-errs).NotTo(gomega.HaveOccurred())
	}
}
