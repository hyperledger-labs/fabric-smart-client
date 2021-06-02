/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

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

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

type BuildServer struct {
	server *http.Server
	lis    net.Listener
}

func NewBuildServer(args ...string) *BuildServer {
	return &BuildServer{
		server: &http.Server{
			Handler: &buildHandler{args: args},
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

func (s *BuildServer) Components() *common.Components {
	Expect(s.lis).NotTo(BeNil())

	helpers.AssertImagesExist(common.RequiredImages...)

	return &common.Components{
		ServerAddress: s.lis.Addr().String(),
	}
}

type artifact struct {
	mutex  sync.Mutex
	input  string
	output string
}

func (a *artifact) build(args ...string) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.output != "" {
		return nil
	}

	output, err := a.gBuild(a.input, args...)
	if err != nil {
		return err
	}

	a.output = output
	return nil
}

func (a *artifact) gBuild(input string, args ...string) (string, error) {
	switch {
	case strings.HasPrefix(input, "git@"):
		fmt.Printf("building %s\n", input)
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
		fmt.Printf("testDir %s\n", testDir)

		goPath := filepath.Join(testDir, "gopath")
		fmt.Printf("goPath %s\n", goPath)
		fabricPath := filepath.Join(testDir, "gopath", "src", packagePath)
		fmt.Printf("fabricPath %s\n", fabricPath)

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
		fmt.Printf("checked out %s done \n", commit)

		fmt.Printf("building [%s,%s] \n", goPath, packagePath+"/"+cmd)

		return gexec.BuildIn(goPath, packagePath+"/"+cmd)
	default:
		return gexec.Build(input, args...)
	}
}

type buildHandler struct {
	mutex     sync.Mutex
	artifacts map[string]*artifact
	args      []string
}

func (b *buildHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	//fmt.Printf("ServeHTTP %v\n", req.URL)
	input := strings.TrimPrefix(req.URL.Path, "/")
	a := b.artifact(input)

	if err := a.build(b.args...); err != nil {
		//fmt.Printf("ServeHTTP %v, failed %s\n", req.URL, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}
	//fmt.Printf("ServeHTTP %v, done %s\n", req.URL, a.output)

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
		a = &artifact{input: input}
		b.artifacts[input] = a
	}

	return a
}
