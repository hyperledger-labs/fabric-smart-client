/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"cmp"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/buildinfo"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// ProgramName is the program name
const ProgramName = "node"

// Cmd returns the Cobra Command for Version
func Cmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the fabric smart client version.",
		Long: `Print current version of the fabric smart client.
With --json it prints one JSON object that also lists the wire format ranges and the schema
steps of every store the binary's SDKs register. It reads no configuration and opens no
database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("trailing args detected")
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			if !asJSON {
				_, err := fmt.Fprint(cmd.OutOrStdout(), GetInfo())
				return err
			}
			raw, err := json.MarshalIndent(getReport(), "", "  ")
			if err != nil {
				return errors.Wrap(err, "failed marshalling version report")
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the versions, component ranges and store steps as JSON")
	return cmd
}

// GetInfo returns the program name, the Go toolchain, the platform, and the build info Go
// embeds in the binary: the FSC version, the application module and the VCS revision.
func GetInfo() string {
	return format(buildinfo.Get())
}

func format(info buildinfo.Info) string {
	app := cmp.Or(strings.TrimSpace(info.AppModule+" "+info.AppVersion), "unknown")
	revision := cmp.Or(info.Revision, "unknown")
	if info.Modified {
		revision += " (modified)"
	}
	return fmt.Sprintf("%s:\n Go version: %s\n OS/Arch: %s/%s\n"+
		" FSC version: %s\n Application: %s\n Revision: %s\n",
		ProgramName, runtime.Version(), runtime.GOOS, runtime.GOARCH,
		info.FSCVersion, app, revision)
}

// report is what `version --json` prints. Release tooling reads it, so its JSON field names
// are a contract: fields are added, never renamed or removed.
type report struct {
	FSCVersion  string `json:"fsc_version"`
	AppModule   string `json:"app_module"`
	AppVersion  string `json:"app_version"`
	VCSRevision string `json:"vcs_revision"`
	VCSModified bool   `json:"vcs_modified"`
	// FramingEpoch and Components are what the binary sends in its stream preamble.
	FramingEpoch uint32      `json:"framing_epoch"`
	Components   []component `json:"components"`
	Stores       []store     `json:"stores"`
}

// component is the range of wire format versions the binary supports for one component.
type component struct {
	Name string `json:"name"`
	Min  uint32 `json:"min"`
	Max  uint32 `json:"max"`
}

// store is the schema of one FSC store type in one SQL dialect.
type store struct {
	Type    string `json:"type"`
	Dialect string `json:"dialect"`
	Steps   []step `json:"steps"`
}

// step is one schema step; Hash is migrate.Step.Hash. Contract is migrate.Step.Contract: once
// the step ran, binaries without it refuse the store, so rolling back past it is a restore.
type step struct {
	Version  int    `json:"version"`
	Name     string `json:"name"`
	Hash     string `json:"hash"`
	Contract bool   `json:"contract"`
}

// getReport returns the report of this binary.
func getReport() report {
	info := buildinfo.Get()
	local := preamble.Local()
	r := report{
		FSCVersion:   info.FSCVersion,
		AppModule:    info.AppModule,
		AppVersion:   info.AppVersion,
		VCSRevision:  info.Revision,
		VCSModified:  info.Modified,
		FramingEpoch: local.GetFramingEpoch(),
		Components:   []component{},
		Stores:       []store{},
	}
	for _, c := range local.GetRanges() {
		r.Components = append(r.Components, component{Name: c.GetComponent(), Min: c.GetMin(), Max: c.GetMax()})
	}
	for _, s := range migrate.Schemas() {
		steps := []step{}
		for _, st := range s.Steps {
			steps = append(steps, step{Version: st.Version, Name: st.Name, Hash: st.Hash(), Contract: st.Contract})
		}
		r.Stores = append(r.Stores, store{Type: s.Store, Dialect: string(s.Dialect), Steps: steps})
	}
	return r
}
