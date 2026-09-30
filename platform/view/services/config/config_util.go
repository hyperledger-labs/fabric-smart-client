/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"encoding/pem"
	"math"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/v2"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// byteSizeRegexp matches byte sizes such as "10mb", "5 GB" or "3k".
var byteSizeRegexp = regexp.MustCompile(`^([0-9]+)\s*(?i)([kmg])b?$`)

// customDecodeHook parses strings of the format "[thing1, thing2, thing3]" into string
// slices, trimming whitespace around each element. "[]" decodes to an empty slice.
func customDecodeHook(f, _ reflect.Type, data any) (any, error) {
	if f.Kind() != reflect.String {
		return data, nil
	}

	raw, ok := data.(string)
	if !ok {
		return nil, errors.Errorf("unexpected data type [%T]", data)
	}
	l := len(raw)
	if l > 1 && raw[0] == '[' && raw[l-1] == ']' {
		inner := strings.TrimSpace(raw[1 : l-1])
		if inner == "" {
			return []string{}, nil
		}
		slice := strings.Split(inner, ",")
		for i, v := range slice {
			slice[i] = strings.TrimSpace(v)
		}
		return slice, nil
	}

	return data, nil
}

// byteSizeDecodeHook parses strings like "10mb", "5gb" into uint32 bytes.
func byteSizeDecodeHook(f, t reflect.Kind, data any) (any, error) {
	if f != reflect.String || t != reflect.Uint32 {
		return data, nil
	}
	raw, ok := data.(string)
	if !ok {
		return nil, errors.Errorf("unexpected data type [%T]", data)
	}
	if raw == "" {
		return data, nil
	}
	m := byteSizeRegexp.FindStringSubmatch(raw)
	if m == nil {
		return data, nil
	}
	size, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return data, errors.Wrapf(err, "invalid byte size value '%s'", raw)
	}
	var shift uint
	switch strings.ToLower(m[2]) {
	case "k":
		shift = 10
	case "m":
		shift = 20
	case "g":
		shift = 30
	}
	// Checked before shifting: a shift that overflows uint64 would wrap silently.
	if size > math.MaxUint32>>shift {
		return data, errors.Errorf("value '%s' overflows uint32", raw)
	}
	return size << shift, nil
}

// stringFromFileDecodeHook decodes a map with a "File" (or "file") key into a string by
// reading the named file. Any other input is returned unchanged.
func stringFromFileDecodeHook(f, t reflect.Kind, data any) (any, error) {
	if f != reflect.Map || t != reflect.String {
		return data, nil
	}
	d, ok := data.(map[string]any)
	if !ok {
		return nil, errors.Errorf("unexpected data type [%T]", data)
	}
	fileName, ok := d["File"]
	if !ok {
		fileName, ok = d["file"]
	}
	switch {
	case !ok:
		return data, nil
	case fileName == nil:
		return nil, errors.Errorf("value of File: was nil")
	}
	fileNameStr, ok := fileName.(string)
	if !ok {
		return nil, errors.Errorf("unexpected File value type [%T]", fileName)
	}
	bytes, err := os.ReadFile(fileNameStr)
	if err != nil {
		return data, err
	}
	return string(bytes), nil
}

// pemBlocksFromFileDecodeHook decodes a map with a string "File" (or "file") value into the
// CERTIFICATE PEM blocks without headers found in the named file; other blocks are skipped.
// Any other input is returned unchanged.
func pemBlocksFromFileDecodeHook(f, t reflect.Kind, data any) (any, error) {
	if f != reflect.Map || t != reflect.Slice {
		return data, nil
	}
	var fileName string
	var ok bool
	switch d := data.(type) {
	case map[string]string:
		fileName, ok = d["File"]
		if !ok {
			fileName, ok = d["file"]
		}
	case map[string]any:
		fileI, found := d["File"]
		if !found {
			fileI = d["file"]
		}
		fileName, ok = fileI.(string)
	}
	switch {
	case !ok:
		return data, nil
	case fileName == "":
		return nil, errors.Errorf("value of File: is empty")
	}
	bytes, err := os.ReadFile(fileName)
	if err != nil {
		return data, err
	}
	var result []string
	for len(bytes) > 0 {
		var block *pem.Block
		block, bytes = pem.Decode(bytes)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		result = append(result, string(pem.EncodeToMemory(block)))
	}
	return result, nil
}

// EnhancedExactUnmarshal is intended to unmarshal a config file into a structure
// producing error when extraneous variables are introduced and supporting
// the time.Duration type
func EnhancedExactUnmarshal(v *koanf.Koanf, key string, output any) error {
	oType := reflect.TypeOf(output)
	if oType.Kind() != reflect.Pointer {
		return errors.Errorf("supplied output argument must be a pointer to a struct but is not pointer")
	}
	config := &mapstructure.DecoderConfig{
		ErrorUnused:      false,
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			customDecodeHook,
			byteSizeDecodeHook,
			stringFromFileDecodeHook,
			pemBlocksFromFileDecodeHook,
		),
	}
	return v.UnmarshalWithConf(key, output, koanf.UnmarshalConf{
		DecoderConfig: config,
		Tag:           "yaml",
	})
}

// StrictUnmarshalSubtree decodes a raw configuration subtree into output, returning an error
// that names any key with no corresponding field. Unlike [EnhancedExactUnmarshal] it does
// not read files: a `{file: ...}` map decodes to the path, not to the file's contents.
func StrictUnmarshalSubtree(raw map[string]any, output any) error {
	// ErrorUnused is scoped here rather than set globally, which would reject every SDK
	// extension section. The file-reading hooks are omitted because they call os.ReadFile
	// before TranslatePath has run, which breaks relative paths.
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		ErrorUnused:      true,
		Result:           output,
		WeaklyTypedInput: true,
		TagName:          "yaml",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			customDecodeHook,
		),
	})
	if err != nil {
		return errors.Wrap(err, "failed building strict decoder")
	}
	return decoder.Decode(raw)
}
