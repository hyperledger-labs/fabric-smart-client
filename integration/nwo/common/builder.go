/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

var logger = logging.MustGetLogger()

type BuilderClient struct {
	ServerAddress string `json:"server_address"`
}

func (c *BuilderClient) Build(path string) string {
	gomega.Expect(c.ServerAddress).NotTo(gomega.BeEmpty(), "build server address is empty")

	resp, err := http.Get(fmt.Sprintf("http://%s/%s", c.ServerAddress, path))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if resp.StatusCode != http.StatusOK {
		gomega.Expect(resp.StatusCode).To(gomega.Equal(http.StatusOK), string(body))
	}

	return string(body)
}

// BuildServer builds Go binaries on request for the components of one network.
// A server builds each package at most once. The binaries are shared by every
// server of the process: see CleanupBuildArtifacts.
type BuildServer struct {
	server *http.Server
	lis    net.Listener
	bh     *buildHandler
}

func NewBuildServer(args ...string) *BuildServer {
	bh := &buildHandler{args: args}
	return &BuildServer{
		bh: bh,
		server: &http.Server{
			Handler: bh,
		},
	}
}

func (s *BuildServer) Serve() {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	s.lis = lis
	go func() {
		_ = s.server.Serve(lis)
	}()
}

// Shutdown stops the server. If deleteArtifacts is set, it also deletes the
// binaries of every build server of the process, see CleanupBuildArtifacts.
func (s *BuildServer) Shutdown(deleteArtifacts bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if deleteArtifacts {
		defer CleanupBuildArtifacts()
	}

	_ = s.server.Shutdown(ctx)
}

func (s *BuildServer) Client() *BuilderClient {
	gomega.Expect(s.lis).NotTo(gomega.BeNil())

	return &BuilderClient{
		ServerAddress: s.lis.Addr().String(),
	}
}

func (s *BuildServer) EnableRaceDetector() {
	s.bh.EnableRaceDetector()
}

type artifact struct {
	mutex               sync.Mutex
	input               string
	output              string
	raceDetectorEnabled bool
}

func (a *artifact) build(args ...string) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.output != "" {
		return nil
	}

	output, err := a.gBuild(a.input, args...)
	if err != nil {
		logger.Errorf("Error building %s: %s", a.input, err)
		return err
	}

	a.output = output
	return nil
}

func (a *artifact) gBuild(input string, args ...string) (string, error) {
	if a.raceDetectorEnabled && !strings.HasPrefix(input, "github.com/hyperledger/fabric/") {
		args = append(args, "-race")
	}

	dir, err := artifactsDir()
	if err != nil {
		return "", err
	}
	// A later network of the process builds into the same file, and go build
	// relinks it only if its sources or flags changed; nwo regenerates the main
	// package of every FSC node from the topology, so the sources can change.
	// The flags are part of the path to keep, for example, the race and the
	// non-race binary of a package side by side instead of relinking in turn.
	// Networks running at the same time share the file, so a node restarted
	// after another network rebuilt its package runs the other sources.
	h := sha256.Sum256([]byte(strings.Join(append([]string{input}, args...), "\x00")))
	output := filepath.Join(dir, hex.EncodeToString(h[:8]), path.Base(input))

	logger.Infof("building [%s,%s] ", input, args)
	start := time.Now()
	cmd := exec.Command("go", append(append([]string{"build"}, args...), "-o", output, input)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", errors.Wrapf(err, "failed to build %s:\n%s", input, out)
	}
	logger.Infof("built [%s] in %s", input, time.Since(start))
	return output, nil
}

var (
	artifactsMutex sync.Mutex
	artifacts      string
)

// artifactsDir returns the directory holding the binaries of every build
// server of the process, creating it on first use.
func artifactsDir() (string, error) {
	artifactsMutex.Lock()
	defer artifactsMutex.Unlock()
	if artifacts == "" {
		dir, err := os.MkdirTemp("", "nwo_artifacts")
		if err != nil {
			return "", err
		}
		artifacts = dir
	}
	return artifacts, nil
}

// CleanupBuildArtifacts deletes the binaries built by every build server of
// the process. Stopping a network keeps them, so that the next network of the
// process does not rebuild unchanged binaries; call it once the process needs
// no more networks, for example in a ginkgo AfterSuite. A build server still
// in use afterwards returns paths of deleted binaries.
func CleanupBuildArtifacts() {
	artifactsMutex.Lock()
	defer artifactsMutex.Unlock()
	if artifacts != "" {
		_ = os.RemoveAll(artifacts)
		artifacts = ""
	}
}

type buildHandler struct {
	mutex               sync.Mutex
	artifacts           map[string]*artifact
	args                []string
	raceDetectorEnabled bool
}

func (b *buildHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// logger.Infof("ServeHTTP %v", req.URL)

	input := strings.TrimPrefix(req.URL.Path, "/")
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	a := b.artifact(input)
	if err := a.build(b.args...); err != nil {
		// logger.Infof("ServeHTTP %v, failed %s", req.URL, err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "%s", err)
		return
	}
	// logger.Infof("ServeHTTP %v, done %s", req.URL, a.output)

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "%s", a.output)
}

func (b *buildHandler) artifact(input string) *artifact {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.artifacts == nil {
		b.artifacts = map[string]*artifact{}
	}

	a, ok := b.artifacts[input]
	if !ok {
		a = &artifact{input: input, raceDetectorEnabled: b.raceDetectorEnabled}
		b.artifacts[input] = a
	}

	return a
}

func (b *buildHandler) EnableRaceDetector() {
	b.raceDetectorEnabled = true
}
