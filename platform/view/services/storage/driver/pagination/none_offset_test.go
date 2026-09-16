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
)

// TestEmptyPagination covers the pagination that forces an empty page: it stays empty
// however it is navigated.
func TestEmptyPagination(t *testing.T) {
	t.Parallel()

	p := Empty()

	prev, err := p.Prev()
	require.NoError(t, err)
	assert.True(t, p.Equal(prev), "going back from empty stays empty")

	next, err := p.Next()
	require.NoError(t, err)
	assert.True(t, p.Equal(next), "going forward from empty stays empty")

	raw, err := p.Serialize()
	require.NoError(t, err)
	assert.Empty(t, raw)

	restored, err := EmptyFromRaw(raw)
	require.NoError(t, err)
	assert.True(t, p.Equal(restored), "an empty pagination round-trips")
}

// TestNonePagination covers the pagination that returns everything in one shot. There is no
// second page, so navigating away from it yields the empty pagination.
func TestNonePagination(t *testing.T) {
	t.Parallel()

	p := None()

	prev, err := p.Prev()
	require.NoError(t, err)
	assert.True(t, Empty().Equal(prev), "there is no page before the only page")

	next, err := p.Next()
	require.NoError(t, err)
	assert.True(t, Empty().Equal(next), "there is no page after the only page")

	raw, err := p.Serialize()
	require.NoError(t, err)
	assert.Empty(t, raw)

	restored, err := NoneFromRaw(raw)
	require.NoError(t, err)
	assert.True(t, p.Equal(restored), "a none pagination round-trips")
}

// TestNoneAndEmptyAreNotEqual checks the two degenerate paginations are distinguishable:
// they mean opposite things, so a caller must not mistake one for the other.
func TestNoneAndEmptyAreNotEqual(t *testing.T) {
	t.Parallel()

	assert.False(t, None().Equal(Empty()))
	assert.False(t, Empty().Equal(None()))

	other, err := Offset(0, 10)
	require.NoError(t, err)
	assert.False(t, None().Equal(other), "a pagination of another type is not equal")
	assert.False(t, Empty().Equal(other))
}

// TestOffsetNavigation covers moving around by pages, and the boundary where going back
// past the beginning degrades to the empty pagination rather than a negative offset.
func TestOffsetNavigation(t *testing.T) {
	t.Parallel()

	const pageSize = 10

	tests := []struct {
		name     string
		from     int
		navigate func(p *offset) (driver.Pagination, error)
		expect   driver.Pagination
	}{
		{"Next advances one page", 0, func(p *offset) (driver.Pagination, error) {
			return p.Next()
		}, mustOffset(t, 10, pageSize)},
		{"Prev retreats one page", 20, func(p *offset) (driver.Pagination, error) {
			return p.Prev()
		}, mustOffset(t, 10, pageSize)},
		{"GoForward advances several pages", 0, func(p *offset) (driver.Pagination, error) {
			return p.GoForward(3)
		}, mustOffset(t, 30, pageSize)},
		{"GoBack retreats several pages", 50, func(p *offset) (driver.Pagination, error) {
			return p.GoBack(2)
		}, mustOffset(t, 30, pageSize)},
		{"GoToPage jumps to a page by number", 0, func(p *offset) (driver.Pagination, error) {
			return p.GoToPage(4)
		}, mustOffset(t, 40, pageSize)},
		{"GoToOffset jumps to an absolute offset", 0, func(p *offset) (driver.Pagination, error) {
			return p.GoToOffset(15)
		}, mustOffset(t, 15, pageSize)},
		{"Prev from the first page is empty", 0, func(p *offset) (driver.Pagination, error) {
			return p.Prev()
		}, Empty()},
		{"GoBack past the beginning is empty", 10, func(p *offset) (driver.Pagination, error) {
			return p.GoBack(5)
		}, Empty()},
		{"GoToOffset with a negative offset is empty", 0, func(p *offset) (driver.Pagination, error) {
			return p.GoToOffset(-1)
		}, Empty()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := mustOffset(t, tc.from, pageSize)

			got, err := tc.navigate(p)

			require.NoError(t, err)
			assert.True(t, tc.expect.Equal(got), "expected %+v, got %+v", tc.expect, got)
		})
	}
}

// TestOffsetEqual checks equality is by offset and page size, and that a pagination of
// another type is never equal.
func TestOffsetEqual(t *testing.T) {
	t.Parallel()

	p := mustOffset(t, 10, 5)

	assert.True(t, p.Equal(mustOffset(t, 10, 5)))
	assert.False(t, p.Equal(mustOffset(t, 20, 5)), "a different offset is not equal")
	assert.False(t, p.Equal(mustOffset(t, 10, 20)), "a different page size is not equal")
	assert.False(t, p.Equal(None()), "a pagination of another type is not equal")
}

// TestOffsetRejectsNegatives checks the constructor's two guards.
func TestOffsetRejectsNegatives(t *testing.T) {
	t.Parallel()

	_, err := Offset(-1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offset")

	_, err = Offset(0, -1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "page size")
}

// TestOffsetFromRawRejectsMalformedCursor checks a cursor that is not valid JSON is
// reported rather than yielding a zero pagination.
func TestOffsetFromRawRejectsMalformedCursor(t *testing.T) {
	t.Parallel()

	_, err := OffsetFromRaw([]byte("not json"))
	require.Error(t, err)
}

func mustOffset(t *testing.T, os, pageSize int) *offset {
	t.Helper()

	p, err := Offset(os, pageSize)
	require.NoError(t, err)

	return p
}
