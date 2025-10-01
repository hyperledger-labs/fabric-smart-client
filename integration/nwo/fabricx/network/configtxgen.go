/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"
)

func generateConfigTxYaml(n *Network) (err error) {
	f, err := os.Create(n.ConfigTxConfigPath())
	if err != nil {
		return err
	}

	defer func() {
		if cerr := f.Close(); cerr != nil {
			if err != nil {
				err = fmt.Errorf("generate configtx error: %w; close error: %w", err, cerr)
			} else {
				err = fmt.Errorf("generate configtx error: %w", cerr)
			}
		}
	}()

	// en := createExtendedNetwork(n)
	pkPath := filepath.Join(
		n.RootDir,
		n.Prefix,
		"crypto",
		"sc_pubkey.pem",
	)

	t, err := template.New("configtx").Funcs(template.FuncMap{
		"MetaNamespaceVerificationKeyPath": func() string { return pkPath },
	}).Parse(configTxTemplate)
	if err != nil {
		return err
	}

	err = t.Execute(io.MultiWriter(f), n)
	if err != nil {
		return err
	}

	return nil
}
