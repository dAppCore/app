// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/rand" // Note: AX-6 - workspace KDF salts require cryptographic randomness; no core CSPRNG wrapper exists.

	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
	coreerr "dappco.re/go/log"
)

const (
	workspaceCryptoMetadataFileName = "workspace.crypto.json"
	workspaceSecretSaltBytes        = 32
)

type workspaceCryptoMetadata struct {
	Version int    `json:"version"`
	Salt    string `json:"salt"`
}

func ensureWorkspaceSecretSalt(ws *Workspace) ([]byte, error) {
	metadata, err := readWorkspaceCryptoMetadata(ws)
	if err != nil {
		return nil, err
	}
	if core.Trim(metadata.Salt) != "" {
		return decodeWorkspaceSecretSalt(metadata.Salt)
	}

	salt := make([]byte, workspaceSecretSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, coreerr.E("app.ensureWorkspaceSecretSalt", "generate workspace salt failed", err)
	}

	metadata.Version = 1
	metadata.Salt = core.Base64Encode(salt)
	if err := writeWorkspaceCryptoMetadata(ws, metadata); err != nil {
		return nil, err
	}

	out := make([]byte, len(salt))
	copy(out, salt)
	return out, nil
}

func readWorkspaceCryptoMetadata(ws *Workspace) (workspaceCryptoMetadata, error) {
	if ws == nil {
		return workspaceCryptoMetadata{}, coreerr.E("app.readWorkspaceCryptoMetadata", "nil workspace", nil)
	}

	medium := workspaceMetadataMedium(ws)
	path := workspaceCryptoMetadataPath(ws)
	if !medium.IsFile(path) {
		return workspaceCryptoMetadata{Version: 1}, nil
	}

	body, err := medium.Read(path)
	if err != nil {
		return workspaceCryptoMetadata{}, coreerr.E("app.readWorkspaceCryptoMetadata", "read workspace crypto metadata failed", err)
	}

	metadata := workspaceCryptoMetadata{}
	result := core.JSONUnmarshalString(body, &metadata)
	if !result.OK {
		return workspaceCryptoMetadata{}, coreerr.E("app.readWorkspaceCryptoMetadata", "parse workspace crypto metadata failed", coreResultError(result))
	}
	if metadata.Version == 0 {
		metadata.Version = 1
	}
	return metadata, nil
}

func writeWorkspaceCryptoMetadata(ws *Workspace, metadata workspaceCryptoMetadata) error {
	if ws == nil {
		return coreerr.E("app.writeWorkspaceCryptoMetadata", "nil workspace", nil)
	}
	if metadata.Version == 0 {
		metadata.Version = 1
	}

	result := core.JSONMarshal(metadata)
	if !result.OK {
		return coreerr.E("app.writeWorkspaceCryptoMetadata", "marshal workspace crypto metadata failed", coreResultError(result))
	}
	body, ok := result.Value.([]byte)
	if !ok {
		return coreerr.E("app.writeWorkspaceCryptoMetadata", "marshal workspace crypto metadata returned non-bytes", nil)
	}

	medium := workspaceMetadataMedium(ws)
	if err := medium.EnsureDir(ws.Root); err != nil {
		return coreerr.E("app.writeWorkspaceCryptoMetadata", "ensure workspace root failed", err)
	}
	if err := medium.WriteMode(workspaceCryptoMetadataPath(ws), string(body), 0600); err != nil {
		return coreerr.E("app.writeWorkspaceCryptoMetadata", "write workspace crypto metadata failed", err)
	}
	return nil
}

func decodeWorkspaceSecretSalt(encoded string) ([]byte, error) {
	result := core.Base64Decode(core.Trim(encoded))
	if !result.OK {
		return nil, coreerr.E("app.decodeWorkspaceSecretSalt", "decode workspace salt failed", coreResultError(result))
	}
	salt, ok := result.Value.([]byte)
	if !ok {
		return nil, coreerr.E("app.decodeWorkspaceSecretSalt", "decoded workspace salt returned non-bytes", nil)
	}
	if len(salt) != workspaceSecretSaltBytes {
		return nil, coreerr.E("app.decodeWorkspaceSecretSalt", "workspace salt length is invalid", nil)
	}
	out := make([]byte, len(salt))
	copy(out, salt)
	return out, nil
}

func workspaceCryptoMetadataPath(ws *Workspace) string {
	if ws == nil {
		return ""
	}
	return ws.Resolve("", workspaceCryptoMetadataFileName)
}

func workspaceMetadataMedium(ws *Workspace) coreio.Medium {
	if ws == nil || ws.medium == nil {
		return coreio.Local
	}
	return ws.medium
}

func coreResultError(result core.Result) error {
	if err, ok := result.Value.(error); ok {
		return err
	}
	return nil
}
