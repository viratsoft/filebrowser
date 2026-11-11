package fbhttp

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/mholt/archives"
)

var publicFolderDownloadSlots = make(chan struct{}, 2)

type publicFolderDownloadPlan struct {
	Paths []string `json:"paths"`
}

var publicFolderDownloadHandler = withShareFile(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "private, no-store")
	folder := d.raw.(*files.FileInfo)
	if d.rawShare.UploadOnly || !folder.IsDir || folder.IsSymlink {
		return http.StatusForbidden, nil
	}
	select {
	case publicFolderDownloadSlots <- struct{}{}:
	default:
		return http.StatusServiceUnavailable, nil
	}
	manifest := r.URL.Query().Get("manifest") == "1"
	plan, entries, err := collectPublicFolderDownload(r.Context(), d, folder.Path, manifest)
	<-publicFolderDownloadSlots
	if err != nil {
		if r.Context().Err() != nil {
			return 0, nil
		}
		return errToStatus(err), err
	}
	if manifest {
		return renderJSON(w, r, plan)
	}
	name := filepath.Base(folder.Path)
	if name == "." || name == string(filepath.Separator) {
		name = filepath.Base(d.rawShare.Path)
	}
	if name == "." || name == "/" || name == "" {
		name = "download"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename*=utf-8''"+url.PathEscape(name+".zip"))
	if err := (archives.Zip{}).Archive(r.Context(), w, entries); err != nil {
		return 0, err
	}
	return 0, nil
}, false, true)

// The manifest and ZIP use the same traversal, so both represent the same
// readable regular files. Directory entries preserve empty folders in ZIP.
func collectPublicFolderDownload(ctx context.Context, d *data, root string, manifest bool) (publicFolderDownloadPlan, []archives.FileInfo, error) {
	plan := publicFolderDownloadPlan{Paths: []string{}}
	var entries []archives.FileInfo
	pending := []string{root}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if err := ctx.Err(); err != nil {
			return plan, nil, err
		}
		dir, err := d.user.Fs.Open(current)
		if err != nil {
			return plan, nil, err
		}
		for {
			children, readErr := dir.Readdir(256)
			for _, info := range children {
				if err := ctx.Err(); err != nil {
					_ = dir.Close()
					return plan, nil, err
				}
				name := filepath.Join(current, info.Name())
				canonical := slashClean(name)
				if strings.Contains(info.Name(), "\\") || !d.Check(canonical) || isPublicUploadTempPath(canonical) {
					continue
				}
				switch {
				case info.IsDir():
					pending = append(pending, name)
				case !info.Mode().IsRegular():
					continue
				case manifest:
					plan.Paths = append(plan.Paths, canonical)
				}
				if !manifest {
					relative, err := filepath.Rel(root, name)
					if err != nil {
						_ = dir.Close()
						return plan, nil, err
					}
					entries = append(entries, archives.FileInfo{
						FileInfo:      info,
						NameInArchive: filepath.ToSlash(relative),
						Open:          func() (fs.File, error) { return d.user.Fs.Open(name) },
					})
				}
			}
			if readErr == io.EOF || (readErr == nil && len(children) == 0) {
				break
			}
			if readErr != nil {
				_ = dir.Close()
				return plan, nil, readErr
			}
		}
		if err := dir.Close(); err != nil {
			return plan, nil, err
		}
	}
	return plan, entries, nil
}
