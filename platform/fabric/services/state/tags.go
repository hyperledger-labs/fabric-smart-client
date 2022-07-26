/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/rwset"
)

func (n *Namespace) setFieldMapping(namespace string, key string, mapping map[string][]byte) error {
	logger.Debugf("setting field mapping for [%s:%s]", namespace, key)
	if len(mapping) == 0 {
		logger.Debugf("setting field mapping for [%s:%s], empty, skipping", namespace, key)
		return nil
	}
	for k, v := range mapping {
		logger.Debugf("setting field mapping for [%s:%s], entry [%s:%s]", namespace, key, k, string(v))
	}

	raw, err := json.Marshal(mapping)
	if err != nil {
		return errors.Wrap(err, "filed marshalling mapping")
	}
	k, err := fieldMappingKey(namespace, key)
	if err != nil {
		return errors.Wrap(err, "filed creating mapping key")
	}
	err = n.tx.SetTransient(k, raw)
	if err != nil {
		return errors.Wrap(err, "failed setting contract")
	}
	logger.Debugf("setting field mapping for [%s:%s][%s], done", namespace, key, string(raw))

	return nil
}

func (n *Namespace) getFieldMapping(namespace string, key string, flag bool) (map[string][]byte, error) {
	logger.Debugf("getting field mapping for [%s:%s]", namespace, key)

	k, err := fieldMappingKey(namespace, key)
	if err != nil {
		return nil, errors.Wrap(err, "filed creating mapping key")
	}
	t := n.tx.GetTransient(k)
	var raw []byte
	if len(t) != 0 {
		logger.Debugf("getting field mapping for [%s:%s], found in transient", namespace, key)
		raw = t
	} else {
		if !flag {
			return nil, nil
		}
		logger.Debugf("getting field mapping for [%s:%s], not found in transient, looking into the rws", namespace, key)
		rws, err := n.tx.RWSet()
		if err != nil {
			return nil, errors.Wrap(err, "filed getting rw set")
		}
		meta, err := rws.GetStateMetadata(namespace, key, fabric.FromBoth)
		if err != nil {
			return nil, errors.Wrap(err, "filed getting metadata")
		}
		if len(meta) == 0 || len(meta[k]) == 0 {
			logger.Debugf("getting field mapping for [%s:%s], not found in rws", namespace, key)
			return map[string][]byte{}, nil
		}
		raw = meta[k]
		logger.Debugf("getting field mapping for [%s:%s], found in rws", namespace, key)
	}

	mapping := map[string][]byte{}
	err = json.Unmarshal(raw, &mapping)
	if err != nil {
		return nil, errors.Wrap(err, "filed unmarshalling mapping")
	}
	for k, v := range mapping {
		logger.Debugf("getting field mapping for [%s:%s], entry [%s:%s]", namespace, key, k, string(v))
	}
	logger.Debugf("getting field mapping for [%s:%s], got [%s][%v]", namespace, key, string(raw), mapping)

	return mapping, nil
}

func (n *Namespace) marshalTags(set *fabric.RWSet, source interface{}) (interface{}, map[string][]byte, error) {
	// dest: source -> dest
	t := reflect.TypeOf(source).Elem()
	dest := reflect.New(t).Interface()
	raw, err := json.Marshal(source)
	if err != nil {
		return nil, nil, err
	}
	err = json.Unmarshal(raw, dest)
	if err != nil {
		return nil, nil, err
	}

	// analyze
	v := reflect.ValueOf(dest).Elem()
	mapping := map[string][]byte{}
	for i := 0; i < t.NumField(); i++ {
		tag, ok := t.Field(i).Tag.Lookup("state")
		if ok {
			switch tag {
			case "hash":
				// todo: supported types are string and byte slice
				// todo: sample a nonce and add it to the transient
				// replace the value with the hash
				field := v.Field(i)
				switch field.Kind() {
				case reflect.String:
					hash := sha256.New()
					hash.Write([]byte(field.String()))
					h := hash.Sum(nil)
					field.SetString(base64.StdEncoding.EncodeToString(h))
				case reflect.Slice:
					mapping[t.Field(i).Name] = field.Bytes()

					hash := sha256.New()
					hash.Write(field.Bytes())
					h := hash.Sum(nil)
					logger.Debugf("computing hash [%s] in place of [%s:%s]",
						base64.StdEncoding.EncodeToString(h), t.Field(i).Name, string(field.Bytes()))
					field.Set(reflect.ValueOf(h))
				}
			}
		}
	}
	return dest, mapping, nil
}

func (n *Namespace) unmarshalTags(set *fabric.RWSet, source interface{}, mapping map[string][]byte) error {
	t := reflect.TypeOf(source).Elem()
	v := reflect.ValueOf(source).Elem()
	for i := 0; i < t.NumField(); i++ {
		tag, ok := t.Field(i).Tag.Lookup("state")
		if ok {
			switch tag {
			case "hash":
				// todo: supported types are string and byte slice
				// replace the value with the hash
				field := v.Field(i)
				switch field.Kind() {
				case reflect.String:
					// todo: load from transient the preimage, check that the image matches, set the preimage.
				case reflect.Slice:
					original, ok := mapping[t.Field(i).Name]
					if !ok {
						return errors.Errorf("mapping not found for [%s]", t.Field(i).Name)
					}

					// check hash
					hash := sha256.New()
					hash.Write(original)
					h := hash.Sum(nil)

					logger.Debugf("recomputing hash [%s] in place of [%s:%s]",
						base64.StdEncoding.EncodeToString(h), t.Field(i).Name, string(original))

					if !bytes.Equal(h, field.Bytes()) {
						return errors.Errorf("failed checking hash, it does not match [%s][%s, %s!=%s]",
							t.Field(i).Name,
							string(original),
							base64.StdEncoding.EncodeToString(h),
							base64.StdEncoding.EncodeToString(field.Bytes()))
					}

					field.Set(reflect.ValueOf(original))
				}
			}
		}
	}
	return nil
}

func fieldMappingKey(ns, key string) (string, error) {
	prefix, attrs, err := rwset.SplitCompositeKey(key)
	if err != nil {
		return "", err
	}
	elems := append([]string{ns, prefix}, attrs...)
	return rwset.CreateCompositeKey("field_mapping", elems)
}
