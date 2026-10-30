// Package fsutil provides durable, permission-safe file writes.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path through a temporary file in the same
// directory, syncs the file and the directory, and renames it into place.
// The temporary file is created with O_EXCL and the final file has perm.
// A reader never observes a partially written file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to set file permissions: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to sync file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to replace file: %w", err)
	}
	cleanup = false

	// Make the rename itself durable. Directory sync is best-effort on
	// platforms that do not support it.
	if d, err := os.Open(filepath.Clean(dir)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// EnsurePrivateDir creates dir when missing and tightens an existing
// directory to perm. It never widens permissions.
func EnsurePrivateDir(dir string, perm os.FileMode) error {
	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}
	return os.Chmod(dir, perm)
}

// TightenFile restricts an existing regular file to perm. Missing files are
// not an error. Symlinks are left alone so a link cannot redirect a chmod.
func TightenFile(path string, perm os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil
	}
	if info.Mode().Perm() == perm {
		return nil
	}
	return os.Chmod(path, perm)
}
