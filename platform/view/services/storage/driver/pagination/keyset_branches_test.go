/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// record stands in for a row returned from the database. The field is named Id rather
// than ID because PropertyName looks it up by that literal name, and one of the tests
// below reads it that way.
type record struct {
	Id   string //nolint:revive // var-naming: the field name is what PropertyName looks up
	Size int
}

func (r record) ID() string { return r.Id }

// TestExtractFieldWrongType checks a field that is not of the expected type is reported as
// a programming error rather than silently yielding a zero value.
func TestExtractFieldWrongType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "an-id", PropertyName[string]("Id").ExtractField(record{Id: "an-id"}))

	assert.Panics(t, func() {
		_ = PropertyName[string]("Size").ExtractField(record{Size: 3})
	}, "an int field read as a string is a mistake worth surfacing")
}

// TestKeysetConstructorsRejectUnexportedField checks both constructors that take a field
// name refuse one reflection cannot read.
func TestKeysetConstructorsRejectUnexportedField(t *testing.T) {
	t.Parallel()

	_, err := KeysetWithField[string](0, 10, "id", PropertyName[string]("id"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exported field")

	valid, err := KeysetWithField[string](0, 10, "id", PropertyName[string]("Id"))
	require.NoError(t, err)

	raw, err := valid.Serialize()
	require.NoError(t, err)

	_, err = KeysetFromRaw(raw, PropertyName[string]("id"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exported field")
}

// TestKeysetWithId checks the constructor for results that carry their own ID.
func TestKeysetWithId(t *testing.T) {
	t.Parallel()

	p, err := KeysetWithId[string, record](0, 10, "id")

	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "an-id", p.idGetter(record{Id: "an-id"}), "the id comes from the result itself")
}

// TestKeysetRejectsNegatives checks the constructor's offset and page size guards.
func TestKeysetRejectsNegatives(t *testing.T) {
	t.Parallel()

	_, err := Keyset[string, any](-1, 10, "id", func(any) string { return "" })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offset")

	_, err = Keyset[string, any](0, -1, "id", func(any) string { return "" })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "page size")
}

// TestNilElementUnsupportedType checks a key type other than int or string is rejected
// loudly: the cursor comparison has no sentinel for it.
func TestNilElementUnsupportedType(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "unsupported type", func() { _ = nilElement[bool]() })
}

// TestKeysetGoToOffsetRejectsNegative checks a negative offset is an error here, unlike the
// offset pagination which degrades to an empty page.
func TestKeysetGoToOffsetRejectsNegative(t *testing.T) {
	t.Parallel()

	p := mustKeyset(t, 0, 10)

	_, err := p.GoToOffset(-1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "offset")
}

// TestKeysetGoToPage checks jumping to a page by number lands on the matching offset.
func TestKeysetGoToPage(t *testing.T) {
	t.Parallel()

	p := mustKeyset(t, 0, 10)

	got, err := p.GoToPage(3)

	require.NoError(t, err)
	assert.True(t, mustKeyset(t, 30, 10).Equal(got), "page 3 of 10 starts at offset 30")
}

// TestKeysetEqualOtherType checks a pagination of another type is never equal.
func TestKeysetEqualOtherType(t *testing.T) {
	t.Parallel()

	assert.False(t, mustKeyset(t, 0, 10).Equal(None()))
}

// TestNewPageWithNonKeysetPagination checks the paginations that carry no cursor are passed
// through with their results untouched.
func TestNewPageWithNonKeysetPagination(t *testing.T) {
	t.Parallel()

	offsetPagination, err := Offset(0, 10)
	require.NoError(t, err)

	for name, p := range map[string]driver.Pagination{
		"offset": offsetPagination,
		"empty":  Empty(),
		"none":   None(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			items := []*record{{Id: "a"}, {Id: "b"}}

			page, err := NewPage(collections.NewSliceIterator(items), p)

			require.NoError(t, err)
			require.NotNil(t, page)
			assert.True(t, p.Equal(page.Pagination), "the pagination is passed through unchanged")

			read, err := iterators.ReadAllPointers(page.Items)
			require.NoError(t, err)
			assert.Len(t, read, len(items), "the results are passed through unchanged")
		})
	}
}

func mustKeyset(t *testing.T, offset, pageSize int) *keyset[string, any] {
	t.Helper()

	p, err := Keyset[string, any](offset, pageSize, dbdriver.FieldName("id"), func(any) string { return "" })
	require.NoError(t, err)

	return p
}
