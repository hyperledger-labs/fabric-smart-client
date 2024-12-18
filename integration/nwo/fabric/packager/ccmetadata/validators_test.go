/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccmetadata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/stretchr/testify/assert"
)

var packageTestDir = filepath.Join(os.TempDir(), "ccmetadata-validator-test")

func TestGoodIndexJSON(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "GoodIndexJSON")
	defer utils.IgnoreError(cleanupDir(testDir))

	fileName := "META-INF/statedb/couchdb/indexes/myIndex.json"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	assert.NoError(t, err, "Error validating a good index")
}

func TestBadIndexJSON(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "BadIndexJSON")
	defer utils.IgnoreError(cleanupDir(testDir))

	fileName := "META-INF/statedb/couchdb/indexes/myIndex.json"
	fileBytes := []byte("invalid json")

	err := ValidateMetadataFile(fileName, fileBytes)

	assert.Error(t, err, "Should have received an InvalidIndexContentError")

	// Type assertion on InvalidIndexContentError
	var invalidIndexContentError *InvalidIndexContentError
	ok := errors.As(err, &invalidIndexContentError)
	assert.True(t, ok, "Should have received an InvalidIndexContentError")

	t.Log("SAMPLE ERROR STRING:", err.Error())
}

func TestIndexWrongLocation(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "IndexWrongLocation")
	defer utils.IgnoreError(cleanupDir(testDir))

	fileName := "META-INF/statedb/couchdb/myIndex.json"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	assert.Error(t, err, "Should have received an UnhandledDirectoryError")

	// Type assertion on UnhandledDirectoryError
	var unhandledDirectoryError *UnhandledDirectoryError
	ok := errors.As(err, &unhandledDirectoryError)
	assert.True(t, ok, "Should have received an UnhandledDirectoryError")

	t.Log("SAMPLE ERROR STRING:", err.Error())
}

func TestInvalidMetadataType(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "InvalidMetadataType")
	defer utils.IgnoreError(cleanupDir(testDir))

	fileName := "myIndex.json"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	assert.Error(t, err, "Should have received an UnhandledDirectoryError")

	// Type assertion on UnhandledDirectoryError
	var unhandledDirectoryError *UnhandledDirectoryError
	ok := errors.As(err, &unhandledDirectoryError)
	assert.True(t, ok, "Should have received an UnhandledDirectoryError")
}

func TestBadMetadataExtension(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "BadMetadataExtension")
	defer utils.IgnoreError(cleanupDir(testDir))

	fileName := "myIndex.go"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	assert.Error(t, err, "Should have received an error")

}

func TestBadFilePaths(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "BadMetadataExtension")
	defer utils.IgnoreError(cleanupDir(testDir))

	// Test bad META-INF
	fileName := "META-INF1/statedb/couchdb/indexes/test1.json"
	fileBytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.Error(t, err, "Should have received an error for bad META-INF directory")

	// Test bad path length
	fileName = "META-INF/statedb/test1.json"
	fileBytes = []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err = ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.Error(t, err, "Should have received an error for bad length")

	// Test invalid database name
	fileName = "META-INF/statedb/goleveldb/indexes/test1.json"
	fileBytes = []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err = ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.Error(t, err, "Should have received an error for invalid database")

	// Test invalid indexes directory name
	fileName = "META-INF/statedb/couchdb/index/test1.json"
	fileBytes = []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err = ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.Error(t, err, "Should have received an error for invalid indexes directory")

	// Test invalid collections directory name
	fileName = "META-INF/statedb/couchdb/collection/testcoll/indexes/test1.json"
	fileBytes = []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err = ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.Error(t, err, "Should have received an error for invalid collections directory")

	// Test valid collections name
	fileName = "META-INF/statedb/couchdb/collections/testcoll/indexes/test1.json"
	fileBytes = []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err = ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.NoError(t, err, "Error should not have been thrown for a valid collection name")

	// Test invalid collections name
	fileName = "META-INF/statedb/couchdb/collections/#testcoll/indexes/test1.json"
	fileBytes = []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err = ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.Error(t, err, "Should have received an error for an invalid collection name")

	// Test invalid collections name
	fileName = "META-INF/statedb/couchdb/collections/testcoll/indexes/test1.txt"
	fileBytes = []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err = ValidateMetadataFile(fileName, fileBytes)
	fmt.Println(err)
	assert.Error(t, err, "Should have received an error for an invalid file name")

}

func TestIndexValidation(t *testing.T) {

	// Test valid index with field sorts
	indexDef := []byte(`{"index":{"fields":[{"size":"desc"}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition := isJSON(indexDef)
	err := validateIndexJSON(indexDefinition)
	assert.NoError(t, err)

	// Test valid index without field sorts
	indexDef = []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.NoError(t, err)

	// Test valid index without design doc, name and type
	indexDef = []byte(`{"index":{"fields":["size","color"]}}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.NoError(t, err)

	// Test valid index with partial filter selector (only tests that it will not return error if included)
	indexDef = []byte(`{
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
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.NoError(t, err)

}

func TestIndexValidationInvalidParameters(t *testing.T) {

	// Test numeric values passed in for parameters
	indexDef := []byte(`{"index":{"fields":[{"size":"desc"}, {"color":"desc"}]},"ddoc":1, "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition := isJSON(indexDef)
	err := validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for numeric design doc")

	// Test invalid design doc parameter
	indexDef = []byte(`{"index":{"fields":["size","color"]},"ddoc1":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for invalid design doc parameter")

	// Test invalid name parameter
	indexDef = []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name1":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for invalid name parameter")

	// Test invalid type parameter, numeric
	indexDef = []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":1}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for numeric type parameter")

	// Test invalid type parameter
	indexDef = []byte(`{"index":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"text"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for invalid type parameter")

	// Test invalid index parameter
	indexDef = []byte(`{"index1":{"fields":["size","color"]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for invalid index parameter")

	// Test missing index parameter
	indexDef = []byte(`{"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for missing index parameter")

}

func TestIndexValidationInvalidFields(t *testing.T) {

	// Test invalid fields parameter
	indexDef := []byte(`{"index":{"fields1":[{"size":"desc"}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition := isJSON(indexDef)
	err := validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for invalid fields parameter")

	// Test invalid field name (numeric)
	indexDef = []byte(`{"index":{"fields":["size", 1]},"ddoc1":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for field name defined as numeric")

	// Test invalid field sort
	indexDef = []byte(`{"index":{"fields":[{"size":"desc1"}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for invalid field sort")

	// Test numeric in sort
	indexDef = []byte(`{"index":{"fields":[{"size":1}, {"color":"desc"}]},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for a numeric in field sort")

	// Test invalid json for fields
	indexDef = []byte(`{"index":{"fields":"size"},"ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for invalid field json")

	// Test missing JSON for fields
	indexDef = []byte(`{"index":"fields","ddoc":"indexSizeSortName", "name":"indexSizeSortName","type":"json"}`)
	_, indexDefinition = isJSON(indexDef)
	err = validateIndexJSON(indexDefinition)
	assert.Error(t, err, "Error should have been thrown for missing JSON for fields")

}

func cleanupDir(dir string) error {
	// clean up any previous files
	err := os.RemoveAll(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(dir, os.ModePerm)
}
