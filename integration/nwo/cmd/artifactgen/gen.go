/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package artifactgen

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/orion"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver"

	"gopkg.in/yaml.v2"
)

type Topology struct {
	Type string `yaml:"type,omitempty"`
}

type Topologies struct {
	Topologies []Topology `yaml:"topologies,omitempty"`
}

type T struct {
	Topologies []interface{} `yaml:"topologies,omitempty"`
}

var topologyFile string
var output string
var port int

// NewCmd returns the Cobra Command for the artifactsgen
func NewCmd() *cobra.Command {
	// Set the flags on the node start command.
	rootCommand := &cobra.Command{
		Use:   "artifactsgen",
		Short: "Gen artifacts.",
		Long:  `Read topology from file and generates artifacts.`,
	}

	genCmd := &cobra.Command{
		Use:   "gen",
		Short: "Gen artifacts.",
		Long:  `Generate artifacts from a topology file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			return gen(args)
		},
	}
	flags := genCmd.Flags()
	flags.StringVarP(&topologyFile, "topology", "t", "", "topology file in yaml format")
	flags.StringVarP(&output, "output", "o", "./testdata", "output folder")
	flags.IntVarP(&port, "port", "p", 20000, "host starting port")

	rootCommand.AddCommand(
		genCmd,
	)

	return rootCommand
}

// gen read topology and generates artifacts
func gen(args []string) error {
	if len(topologyFile) == 0 {
		return errors.Errorf("expecting topology file path")
	}
	raw, err := ioutil.ReadFile(topologyFile)
	if err != nil {
		return errors.Wrapf(err, "failed reading topology file [%s]", topologyFile)
	}
	names := &Topologies{}
	if err := yaml.Unmarshal(raw, names); err != nil {
		return errors.Wrapf(err, "failed unmarshalling topology file [%s]", topologyFile)
	}

	t := &T{}
	if err := yaml.Unmarshal(raw, t); err != nil {
		return errors.Wrapf(err, "failed unmarshalling topology file [%s]", topologyFile)
	}
	t2 := []api.Topology{}
	for i, topology := range names.Topologies {
		switch topology.Type {
		case fabric.TopologyName:
			top := fabric.NewTopology()
			r, err := yaml.Marshal(t.Topologies[i])
			if err != nil {
				return errors.Wrapf(err, "failed remarshalling topology configuration [%s]", topologyFile)
			}
			if err := yaml.Unmarshal(r, top); err != nil {
				return errors.Wrapf(err, "failed unmarshalling topology file [%s]", topologyFile)
			}
			t2 = append(t2, top)
		case fsc.TopologyName:
			top := fsc.NewTopology()
			r, err := yaml.Marshal(t.Topologies[i])
			if err != nil {
				return errors.Wrapf(err, "failed remarshalling topology configuration [%s]", topologyFile)
			}
			if err := yaml.Unmarshal(r, top); err != nil {
				return errors.Wrapf(err, "failed unmarshalling topology file [%s]", topologyFile)
			}
			t2 = append(t2, top)
		case weaver.TopologyName:
			top := weaver.NewTopology()
			r, err := yaml.Marshal(t.Topologies[i])
			if err != nil {
				return errors.Wrapf(err, "failed remarshalling topology configuration [%s]", topologyFile)
			}
			if err := yaml.Unmarshal(r, top); err != nil {
				return errors.Wrapf(err, "failed unmarshalling topology file [%s]", topologyFile)
			}
			t2 = append(t2, top)
		case orion.TopologyName:
			top := orion.NewTopology()
			r, err := yaml.Marshal(t.Topologies[i])
			if err != nil {
				return errors.Wrapf(err, "failed remarshalling topology configuration [%s]", topologyFile)
			}
			if err := yaml.Unmarshal(r, top); err != nil {
				return errors.Wrapf(err, "failed unmarshalling topology file [%s]", topologyFile)
			}
			t2 = append(t2, top)
		}
	}

	_, err = integration.GenerateAt(port, output, false, t2...)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating generator for [%s]", topologyFile)
	}

	return nil
}
