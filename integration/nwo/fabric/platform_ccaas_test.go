/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCCaaSBuilderPath(t *testing.T) {
	base := t.TempDir()
	bin := filepath.Join(base, "bin")
	builder := filepath.Join(base, "builders", "ccaas")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(builder, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAB_BINS", bin)

	got, ok := ccaasBuilderPath()
	if !ok {
		t.Fatalf("expected builder found at %s", builder)
	}
	if got != builder {
		t.Fatalf("got %s, want %s", got, builder)
	}

	// A separate temp dir whose sibling "builders/ccaas" was never created.
	// Note: reusing a path under the same base (e.g. filepath.Join(base,
	// "nonexistent")) would NOT exercise the not-found case, since
	// filepath.Dir would still resolve to base, whose builders/ccaas
	// already exists from the setup above.
	other := t.TempDir()
	t.Setenv("FAB_BINS", filepath.Join(other, "bin"))
	if _, ok := ccaasBuilderPath(); ok {
		t.Fatalf("expected not found when builders/ccaas absent")
	}

	t.Setenv("FAB_BINS", "")
	if _, ok := ccaasBuilderPath(); ok {
		t.Fatalf("expected not found when FAB_BINS is empty")
	}
}
