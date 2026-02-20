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

const (
	maxFileSize    = 100 * 1024 * 1024  // 100 MB per file
	maxTotalSize   = 1024 * 1024 * 1024 // 1 GB total extracted
	maxFileCount   = 50000              // maximum number of files in archive
)

// ExtractZip unpacks a zip archive to a temp directory.
// Returns the path to the extracted directory and a cleanup function
// that removes the temp directory.
// Validates all paths to prevent zip-slip (path traversal) attacks.
// Enforces size limits to prevent zip bomb attacks.
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

	if len(reader.File) > maxFileCount {
		cleanupFn()
		return "", nil, fmt.Errorf("zip archive contains %d files, exceeds maximum of %d", len(reader.File), maxFileCount)
	}

	var totalExtracted int64

	for _, file := range reader.File {
		// Skip symlinks to prevent symlink-based attacks.
		if file.Mode()&os.ModeSymlink != 0 {
			continue
		}

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

		limitedReader := io.LimitReader(rc, maxFileSize+1)
		n, err := io.Copy(outFile, limitedReader)
		if err != nil {
			outFile.Close()
			rc.Close()
			cleanupFn()
			return "", nil, fmt.Errorf("failed to extract %s: %w", file.Name, err)
		}
		if n > maxFileSize {
			outFile.Close()
			rc.Close()
			cleanupFn()
			return "", nil, fmt.Errorf("file %s exceeds maximum size of %d bytes", file.Name, maxFileSize)
		}

		totalExtracted += n
		if totalExtracted > maxTotalSize {
			outFile.Close()
			rc.Close()
			cleanupFn()
			return "", nil, fmt.Errorf("total extracted size exceeds maximum of %d bytes", maxTotalSize)
		}

		outFile.Close()
		rc.Close()
	}

	return tmpDir, cleanupFn, nil
}
