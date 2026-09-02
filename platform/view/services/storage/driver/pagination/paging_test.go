/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// build renders a SELECT the way a store does, so the assertions are on the
// statement a caller would actually execute.
func build(t *testing.T, p driver.Pagination) (string, []sqlbuild.Param) {
	t.Helper()
	pag := Paging(p)
	return sqlbuild.New().
		WriteString("SELECT tx_id FROM status").
		WriteWhere(pag.Where).
		WritePaging(pag).
		Build()
}

func TestPagingNil(t *testing.T) {
	t.Parallel()

	sql, params := build(t, nil)
	require.Equal(t, "SELECT tx_id FROM status", sql)
	require.Nil(t, params)
}

func TestPagingNone(t *testing.T) {
	t.Parallel()

	sql, params := build(t, None())
	require.Equal(t, "SELECT tx_id FROM status", sql)
	require.Nil(t, params)
}

func TestPagingEmpty(t *testing.T) {
	t.Parallel()

	sql, params := build(t, Empty())
	require.Equal(t, "SELECT tx_id FROM status LIMIT 0", sql)
	require.Nil(t, params)
}

func TestPagingOffset(t *testing.T) {
	t.Parallel()

	p, err := Offset(0, 5)
	require.NoError(t, err)
	sql, _ := build(t, p)
	require.Equal(t, "SELECT tx_id FROM status LIMIT 5", sql)

	p, err = Offset(10, 5)
	require.NoError(t, err)
	sql, _ = build(t, p)
	require.Equal(t, "SELECT tx_id FROM status LIMIT 5 OFFSET 10", sql)
}

func TestPagingKeysetString(t *testing.T) {
	t.Parallel()

	k, err := KeysetWithField[string](0, 5, "tx_id", "TxID")
	require.NoError(t, err)

	sql, params := build(t, k)
	require.Equal(t, "SELECT tx_id FROM status ORDER BY tx_id ASC LIMIT 5", sql)
	require.Nil(t, params)

	// once a cursor is set it wins over the offset
	k.Offset = 7
	k.FirstID = "tx3"
	sql, params = build(t, k)
	require.Equal(t, "SELECT tx_id FROM status WHERE tx_id > $1 ORDER BY tx_id ASC LIMIT 5", sql)
	require.Equal(t, []sqlbuild.Param{"tx3"}, params)
}

func TestPagingKeysetInt(t *testing.T) {
	t.Parallel()

	k, err := KeysetWithField[int](3, 5, "pos", "Pos")
	require.NoError(t, err)

	sql, params := build(t, k)
	require.Equal(t, "SELECT tx_id FROM status ORDER BY pos ASC LIMIT 5 OFFSET 3", sql)
	require.Nil(t, params)

	k.FirstID = 42
	sql, params = build(t, k)
	require.Equal(t, "SELECT tx_id FROM status WHERE pos > $1 ORDER BY pos ASC LIMIT 5", sql)
	require.Equal(t, []sqlbuild.Param{42}, params)
}

// Keyset with a concrete element type used to panic in ApplyToSquirrel and
// NewDefaultInterpreter, which type-switched on keyset[I, any] only. Dispatching
// through a method covers every instantiation.
func TestPagingKeysetConcreteElementType(t *testing.T) {
	t.Parallel()

	k, err := Keyset[string, driver.TxStatus](0, 5, "tx_id", func(s driver.TxStatus) string { return s.TxID })
	require.NoError(t, err)

	sql, _ := build(t, k)
	require.Equal(t, "SELECT tx_id FROM status ORDER BY tx_id ASC LIMIT 5", sql)
}

type foreignPagination struct{}

func (foreignPagination) Prev() (driver.Pagination, error) { return nil, nil }
func (foreignPagination) Next() (driver.Pagination, error) { return nil, nil }
func (foreignPagination) Equal(driver.Pagination) bool     { return false }
func (foreignPagination) Serialize() ([]byte, error)       { return nil, nil }

func TestPagingPanicsOnForeignPagination(t *testing.T) {
	t.Parallel()

	require.PanicsWithValue(t,
		"invalid pagination option {}",
		func() { Paging(foreignPagination{}) },
	)
}

func TestOffsetFromRawRoundTrip(t *testing.T) {
	t.Parallel()

	p, err := Offset(10, 5)
	require.NoError(t, err)
	raw, err := p.Serialize()
	require.NoError(t, err)

	back, err := OffsetFromRaw(raw)
	require.NoError(t, err)
	require.True(t, back.Equal(p))

	sql, params := build(t, back)
	require.Equal(t, "SELECT tx_id FROM status LIMIT 5 OFFSET 10", sql)
	require.Nil(t, params)
}

// An empty column name would drop ORDER BY entirely, so the keyset would page
// through rows in whatever order the database returns them — silently skipping
// and repeating rows instead of failing.
func TestKeysetRejectsEmptyColumnName(t *testing.T) {
	t.Parallel()

	_, err := KeysetWithField[string](0, 5, "", "TxID")
	require.Error(t, err)

	_, err = Keyset[string, driver.TxStatus](0, 5, "", func(s driver.TxStatus) string { return s.TxID })
	require.Error(t, err)

	_, err = KeysetWithId[string, keysetID](0, 5, "")
	require.Error(t, err)
}

type keysetID struct{ TxID string }

func (k keysetID) Id() string { return k.TxID }

// The column name is interpolated straight into ORDER BY and into the cursor
// comparison, and a deserialized cursor is untrusted input.
func TestKeysetFromRawRejectsUnusableColumnNames(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"offset":0,"page_size":5,"sqlid_name":""}`,
		`{"offset":0,"page_size":5}`,
		`{"offset":0,"page_size":5,"sqlid_name":"tx_id ASC; DROP TABLE status--"}`,
		`{"offset":0,"page_size":5,"sqlid_name":"tx_id, code"}`,
		`{"offset":0,"page_size":5,"sqlid_name":"1pos"}`,
	} {
		_, err := KeysetFromRaw[string]([]byte(raw), "TxID")
		require.Error(t, err, raw)
	}
}

func TestKeysetFromRawAcceptsAPlainColumnName(t *testing.T) {
	t.Parallel()

	k, err := KeysetFromRaw[string]([]byte(`{"offset":0,"page_size":5,"sqlid_name":"tx_id"}`), "TxID")
	require.NoError(t, err)

	sql, _ := build(t, k)
	require.Equal(t, "SELECT tx_id FROM status ORDER BY tx_id ASC LIMIT 5", sql)
}

// A negative page size renders no LIMIT clause at all, so an unvalidated cursor
// would turn a page request into a full-table read.
func TestOffsetFromRawRejectsNegativeValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"offset":0,"pageSize":-1}`,
		`{"offset":-1,"pageSize":5}`,
	} {
		_, err := OffsetFromRaw([]byte(raw))
		require.Error(t, err, raw)
	}
}
