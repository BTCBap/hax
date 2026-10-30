package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BTCBap/hax/internal/fsutil"
)

// writePrivateExport writes data into dir with owner-only permissions and
// avoids overwriting an existing file by adding a numeric suffix.
func writePrivateExport(dir, name string, data []byte) (string, error) {
	if err := fsutil.EnsurePrivateDir(dir, 0o700); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	path := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		} else if err != nil {
			return "", err
		}
		if i > 100 {
			return "", fmt.Errorf("too many existing exports named %s", name)
		}
		path = filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
	}
	if err := fsutil.WriteFileAtomic(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
