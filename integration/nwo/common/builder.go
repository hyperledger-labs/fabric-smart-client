/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("nwo.builder")

type BuilderClient struct {
	ServerAddress string `json:"server_address"`
}

func (c *BuilderClient) Build(path string) string {
	Expect(c.ServerAddress).NotTo(BeEmpty(), "build server address is empty")

	resp, err := http.Get(fmt.Sprintf("http://%s/%s", c.ServerAddress, path))
	Expect(err).NotTo(HaveOccurred())

	body, err := ioutil.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	if resp.StatusCode != http.StatusOK {
		Expect(resp.StatusCode).To(Equal(http.StatusOK), string(body))
	}

	return string(body)
}

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
	Expect(err).NotTo(HaveOccurred())

	s.lis = lis
	go s.server.Serve(lis)
}

func (s *BuildServer) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	defer gexec.CleanupBuildArtifacts()

	s.server.Shutdown(ctx)
}

func (s *BuildServer) Client() *BuilderClient {
	Expect(s.lis).NotTo(BeNil())

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
	switch {
	case strings.HasPrefix(input, "git@"):
		logger.Infof("building %s", input)
		// split input in repo#commit#packagePath#cmd
		entries := strings.Split(input, ";")
		repo := entries[0]
		commit := entries[1]
		packagePath := entries[2]
		cmd := entries[3]

		testDir, err := ioutil.TempDir("", "fabric")
		if err != nil {
			return "", err
		}
		logger.Infof("testDir %s", testDir)

		goPath := filepath.Join(testDir, "gopath")
		logger.Infof("goPath %s", goPath)
		fabricPath := filepath.Join(testDir, "gopath", "src", packagePath)
		logger.Infof("fabricPath %s", fabricPath)

		r, err := git.PlainClone(fabricPath, false, &git.CloneOptions{
			URL:      repo,
			Progress: os.Stdout,
		})
		if err != nil {
			return "", err
		}
		w, err := r.Worktree()
		if err != nil {
			return "", err
		}
		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(commit),
		})
		if err != nil {
			return "", err
		}
		logger.Infof("checked out %s done ", commit)

		logger.Infof("building [%s,%s] ", goPath, packagePath+"/"+cmd)

		return gexec.BuildIn(goPath, packagePath+"/"+cmd)
	default:
		if a.raceDetectorEnabled && !strings.HasPrefix(input, "github.com/hyperledger/fabric/") {
			logger.Infof("building [%s,%s] with race detection ", input, args)
			return gexec.Build(input, args...)
		}

		logger.Infof("building [%s,%s] ", input, args)
		return gexec.Build(input, args...)
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
	input = strings.Replace(input, "\n", "", -1)
	input = strings.Replace(input, "\r", "", -1)
	a := b.artifact(input)
	if err := a.build(b.args...); err != nil {
		// logger.Infof("ServeHTTP %v, failed %s", req.URL, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}
	// logger.Infof("ServeHTTP %v, done %s", req.URL, a.output)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s", a.output)
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
