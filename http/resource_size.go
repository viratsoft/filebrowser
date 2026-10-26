package fbhttp

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// scanFolderDetails reads directory metadata in bounded, unsorted batches.
// Readdir supplies file sizes without another scoped Stat for every file.
// All directory opens still use the user's scoped filesystem; rules apply to
// every entry. Symlinks are excluded rather than followed or counted twice.
func scanFolderDetails(ctx context.Context, afs afero.Fs, root string, check func(string) bool) (resourceSizeResponse, error) {
	var result resourceSizeResponse
	pending := []string{root}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		dir, err := afs.Open(current)
		if err != nil {
			return result, err
		}
		err = readFolderDetails(ctx, dir, current, check, &result, &pending)
		closeErr := dir.Close()
		if err != nil {
			return result, err
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	return result, nil
}

func readFolderDetails(ctx context.Context, dir afero.File, current string, check func(string) bool, result *resourceSizeResponse, pending *[]string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := dir.Readdir(256)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			path := filepath.Join(current, entry.Name())
			if !check(path) || entry.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if entry.IsDir() {
				*pending = append(*pending, path)
			} else if entry.Mode().IsRegular() {
				result.Size += entry.Size()
				result.NumFiles++
			}
		}
		if errors.Is(err, io.EOF) || len(entries) == 0 {
			return nil
		}
	}
}
