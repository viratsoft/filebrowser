package fbhttp

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/spf13/afero"
)

// publicTusUpload is deliberately derived on every TUS request. In particular,
// the URL is not authority: the share password/token and signed visitor cookie
// are checked again for POST, HEAD and PATCH before a path is touched.
type publicTusUpload struct {
	link       *share.Link
	session    uploadSession
	targetPath string
	tempPath   string
	cacheKey   string
	owner      publicUploadOwner
}

func publicTusTarget(w http.ResponseWriter, r *http.Request, d *data) (*publicTusUpload, int, error) {
	id, relativePath := ifPathWithName(r)
	if id == "" || relativePath == "/" || strings.Contains(relativePath, "\\") || strings.Contains(strings.TrimPrefix(relativePath, "/"), "/") {
		return nil, http.StatusBadRequest, nil
	}
	link, err := d.store.Share.GetByHash(id)
	if err != nil {
		return nil, errToStatus(err), err
	}
	if !link.AllowUpload {
		return nil, http.StatusForbidden, nil
	}
	if status, err := authenticateShareRequest(r, link); status != 0 || err != nil {
		return nil, status, err
	}
	session, err := publicUploadSessions.session(w, r, link.Hash, d.settings.Key, link.SessionUploadFolder)
	if err != nil {
		return nil, uploadSessionErrorStatus(err), err
	}
	user, err := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, link.UserID)
	if err != nil {
		return nil, errToStatus(err), err
	}
	if !user.Perm.Share || !user.Perm.Download || !user.Perm.Create {
		return nil, http.StatusForbidden, nil
	}
	d.user = user
	root, err := files.NewFileInfo(&files.FileOptions{Fs: d.user.Fs, Path: link.Path, Checker: d})
	if err != nil {
		return nil, errToStatus(err), err
	}
	if !root.IsDir {
		return nil, http.StatusBadRequest, nil
	}

	result := &publicTusUpload{link: link, session: session}
	if link.MatchFolderOwner {
		rootInfo, err := d.user.Fs.Stat(link.Path)
		if err != nil {
			return nil, errToStatus(err), err
		}
		result.owner, err = publicUploadOwnerFor(rootInfo)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
	}

	basePath := slashClean(link.Path)
	d.user.Fs = files.NewFs(d.user.Fs, basePath, d.server.FollowExternalSymlinks)
	d.checkerPrefix = basePath
	result.targetPath = relativePath
	if session.Folder != "" {
		result.targetPath = "/" + session.Folder + relativePath
	}
	if !d.Check(result.targetPath) {
		return nil, http.StatusForbidden, nil
	}
	// Include the signed visitor identity. Two visitors must never resume or
	// complete one another's transfer, even if they choose the same filename.
	result.cacheKey = "public-tus:" + link.Hash + ":" + session.ID + ":" + result.targetPath
	// Keep incomplete content outside the visitor listing. It is published with
	// an atomic rename only after the final PATCH has been synced.
	result.tempPath = path.Join(path.Dir(result.targetPath), ".filebrowser-upload-"+session.ID[:16]+"-"+path.Base(result.targetPath))
	return result, 0, nil
}

func publicTusPostHandler(cache UploadCache) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		upload, status, err := publicTusTarget(w, r, d)
		if status != 0 || err != nil {
			return status, err
		}
		length, err := getUploadLength(r)
		if err != nil || length < 0 {
			return http.StatusBadRequest, fmt.Errorf("invalid upload length: %w", err)
		}
		if exists, err := afero.Exists(d.user.Fs, upload.targetPath); err != nil {
			return http.StatusInternalServerError, err
		} else if exists {
			return http.StatusConflict, nil
		}

		folderCreated := false
		if upload.session.Folder != "" {
			folderCreated, err = createVisitorSessionFolder(d.user.Fs, upload.session.Folder, d.settings.DirMode)
			if err != nil {
				return errToStatus(err), err
			}
			if upload.link.MatchFolderOwner {
				if err = applyPublicUploadOwner(d.user.Fs, "/"+upload.session.Folder, upload.owner); err != nil {
					if folderCreated {
						_ = d.user.Fs.Remove("/" + upload.session.Folder)
					}
					return http.StatusInternalServerError, err
				}
			}
		}
		file, err := d.user.Fs.OpenFile(upload.tempPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, d.settings.FileMode)
		if err != nil {
			if folderCreated {
				_ = d.user.Fs.Remove("/" + upload.session.Folder)
			}
			if os.IsExist(err) {
				return http.StatusConflict, nil
			}
			return errToStatus(err), err
		}
		if err = file.Close(); err != nil {
			_ = d.user.Fs.Remove(upload.tempPath)
			return http.StatusInternalServerError, err
		}
		if upload.link.MatchFolderOwner {
			if err = applyPublicUploadOwner(d.user.Fs, upload.tempPath, upload.owner); err != nil {
				_ = d.user.Fs.Remove(upload.tempPath)
				if folderCreated {
					_ = d.user.Fs.Remove("/" + upload.session.Folder)
				}
				return http.StatusInternalServerError, err
			}
		}
		cache.Register(upload.cacheKey, length, func() error { return d.user.Fs.Remove(upload.tempPath) })
		if length == 0 {
			if err := d.user.Fs.Rename(upload.tempPath, upload.targetPath); err != nil {
				return http.StatusInternalServerError, err
			}
			cache.Complete(upload.cacheKey)
			publicUploadSessions.add(upload.link.Hash, upload.session, strings.TrimPrefix(upload.targetPath, "/"))
		}
		basePath := "/" + strings.Trim(strings.TrimSpace(d.server.BaseURL), "/")
		if basePath == "/" {
			basePath = ""
		}
		w.Header().Set("Location", basePath+"/api/public/tus/"+url.PathEscape(upload.link.Hash)+"/"+url.PathEscape(strings.TrimPrefix(relativePathFromTarget(upload), "/")))
		return http.StatusCreated, nil
	}
}

