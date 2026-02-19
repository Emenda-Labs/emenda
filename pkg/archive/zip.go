package archive

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractZip unpacks a zip archive to a temp directory.
// Returns the path to the extracted directory and a cleanup function
// that removes the temp directory.
// Validates all paths to prevent zip-slip (path traversal) attacks.
func ExtractZip(data []byte, prefix string) (dir string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "emenda-"+prefix+"-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cleanupFn := func() { os.RemoveAll(tmpDir) }

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		cleanupFn()
		return "", nil, fmt.Errorf("failed to read zip archive: %w", err)
	}

	for _, file := range reader.File {
		target := filepath.Join(tmpDir, file.Name)

		// Zip-slip protection: ensure resolved path is within tmpDir
		resolvedTarget, err := filepath.Abs(target)
		if err != nil {
			cleanupFn()
			return "", nil, fmt.Errorf("failed to resolve path %s: %w", file.Name, err)
		}
		resolvedBase, err := filepath.Abs(tmpDir)
		if err != nil {
			cleanupFn()
			return "", nil, fmt.Errorf("failed to resolve base path: %w", err)
		}
		if !strings.HasPrefix(resolvedTarget, resolvedBase+string(os.PathSeparator)) && resolvedTarget != resolvedBase {
			cleanupFn()
			return "", nil, fmt.Errorf("zip entry attempts path traversal: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				cleanupFn()
				return "", nil, fmt.Errorf("failed to create directory %s: %w", file.Name, err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			cleanupFn()
			return "", nil, fmt.Errorf("failed to create parent directory for %s: %w", file.Name, err)
		}

		rc, err := file.Open()
		if err != nil {
			cleanupFn()
			return "", nil, fmt.Errorf("failed to open zip entry %s: %w", file.Name, err)
		}

		outFile, err := os.Create(target)
		if err != nil {
			rc.Close()
			cleanupFn()
			return "", nil, fmt.Errorf("failed to create file %s: %w", file.Name, err)
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			cleanupFn()
			return "", nil, fmt.Errorf("failed to extract %s: %w", file.Name, err)
		}

		outFile.Close()
		rc.Close()
	}

	return tmpDir, cleanupFn, nil
}
