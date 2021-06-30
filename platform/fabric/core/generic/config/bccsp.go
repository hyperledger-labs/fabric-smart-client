/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

type FileKeyStore struct {
	KeyStore string `yaml:"KeyStore,omitempty"`
}

type SoftwareProvider struct {
	Hash         string        `yaml:"Hash,omitempty"`
	Security     int           `yaml:"Security,omitempty"`
	FileKeyStore *FileKeyStore `yaml:"FileKeyStore,omitempty"`
}

type BCCSP struct {
	Default string            `yaml:"Default,omitempty"`
	SW      *SoftwareProvider `yaml:"SW,omitempty"`
}
