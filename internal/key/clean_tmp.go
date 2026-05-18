package key

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"wxview/internal/app"
)

const cacheTmpMinAge = 15 * time.Minute

type CleanTmpResult struct {
	Path         string `json:"path"`
	Removed      int    `json:"removed"`
	RemovedBytes int64  `json:"removed_bytes"`
	Kept         int    `json:"kept"`
}

func CleanTmp() (CleanTmpResult, error) {
	base, err := app.BaseDir()
	if err != nil {
		return CleanTmpResult{}, err
	}
	root := filepath.Join(base, "cache")
	result := CleanTmpResult{Path: root}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	now := time.Now()
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "index" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".tmp") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if now.Sub(info.ModTime()) < cacheTmpMinAge {
			result.Kept++
			return nil
		}
		result.RemovedBytes += info.Size()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		result.Removed++
		return nil
	})
	return result, err
}
