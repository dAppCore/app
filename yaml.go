// SPDX-License-Identifier: EUPL-1.2

package app

import "gopkg.in/yaml.v3"

// yamlMarshalBytes is a thin wrapper over gopkg.in/yaml.v3 Marshal so
// every packaging file shares one entry point. Keeps the encoder swap
// cheap — change this one function and every wrap/install path
// follows.
//
//	body, err := yamlMarshalBytes(manifest)
func yamlMarshalBytes(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

// yamlUnmarshalImpl is the matching wrapper for yaml.Unmarshal so the
// encoder swap stays mechanical. Used by marketplace_verify.go and any
// other path that needs to round-trip a manifest body in memory.
//
//	_ = yamlUnmarshalImpl(body, &manifest)
func yamlUnmarshalImpl(body []byte, dst any) error {
	return yaml.Unmarshal(body, dst)
}
