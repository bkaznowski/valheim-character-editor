package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// backupBeforeOverwrite copies the file at path to a timestamped sibling
// (matching Valheim's own "<name>_backup_YYYYMMDD-HHMMSS.fch" naming
// convention, so it shows up right alongside the game's own auto-backups)
// before it gets overwritten by a save. No-op if the file doesn't exist
// yet (nothing to back up).
func backupBeforeOverwrite(path string) error {
	src, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer src.Close()

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	backupPath := fmt.Sprintf("%s_backup_%s%s", base, time.Now().Format("20060102-150405"), ext)

	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(backupPath)
		return err
	}
	return nil
}
