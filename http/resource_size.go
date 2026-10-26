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
			if current != root && isFolderReadError(err) {
				result.Incomplete = true
				continue
			}
			return result, err
		}
		err = readFolderDetails(ctx, dir, current, check, &result, &pending)
		closeErr := dir.Close()
		if err != nil {
			if isFolderReadError(err) {
				result.Incomplete = true
				continue
			}
			return result, err
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	return result, nil
}

// A concurrent delete, mount failure or unreadable descendant must not throw
// away all the readable totals. Never call such a result complete, and never
// include the failed server-side path in the response. Root-open errors still
// propagate so a wholly inaccessible target gets its proper 403/404 status.
func isFolderReadError(err error) bool {
	var pathError *os.PathError
	return errors.As(err, &pathError) || errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist)
}

func readFolderDetails(ctx context.Context, dir afero.File, current string, check func(string) bool, result *resourceSizeResponse, pending *[]string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.Readdir(256)
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
		// Readdir may return valid entries together with an error. Preserve
		// those entries before handling the error; the caller marks it partial.
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if errors.Is(readErr, io.EOF) || len(entries) == 0 {
			return nil
		}
	}
}
