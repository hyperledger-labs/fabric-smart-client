/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package ccaas provides the mechanics for deploying Fabric chaincode using the
// Chaincode-as-a-Service model: lifecycle packaging, image checks, and the
// container lifecycle. It never imports the nwo fabric network package.
package ccaas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Connection is the connection.json the peer uses to dial the chaincode server.
type Connection struct {
	Address     string `json:"address"`
	DialTimeout string `json:"dial_timeout"`
	TLSRequired bool   `json:"tls_required"`
}

type metadata struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

// BuildPackage writes a ccaas _lifecycle package to outputFile:
//
//	<outputFile>            (tar.gz)
//	  metadata.json         {"path":"","type":"ccaas","label":<label>}
//	  code.tar.gz           (tar.gz)
//	    connection.json     <conn>
func BuildPackage(outputFile, label string, conn Connection) error {
	if err := os.MkdirAll(filepath.Dir(outputFile), 0o755); err != nil {
		return errors.Wrapf(err, "failed to create package dir")
	}

	code, err := tarGz([]tarEntry{{Name: "connection.json", Data: mustJSON(conn)}})
	if err != nil {
		return errors.Wrapf(err, "failed to build code.tar.gz")
	}
	md := mustJSON(metadata{Type: "ccaas", Label: label})
	pkg, err := tarGz([]tarEntry{
		{Name: "metadata.json", Data: md},
		{Name: "code.tar.gz", Data: code},
	})
	if err != nil {
		return errors.Wrapf(err, "failed to build package")
	}
	if err := os.WriteFile(outputFile, pkg, 0o644); err != nil {
		return errors.Wrapf(err, "failed to write package %s", outputFile)
	}
	return nil
}

// tarEntry is one file in a tar.gz. Entries are written in slice order:
// the package id is the hash of these bytes, so the order must not vary.
type tarEntry struct {
	Name string
	Data []byte
}

func tarGz(files []tarEntry) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.Name, Mode: 0o644, Size: int64(len(f.Data)),
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.Data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
