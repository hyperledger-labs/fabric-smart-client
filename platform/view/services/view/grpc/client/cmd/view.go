/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	tracing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
	"github.com/spf13/cobra"
)

var (
	// Function used to terminate the CLI
	terminate = os.Exit
	// Function used to redirect output to
	outWriter io.Writer = os.Stderr

	endpoint string
	input    string
	function string
	stdin    bool
	rawInput []byte

	configFile string
	tlsCA      string
	tlsCert    string
	tlsKey     string
	userKey    string
	userCert   string
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Invoke a view.",
		Long:  `Invoke a view.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return invoke()
		},
	}
	// TODO: introduce -web flag to use REST API
	flags := cmd.Flags()
	flags.StringVarP(&input, "input", "i", "", "Sets the input to the view function, encoded either as base64, or as-is")
	flags.StringVarP(&function, "function", "f", "", "Sets the function name to be invoked")
	flags.BoolVarP(&stdin, "stdin", "s", false, "Sets standard input as the input stream")

	flags.StringVarP(&configFile, "configFile", "c", "", "Specifies the config file to load the configuration from")
	flags.StringVarP(&endpoint, "endpoint", "e", "", "Sets the endpoint of the node to connect to (host:port)")
	flags.StringVarP(&tlsCA, "peerTLSCA", "a", "", "Sets the TLS CA certificate file path that verifies the TLS peer's certificate")
	flags.StringVarP(&tlsCert, "tlsCert", "t", "", "(Optional) Sets the client TLS certificate file path that is used when the peer enforces client authentication")
	flags.StringVarP(&tlsKey, "tlsKey", "k", "", "(Optional) Sets the client TLS key file path that is used when the peer enforces client authentication")
	flags.StringVarP(&userKey, "userKey", "u", "", "Sets the user's key file path that is used to sign messages sent to the peer")
	flags.StringVarP(&userCert, "userCert", "r", "", "Sets the user's certificate file path that is used to authenticate the messages sent to the peer")

	return cmd
}

func validateInput(config *Config) error {
	if config.Address == "" {
		return errors.Errorf("endpoint must be specified")
	}

	if function == "" {
		return errors.Errorf("function name must be specified")
	}

	// Check if input is to be read from stdin
	if stdin {
		stdinInput, err := io.ReadAll(os.Stdin)
		if err != nil {
			return errors.Errorf("failed reading input from stdin: %v", err)
		}
		rawInput = stdinInput
		return nil
	}

	if input == "" {
		// If input isn't specified, the input is nil
		return nil
	}

	// Check if it's a base64 encoded string
	rawBase64Encoded, err := base64.StdEncoding.DecodeString(input)
	if err == nil {
		rawInput = rawBase64Encoded
		return nil
	}

	rawInput = []byte(input)

	return nil
}

func parseFlagsToConfig() Config {
	conf := Config{
		Address: endpoint,
		SignerConfig: SignerConfig{
			IdentityPath: userCert,
			KeyPath:      userKey,
		},
		TLSConfig: TLSConfig{
			KeyPath:        tlsKey,
			CertPath:       tlsCert,
			PeerCACertPath: tlsCA,
		},
	}
	return conf
}

func loadConfig(file string) Config {
	conf, err := ConfigFromFile(file)
	if err != nil {
		out("Failed loading config", err)
		terminate(1)
		return Config{}
	}
	return conf
}

func out(a ...interface{}) {
	_, _ = fmt.Fprintln(outWriter, a...)
}

func invoke() error {
	var config Config
	if configFile == "" {
		config = parseFlagsToConfig()
	} else {
		config = loadConfig(configFile)
	}

	if err := validateInput(&config); err != nil {
		return err
	}

	cc := &grpc.ConnectionConfig{
		Address:           config.Address,
		TLSEnabled:        true,
		TLSRootCertFile:   path.Join(config.TLSConfig.PeerCACertPath),
		ConnectionTimeout: 10 * time.Second,
	}

	signer, err := client.NewX509SigningIdentity(config.SignerConfig.IdentityPath, config.SignerConfig.KeyPath)
	if err != nil {
		return err
	}

	tracerProvider, err := tracing2.NewProviderFromConfig(tracing2.NoOp)
	if err != nil {
		return err
	}
	c, err := client.NewClient(
		&client.Config{
			ConnectionConfig: cc,
		},
		signer,
		tracerProvider,
	)
	if err != nil {
		return err
	}

	res, err := c.CallView(function, rawInput)
	if err != nil {
		return err
	}

	switch v := res.(type) {
	case []byte:
		fmt.Println(string(v))
	default:
		fmt.Println(v)
	}
	return nil
}
