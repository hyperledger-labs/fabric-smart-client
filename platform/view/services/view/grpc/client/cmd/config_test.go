/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	configFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("config-%d.yaml", rand.Int()))
	fmt.Println(configFilePath)
	t.Run("save and load a config", func(t *testing.T) {
		c := Config{
			TLSConfig: TLSClientConfig{
				Enabled:            true,
				ClientAuthRequired: true,
				ClientCertPath:     "foo",
				ClientKeyPath:      "foo",
				RootCACertPath:     "foo",
			},
			SignerConfig: SignerConfig{
				KeyPath:      "foo",
				IdentityPath: "foo",
				MSPID:        "foo",
			},
		}

		err := c.ToFile(configFilePath)
		defer utils.IgnoreErrorWithOneArg(os.RemoveAll, configFilePath)
		require.NoError(t, err)

		c2, err := ConfigFromFile(configFilePath)
		require.NoError(t, err)
		require.Equal(t, c, c2)
	})

	t.Run("bad config isn't saved", func(t *testing.T) {
		c := Config{}
		err := c.ToFile(configFilePath)
		require.Contains(t, err.Error(), "config isn't valid")
	})

	t.Run("bad config isn't loaded", func(t *testing.T) {
		_, err := ConfigFromFile(filepath.Join("testdata", "not_a_yaml.yaml"))
		require.Contains(t, err.Error(), "error unmarshalling YAML file")
	})

	t.Run("file that doesn't exist isn't loaded", func(t *testing.T) {
		_, err := ConfigFromFile(filepath.Join("testdata", "not_a_file.yaml"))
		require.Contains(t, err.Error(), "no such file or directory")
	})
}
