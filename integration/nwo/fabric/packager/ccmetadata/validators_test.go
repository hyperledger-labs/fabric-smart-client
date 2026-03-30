/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccmetadata

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoodIndexJSON(t *testing.T) {
	fileName := "META-INF/statedb/couchdb/indexes/myIndex.json"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	require.NoError(t, err, "Error validating a good index")
}

func TestBadIndexJSON(t *testing.T) {
	fileName := "META-INF/statedb/couchdb/indexes/myIndex.json"
	fileBytes := []byte("invalid json")

	err := ValidateMetadataFile(fileName, fileBytes)

	require.Error(t, err, "Should have received an InvalidIndexContentError")

	// Type assertion on InvalidIndexContentError
	var invalidIndexContentError *InvalidIndexContentError
	ok := errors.As(err, &invalidIndexContentError)
	require.True(t, ok, "Should have received an InvalidIndexContentError")

	t.Log("SAMPLE ERROR STRING:", err.Error())
}

func TestIndexWrongLocation(t *testing.T) {
	fileName := "META-INF/statedb/couchdb/myIndex.json"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	require.Error(t, err, "Should have received an UnhandledDirectoryError")

	// Type assertion on UnhandledDirectoryError
	var unhandledDirectoryError *UnhandledDirectoryError
	ok := errors.As(err, &unhandledDirectoryError)
	require.True(t, ok, "Should have received an UnhandledDirectoryError")

	t.Log("SAMPLE ERROR STRING:", err.Error())
}

func TestInvalidMetadataType(t *testing.T) {
	fileName := "myIndex.json"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	require.Error(t, err, "Should have received an UnhandledDirectoryError")

	// Type assertion on UnhandledDirectoryError
	var unhandledDirectoryError *UnhandledDirectoryError
	ok := errors.As(err, &unhandledDirectoryError)
	require.True(t, ok, "Should have received an UnhandledDirectoryError")
}

func TestBadMetadataExtension(t *testing.T) {
	fileName := "myIndex.go"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	require.Error(t, err, "Should have received an error")
}

func TestBadFilePaths(t *testing.T) {
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	t.Run("bad META-INF", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF1/statedb/couchdb/indexes/test1.json", fileBytes)
		t.Log(err)
		require.Error(t, err, "Should have received an error for bad META-INF directory")
	})

	t.Run("bad path length", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF/statedb/test1.json", fileBytes)
		t.Log(err)
		require.Error(t, err, "Should have received an error for bad length")
	})

	t.Run("invalid database name", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF/statedb/goleveldb/indexes/test1.json", fileBytes)
		t.Log(err)
		require.Error(t, err, "Should have received an error for invalid database")
	})

	t.Run("invalid indexes directory name", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF/statedb/couchdb/index/test1.json", fileBytes)
		t.Log(err)
		require.Error(t, err, "Should have received an error for invalid indexes directory")
	})

	t.Run("invalid collections directory name", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF/statedb/couchdb/collection/testcoll/indexes/test1.json", fileBytes)
		t.Log(err)
		require.Error(t, err, "Should have received an error for invalid collections directory")
	})

	t.Run("valid collections name", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF/statedb/couchdb/collections/testcoll/indexes/test1.json", fileBytes)
		t.Log(err)
		require.NoError(t, err, "Error should not have been thrown for a valid collection name")
	})

	t.Run("invalid collections name with special char", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF/statedb/couchdb/collections/#testcoll/indexes/test1.json", fileBytes)
		t.Log(err)
		require.Error(t, err, "Should have received an error for an invalid collection name")
	})

	t.Run("invalid file extension in collection", func(t *testing.T) {
		err := ValidateMetadataFile("META-INF/statedb/couchdb/collections/testcoll/indexes/test1.txt", fileBytes)
		t.Log(err)
		require.Error(t, err, "Should have received an error for an invalid file name")
	})
}

func TestIndexValidation(t *testing.T) {
	t.Run("valid index with field sorts", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":[{"size":"desc"}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.NoError(t, validateIndexJSON(indexDefinition))
	})

	t.Run("valid index without field sorts", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.NoError(t, validateIndexJSON(indexDefinition))
	})

	t.Run("valid index without design doc, name and type", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":["size","color"]}}`)
		_, indexDefinition := isJSON(indexDef)
		require.NoError(t, validateIndexJSON(indexDefinition))
	})

	t.Run("valid index with partial filter selector", func(t *testing.T) {
		indexDef := []byte(`{
			  "index": {
			    "partial_filter_selector": {
			      "status": {
			        "$ne": "archived"
			      }
			    },
			    "fields": ["type"]
			  },
			  "ddoc" : "type-not-archived",
			  "type" : "json"
			}`)
		_, indexDefinition := isJSON(indexDef)
		require.NoError(t, validateIndexJSON(indexDefinition))
	})
}

func TestIndexValidationInvalidParameters(t *testing.T) {
	t.Run("numeric design doc", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":[{"size":"desc"}, {"color":"desc"}]},"ddoc":1, "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for numeric design doc")
	})

	t.Run("invalid design doc parameter name", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":["size","color"]},"ddoc1":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for invalid design doc parameter")
	})

	t.Run("invalid name parameter", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name1":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for invalid name parameter")
	})

	t.Run("numeric type parameter", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":1}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for numeric type parameter")
	})

	t.Run("invalid type value", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"text"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for invalid type parameter")
	})

	t.Run("invalid index parameter name", func(t *testing.T) {
		indexDef := []byte(`{"index1":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for invalid index parameter")
	})

	t.Run("missing index parameter", func(t *testing.T) {
		indexDef := []byte(`{"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for missing index parameter")
	})
}

func TestIndexValidationInvalidFields(t *testing.T) {
	t.Run("invalid fields parameter name", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields1":[{"size":"desc"}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for invalid fields parameter")
	})

	t.Run("numeric field name", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":["size", 1]},"ddoc1":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for field name defined as numeric")
	})

	t.Run("invalid field sort value", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":[{"size":"desc1"}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for invalid field sort")
	})

	t.Run("numeric field sort value", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":[{"size":1}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for a numeric in field sort")
	})

	t.Run("fields as string instead of array", func(t *testing.T) {
		indexDef := []byte(`{"index":{"fields":"size"},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for invalid field json")
	})

	t.Run("index as string instead of object", func(t *testing.T) {
		indexDef := []byte(`{"index":"fields","ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
		_, indexDefinition := isJSON(indexDef)
		require.Error(t, validateIndexJSON(indexDefinition), "Error should have been thrown for missing JSON for fields")
	})
}
