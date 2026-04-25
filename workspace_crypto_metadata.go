// SPDX-License-Identifier: EUPL-1.2

package app

import (
	"crypto/rand" // Note: AX-6 - workspace KDF salts require cryptographic randomness; no core CSPRNG wrapper exists.
	"errors"
	"io"
	"io/fs"
	"os" // Note: AX-6 - O_CREATE|O_EXCL is required for atomic salt-file creation; no core primitive exists.
	"time"

	core "dappco.re/go/core"
	coreio "dappco.re/go/io"
	coreerr "dappco.re/go/log"
)

const (
	workspaceCryptoMetadataFileName = "workspace.crypto.json"
	workspaceSecretSaltBytes        = 32
	workspaceSecretSaltReadAttempts = 100
	workspaceSecretSaltReadDelay    = time.Millisecond
)

var (
	openWorkspaceCryptoMetadataFile  = os.OpenFile
	writeWorkspaceCryptoMetadataBody = func(w io.Writer, body string) error {
		_, err := io.WriteString(w, body)
		return err
	}
)

type workspaceCryptoMetadata struct {
	Version int    `json:"version"`
	Salt    string `json:"salt"`
}

func ensureWorkspaceSecretSalt(ws *Workspace) ([]byte, error) {
	metadata, err := readWorkspaceCryptoMetadata(ws)
	if err != nil {
		if workspaceCryptoMetadataFileExists(ws) {
			if salt, retryErr := readWorkspaceSecretSaltAfterCreateRace(ws); retryErr == nil {
				return salt, nil
			}
		}
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
		if errors.Is(err, fs.ErrExist) {
			return readWorkspaceSecretSaltAfterCreateRace(ws)
		}
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
	if err := createWorkspaceCryptoMetadata(medium, workspaceCryptoMetadataPath(ws), string(body)); err != nil {
		return coreerr.E("app.writeWorkspaceCryptoMetadata", "write workspace crypto metadata failed", err)
	}
	return nil
}

func createWorkspaceCryptoMetadata(medium coreio.Medium, path, body string) error {
	if medium != coreio.Local {
		if medium.IsFile(path) {
			return fs.ErrExist
		}
		return medium.WriteMode(path, body, 0600)
	}

	file, err := openWorkspaceCryptoMetadataFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := writeWorkspaceCryptoMetadataBody(file, body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func readWorkspaceSecretSaltAfterCreateRace(ws *Workspace) ([]byte, error) {
	var lastErr error
	for i := 0; i < workspaceSecretSaltReadAttempts; i++ {
		metadata, err := readWorkspaceCryptoMetadata(ws)
		if err == nil && core.Trim(metadata.Salt) != "" {
			salt, err := decodeWorkspaceSecretSalt(metadata.Salt)
			if err == nil {
				return salt, nil
			}
			lastErr = err
		} else if err != nil {
			lastErr = err
		}

		if i+1 < workspaceSecretSaltReadAttempts {
			time.Sleep(workspaceSecretSaltReadDelay)
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, coreerr.E("app.ensureWorkspaceSecretSalt", "workspace salt race winner did not persist salt", nil)
}

func workspaceCryptoMetadataFileExists(ws *Workspace) bool {
	if ws == nil {
		return false
	}
	return workspaceMetadataMedium(ws).IsFile(workspaceCryptoMetadataPath(ws))
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