func relativePathFromTarget(upload *publicTusUpload) string {
	if upload.session.Folder != "" {
		return strings.TrimPrefix(upload.targetPath, "/"+upload.session.Folder)
	}
	return upload.targetPath
}

func publicTusHeadHandler(cache UploadCache) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		w.Header().Set("Cache-Control", "no-store")
		upload, status, err := publicTusTarget(w, r, d)
		if status != 0 || err != nil {
			return status, err
		}
		length, err := cache.GetLength(upload.cacheKey)
		if err != nil {
			return http.StatusNotFound, nil
		}
		info, err := d.user.Fs.Stat(upload.tempPath)
		if err != nil || info.IsDir() {
			return http.StatusNotFound, nil
		}
		w.Header().Set("Upload-Offset", strconv.FormatInt(info.Size(), 10))
		w.Header().Set("Upload-Length", strconv.FormatInt(length, 10))
		return http.StatusOK, nil
	}
}

func publicTusPatchHandler(cache UploadCache) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		upload, status, err := publicTusTarget(w, r, d)
		if status == 0 && err == nil {
			status, err = publicTusPatchUpload(w, r, d, cache, upload)
		}
		if status >= 400 {
			drainRequestBody(r)
		}
		return status, err
	}
}

func publicTusPatchUpload(w http.ResponseWriter, r *http.Request, d *data, cache UploadCache, upload *publicTusUpload) (int, error) {
	if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
		return http.StatusUnsupportedMediaType, nil
	}
	offset, err := getUploadOffset(r)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("invalid upload offset")
	}
	length, err := cache.GetLength(upload.cacheKey)
	if err != nil {
		return http.StatusNotFound, nil
	}
	if offset > length {
		return http.StatusBadRequest, fmt.Errorf("upload offset exceeds declared length")
	}
	info, err := d.user.Fs.Stat(upload.tempPath)
	if os.IsNotExist(err) {
		return http.StatusNotFound, nil
	}
	if err != nil {
		return errToStatus(err), err
	}
	if info.IsDir() {
		return http.StatusBadRequest, nil
	}
	if info.Size() != offset {
		return http.StatusConflict, nil
	}
	stop := keepUploadActive(cache, upload.cacheKey)
	defer stop()
	file, err := d.user.Fs.OpenFile(upload.tempPath, os.O_WRONLY|os.O_APPEND, d.settings.FileMode)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer file.Close()
	remaining := length - offset
	written, err := io.Copy(file, io.LimitReader(r.Body, remaining+1))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if written > remaining {
		if truncErr := file.Truncate(offset); truncErr != nil {
			return http.StatusInternalServerError, truncErr
		}
		return http.StatusRequestEntityTooLarge, nil
	}
	if err = file.Sync(); err != nil {
		return http.StatusInternalServerError, err
	}
	newOffset := offset + written
	w.Header().Set("Upload-Offset", strconv.FormatInt(newOffset, 10))
	if newOffset == length {
		if err := d.user.Fs.Rename(upload.tempPath, upload.targetPath); err != nil {
			return http.StatusInternalServerError, err
		}
		cache.Complete(upload.cacheKey)
		publicUploadSessions.add(upload.link.Hash, upload.session, strings.TrimPrefix(upload.targetPath, "/"))
		_ = d.RunHook(func() error { return nil }, "upload", upload.targetPath, "", d.user)
	}
	return http.StatusNoContent, nil
}
