/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeYAML_SingleDocument(t *testing.T) {
	t.Parallel()
	input := []byte(`
a: 1
b: hello
`)
	out, err := MergeYAML(input)
	require.NoError(t, err)
	require.YAMLEq(t, string(input), string(out))
}

func TestMergeYAML_NoDocuments(t *testing.T) {
	t.Parallel()
	out, err := MergeYAML()
	require.NoError(t, err)
	require.YAMLEq(t, "{}", string(out))
}

func TestMergeYAML_NonOverlappingKeys(t *testing.T) {
	t.Parallel()
	a := []byte(`a: 1`)
	b := []byte(`b: 2`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, "a: 1\nb: 2", string(out))
}

func TestMergeYAML_ScalarOverride(t *testing.T) {
	t.Parallel()
	// later document wins for scalar values
	a := []byte(`key: original`)
	b := []byte(`key: overridden`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, `key: overridden`, string(out))
}

func TestMergeYAML_DeepMapMerge(t *testing.T) {
	t.Parallel()
	// nested maps are merged recursively, not replaced wholesale
	a := []byte(`
outer:
  a: 1
  b: 2
`)
	b := []byte(`
outer:
  b: 99
  c: 3
`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, `outer: {a: 1, b: 99, c: 3}`, string(out))
}

func TestMergeYAML_ArrayReplacement(t *testing.T) {
	t.Parallel()
	// arrays are replaced by the later document, not appended
	a := []byte(`items: [1, 2, 3]`)
	b := []byte(`items: [4, 5]`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, `items: [4, 5]`, string(out))
}

func TestMergeYAML_MapReplacesScalar(t *testing.T) {
	t.Parallel()
	// if types differ, later value wins unconditionally
	a := []byte(`key: scalar`)
	b := []byte(`key: {nested: value}`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, `key: {nested: value}`, string(out))
}

func TestMergeYAML_ScalarReplacesMap(t *testing.T) {
	t.Parallel()
	a := []byte(`key: {nested: value}`)
	b := []byte(`key: scalar`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, `key: scalar`, string(out))
}

func TestMergeYAML_MultipleDocuments(t *testing.T) {
	t.Parallel()
	// merge is applied left to right; last writer wins on conflicts
	a := []byte(`x: 1`)
	b := []byte(`x: 2`)
	c := []byte(`x: 3`)
	out, err := MergeYAML(a, b, c)
	require.NoError(t, err)
	require.YAMLEq(t, `x: 3`, string(out))
}

func TestMergeYAML_EmptyDocumentIgnored(t *testing.T) {
	t.Parallel()
	a := []byte(`a: 1`)
	b := []byte(``)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, `a: 1`, string(out))
}

func TestMergeYAML_DeeplyNestedMaps(t *testing.T) {
	t.Parallel()
	a := []byte(`
level1:
  level2:
    level3:
      x: 1
      y: 2
`)
	b := []byte(`
level1:
  level2:
    level3:
      y: 99
      z: 3
    extra: hello
`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, `level1: {level2: {level3: {x: 1, y: 99, z: 3}, extra: hello}}`, string(out))
}

func TestMergeYAML_PreservesTypes(t *testing.T) {
	t.Parallel()
	a := []byte(`
int_val: 42
bool_val: true
float_val: 3.14
`)
	b := []byte(`str_val: hello`)
	out, err := MergeYAML(a, b)
	require.NoError(t, err)
	require.YAMLEq(t, "int_val: 42\nbool_val: true\nfloat_val: 3.14\nstr_val: hello", string(out))
}
