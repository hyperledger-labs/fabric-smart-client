/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package migratetest checks in tests that released schema steps stay unchanged.
//
// A golden file holds one line per step, "<version> <hash> <name>", where hash is
// migrate.Step.Hash. CheckSteps fails when a line of the file differs from the
// step of that version or has no step, so a released step cannot be edited,
// reordered or removed unnoticed. Steps beyond the file are new: CheckSteps
// fails and asks to append them by running the test with FSC_UPDATE_STEPS=1,
// which never rewrites an existing line.
package migratetest

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// UpdateEnv is the environment variable that makes CheckSteps append new steps
// to the golden file instead of failing.
const UpdateEnv = "FSC_UPDATE_STEPS"

// goldenLine returns the golden line of s.
func goldenLine(s migrate.Step) string { return fmt.Sprintf("%d %s %s", s.Version, s.Hash(), s.Name) }

// CheckSteps compares steps with the golden file at path; see the package
// documentation. Generate the steps with fixed table names, since steps embed
// them in their SQL.
func CheckSteps(tb testing.TB, golden string, steps []migrate.Step) {
	tb.Helper()
	raw, err := os.ReadFile(golden)
	if err != nil && !os.IsNotExist(err) {
		tb.Fatalf("read %s: %v", golden, err)
	}
	var lines []string
	if s := strings.TrimSpace(string(raw)); s != "" {
		lines = strings.Split(s, "\n")
	}
	changed := false
	for i, line := range lines {
		if i >= len(steps) {
			changed = true
			tb.Errorf("%s:%d: released step %q was removed", golden, i+1, line)
			continue
		}
		if want := goldenLine(steps[i]); line != want {
			changed = true
			tb.Errorf("%s:%d: released step changed:\n  released: %s\n  now:      %s\nA released step must not change; add a new step instead.", golden, i+1, line, want)
		}
	}
	if changed || len(steps) <= len(lines) {
		return
	}
	var added []string
	for _, s := range steps[len(lines):] {
		added = append(added, goldenLine(s))
	}
	if os.Getenv(UpdateEnv) == "" {
		tb.Errorf("%s: %d new step(s) not recorded:\n  %s\nRun the test with %s=1 to append them.", golden, len(added), strings.Join(added, "\n  "), UpdateEnv)
		return
	}
	f, err := os.OpenFile(golden, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // golden files are committed and world-readable
	if err != nil {
		tb.Fatalf("open %s: %v", golden, err)
	}
	defer func() { _ = f.Close() }()
	text := strings.Join(added, "\n") + "\n"
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		text = "\n" + text
	}
	if _, err := f.WriteString(text); err != nil {
		tb.Fatalf("append to %s: %v", golden, err)
	}
}
