package fbhttp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/spf13/afero"
)

var publicFolderSizeSlots = make(chan struct{}, 2)

// Check every requested directory component before scanning. A directly
// requested path through a link must not bypass rules on its real target.
func checkRealShareDirectories(fs afero.Fs, name string, check func(string) bool) error {
	if !check(name) || isPublicUploadTempPath(name) {
		return os.ErrPermission
	}
	if name == "/" {
		return nil
	}
	lstater, ok := fs.(afero.Lstater)
	if !ok {
		return os.ErrPermission
	}
	current := "/"
	for _, segment := range strings.Split(strings.Trim(name, "/"), "/") {
		current = path.Join(current, segment)
		if !check(current) {
			return os.ErrPermission
		}
		entry, usedLstat, err := lstater.LstatIfPossible(current)
		if err != nil {
			return err
		}
		if !usedLstat || !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 {
			return os.ErrPermission
		}
	}
	return nil
}

var publicFolderSizeHandler = withShareFile(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "private, no-store")
	folder := d.raw.(*files.FileInfo)
	if d.rawShare.UploadOnly || !folder.IsDir || folder.IsSymlink {
		return http.StatusForbidden, nil
	}

	select {
	case publicFolderSizeSlots <- struct{}{}:
		defer func() { <-publicFolderSizeSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		return http.StatusServiceUnavailable, nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	details, err := scanFolderDetails(ctx, folder.Fs, folder.Path, func(name string) bool {
		return !isPublicUploadTempPath(name) && d.Check(name)
	})
	switch {
	case r.Context().Err() != nil:
		return 0, r.Context().Err()
	case errors.Is(err, context.DeadlineExceeded):
		details.Incomplete = true
	case err != nil:
		return errToStatus(err), err
	}
	return renderJSON(w, r, struct {
		Size       int64 `json:"size"`
		Incomplete bool  `json:"incomplete,omitempty"`
	}{Size: details.Size, Incomplete: details.Incomplete})
}, false, true)
