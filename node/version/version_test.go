/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/buildinfo"
	fabricsql "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	viewsql "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

func TestGetInfo(t *testing.T) {
	t.Parallel()
	info := GetInfo()
	require.Contains(t, info, ProgramName)
	require.Contains(t, info, runtime.Version())
	require.Contains(t, info, runtime.GOOS)
	require.Contains(t, info, runtime.GOARCH)
}

func TestCmd_TrailingArgs(t *testing.T) {
	t.Parallel()

	cmd := Cmd()
	cmd.SetArgs([]string{"unexpected"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing args detected")
}

func TestCmd_NoArgs(t *testing.T) {
	t.Parallel()

	cmd := Cmd()
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
}

func TestFormat(t *testing.T) {
	t.Parallel()
	prefix := ProgramName + ":\n Go version: " + runtime.Version() + "\n OS/Arch: " + runtime.GOOS + "/" + runtime.GOARCH + "\n"

	got := format(buildinfo.Info{FSCVersion: "v0.27.0", AppModule: "github.com/acme/app", AppVersion: "v1.4.0", Revision: "abc123", Modified: true})
	require.Equal(t, prefix+" FSC version: v0.27.0\n Application: github.com/acme/app v1.4.0\n Revision: abc123 (modified)\n", got)

	got = format(buildinfo.Info{FSCVersion: "(devel)"})
	require.True(t, strings.HasPrefix(got, prefix), got)
	require.Contains(t, got, " FSC version: (devel)\n")
	require.Contains(t, got, " Application: unknown\n")
	require.Contains(t, got, " Revision: unknown\n")
}

// TestJSON checks that --json prints a report with every component and every registered store,
// with the steps of the golden files. The view and Fabric SDKs register these lists when built.
func TestJSON(t *testing.T) { //nolint:paralleltest // registers process-wide schemas and components
	migrate.RegisterSchemas("view", viewsql.Schemas)
	migrate.RegisterSchemas("fabric", fabricsql.Schemas)
	// An application component is reported without any SDK installed.
	preamble.RegisterComponent("iou", 1, 2)
	t.Cleanup(func() { require.NoError(t, preamble.SetComponents(preamble.FSCComponents)) })
	cmd := Cmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--json"})
	require.NoError(t, cmd.Execute())

	var r report
	require.NoError(t, json.Unmarshal(out.Bytes(), &r))
	require.Equal(t, buildinfo.Get().FSCVersion, r.FSCVersion)
	require.Equal(t, preamble.FramingEpoch, r.FramingEpoch)
	components := map[string]component{}
	for _, c := range r.Components {
		components[c.Name] = c
	}
	require.Len(t, components, 4)
	for _, name := range []string{"view", "fabric", "fabricx"} {
		require.Contains(t, components, name)
	}
	require.Equal(t, component{Name: "iou", Min: 1, Max: 2}, components["iou"])

	want := map[string]string{}
	for _, dir := range []string{
		"../../platform/view/services/storage/driver/sql/testdata/steps",
		"../../platform/fabric/services/db/driver/sql/testdata/steps",
	} {
		files, err := filepath.Glob(filepath.Join(dir, "*.steps"))
		require.NoError(t, err)
		require.NotEmpty(t, files, dir)
		for _, f := range files {
			raw, err := os.ReadFile(f)
			require.NoError(t, err)
			want[strings.TrimSuffix(filepath.Base(f), ".steps")] = string(raw)
		}
	}
	got := map[string]string{}
	contract := map[string]bool{}
	for _, s := range r.Stores {
		var lines strings.Builder
		for _, step := range s.Steps {
			fmt.Fprintf(&lines, "%d %s %s\n", step.Version, step.Hash, step.Name)
			contract[fmt.Sprintf("%s-%s %d", s.Dialect, s.Type, step.Version)] = step.Contract
		}
		got[s.Dialect+"-"+s.Type] = lines.String()
	}
	require.Equal(t, want, got)

	// Every step is reported with its flag; these are the FSC contract steps.
	wantContract := map[string]bool{}
	for key := range contract {
		wantContract[key] = false
	}
	for _, key := range []string{"postgres-vault 2", "sqlite-vault 2", "postgres-endorsetx 2", "sqlite-endorsetx 2"} {
		wantContract[key] = true
	}
	require.Equal(t, wantContract, contract)
	require.Contains(t, out.String(), `"contract": true`)
}
