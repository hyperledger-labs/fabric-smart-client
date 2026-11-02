/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package migratetest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

var steps = []migrate.Step{
	{Version: 1, Name: "create", SQL: []string{"CREATE TABLE a (id INT)"}},
	{Version: 2, Name: "index", SQL: []string{"CREATE INDEX i ON a (id)"}},
}

// check runs CheckSteps on a fake T and reports whether it failed.
func check(golden string, steps []migrate.Step) bool {
	ft := &testing.T{}
	done := make(chan struct{})
	go func() { // CheckSteps may call Fatalf, which ends the goroutine
		defer close(done)
		CheckSteps(ft, golden, steps)
	}()
	<-done
	return ft.Failed()
}

func TestCheckSteps(t *testing.T) { //nolint:paralleltest // sets UpdateEnv
	golden := filepath.Join(t.TempDir(), "s.steps")

	require.True(t, check(golden, steps), "a missing file fails without the update variable")
	_, err := os.Stat(golden)
	require.True(t, os.IsNotExist(err), "nothing is written without the update variable")

	t.Setenv(UpdateEnv, "1")
	require.False(t, check(golden, steps[:1]))
	require.False(t, check(golden, steps), "a new step is appended")
	raw, err := os.ReadFile(golden)
	require.NoError(t, err)
	require.Equal(t, goldenLine(steps[0])+"\n"+goldenLine(steps[1])+"\n", string(raw))

	require.False(t, check(golden, steps), "matching steps pass")
	require.True(t, check(golden, steps[:1]), "a removed step fails")
	edited := []migrate.Step{steps[0], {Version: 2, Name: "index", SQL: []string{"CREATE INDEX j ON a (id)"}}}
	require.True(t, check(golden, edited), "an edited step fails, even with the update variable")
	raw2, err := os.ReadFile(golden)
	require.NoError(t, err)
	require.Equal(t, string(raw), string(raw2), "a failing check never rewrites the file")

	require.NoError(t, os.WriteFile(golden, []byte(goldenLine(steps[0])), 0o600))
	require.False(t, check(golden, steps), "a file without a trailing newline gets its new line on its own line")
	require.False(t, check(golden, steps))

	require.NoError(t, os.WriteFile(golden, []byte(goldenLine(steps[0])+"x\n"), 0o600))
	require.True(t, check(golden, steps), "editing a golden line fails")
}
