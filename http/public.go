package fbhttp

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/spf13/afero"
	"golang.org/x/crypto/bcrypt"
)

var withHashFile = func(fn handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		id, ifPath := ifPathWithName(r)
		link, err := d.store.Share.GetByHash(id)
		if err != nil {
			return errToStatus(err), err
		}

		status, err := authenticateShareRequest(r, link)
		if status != 0 || err != nil {
			return status, err
		}
		if link.UploadOnly {
			// An upload-only share has one readable endpoint: its root listing.
			// File metadata and content remain unavailable even to the visitor that
			// uploaded the file, so guessing a filename cannot become a read path.
			if ifPath != "/" {
				return http.StatusForbidden, nil
			}
		}

		user, err := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, link.UserID)
		if err != nil {
			return errToStatus(err), err
		}

		if !user.Perm.Share || !user.Perm.Download {
			return http.StatusForbidden, nil
		}

		d.user = user

		file, err := files.NewFileInfo(&files.FileOptions{
			Fs:         d.user.Fs,
			Path:       link.Path,
			Modify:     d.user.Perm.Modify,
			Expand:     false,
			ReadHeader: d.server.TypeDetectionByHeader,
			CalcImgRes: d.server.TypeDetectionByHeader,
			Checker:    d,
			Token:      link.Token,
		})
		if err != nil {
			return errToStatus(err), err
		}
		if link.UploadOnly && !file.IsDir {
			// Keep the database invariant enforced at request time as well. This
			// protects old or manually edited records that predate API validation.
			return http.StatusForbidden, nil
		}

		// share base path. Canonicalized because it roots both the rebased
		// filesystem and checkerPrefix below, and a stored path that is not
		// "/"-separated would make the two disagree on Windows.
		basePath := slashClean(link.Path)

		// file relative path
		filePath := ""

		if file.IsDir {
			filePath = ifPath
		}

		// set fs root to the shared file/folder. Unless external symlinks are
		// explicitly allowed, this is a ScopedFs (not a bare BasePathFs) so the
		// share is also symlink-confined: a link inside the shared subtree that
		// points elsewhere in the owner's scope — outside the share — must not be
		// followed.
		d.user.Fs = files.NewFs(d.user.Fs, basePath, d.server.FollowExternalSymlinks)

		// the filesystem is now rebased onto basePath, so paths handed to the
		// rule checker are relative to it. Resolve them back to the user's
		// original scope so deny rules below the share root keep applying.
		d.checkerPrefix = basePath

		file, err = files.NewFileInfo(&files.FileOptions{
			Fs:      d.user.Fs,
			Path:    filePath,
			Modify:  d.user.Perm.Modify,
			Expand:  true,
			Checker: d,
			Token:   link.Token,
		})
		if err != nil {
			return errToStatus(err), err
		}

		if file.IsDir {
			// extract name from the last directory in the path
			name := filepath.Base(strings.TrimRight(link.Path, string(filepath.Separator)))
			file.Name = name
		}

		d.raw = file
		d.rawShare = link
		return fn(w, r, d)
	}
}

// ref to https://github.com/filebrowser/filebrowser/pull/727
// `/api/public/dl/MEEuZK-v/file-name.txt` for old browsers to save file with correct name
func ifPathWithName(r *http.Request) (id, filePath string) {
	pathElements := strings.Split(r.URL.Path, "/")
	// prevent maliciously constructed parameters like `/api/public/dl/XZzCDnK2_not_exists_hash_name`
	// len(pathElements) will be 1, and golang will panic `runtime error: index out of range`

	switch len(pathElements) {
	case 1:
		return r.URL.Path, "/"
	default:
		// Public share routes do not pass through withUser, so canonicalize the
		// share-relative path here instead.
		return pathElements[0], slashClean(path.Join(pathElements[1:]...))
	}
}

var publicShareHandler = withHashFile(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	file := d.raw.(*files.FileInfo)

	if file.IsDir {
		file.Sorting = files.Sorting{By: "name", Asc: false}
		file.ApplySort()
		if d.rawShare.UploadOnly {
			// The response is visitor-session-specific. Never allow a browser or
			// intermediary cache to replay one visitor's listing to another.
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Vary", "Cookie")
			session, err := publicUploadSessions.session(w, r, d.rawShare.Hash, d.settings.Key, d.rawShare.SessionUploadFolder)
			if err != nil {
				return uploadSessionErrorStatus(err), err
			}
			if session.Folder != "" {
				file, err = visitorSessionFolderInfo(d, file, session.Folder)
				if err != nil {
					return errToStatus(err), err
				}
			} else {
				allowed := publicUploadSessions.files(d.rawShare.Hash, session)
				items := file.Items[:0]
				for _, item := range file.Items {
					if _, ok := allowed[item.Name]; ok {
						items = append(items, item)
					}
				}
				file.Items, file.NumFiles, file.NumDirs = items, len(items), 0
			}
		}
	}

	return renderJSON(w, r, struct {
		*files.FileInfo
		AllowUpload bool `json:"allowUpload"`
		UploadOnly  bool `json:"uploadOnly"`
	}{FileInfo: file, AllowUpload: d.rawShare.AllowUpload, UploadOnly: d.rawShare.UploadOnly})
})

// publicUploadHandler accepts a new file at the root of an upload-enabled
// directory share. It does not accept nested paths or overwrites, keeping
// anonymous access limited to adding files only.
var publicUploadHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	id, relativePath := ifPathWithName(r)
	if id == "" || relativePath == "/" || strings.Contains(relativePath, "\\") || strings.Contains(strings.TrimPrefix(relativePath, "/"), "/") {
		return http.StatusBadRequest, nil
	}

	link, err := d.store.Share.GetByHash(id)
	if err != nil {
		return errToStatus(err), err
	}
	if !link.AllowUpload {
		return http.StatusForbidden, nil
	}
	if status, err := authenticateShareRequest(r, link); status != 0 || err != nil {
		return status, err
	}
	session, err := publicUploadSessions.session(w, r, link.Hash, d.settings.Key, link.SessionUploadFolder)
	if err != nil {
		return uploadSessionErrorStatus(err), err
	}

	user, err := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, link.UserID)
	if err != nil {
		return errToStatus(err), err
	}
	if !user.Perm.Share || !user.Perm.Download || !user.Perm.Create {
		return http.StatusForbidden, nil
	}

	d.user = user
	root, err := files.NewFileInfo(&files.FileOptions{Fs: d.user.Fs, Path: link.Path, Checker: d})
	if err != nil {
		return errToStatus(err), err
	}
	if !root.IsDir {
		return http.StatusBadRequest, nil
	}
	var folderOwner publicUploadOwner
	if link.MatchFolderOwner {
		rootInfo, err := d.user.Fs.Stat(link.Path)
		if err != nil {
			return errToStatus(err), err
		}
		folderOwner, err = publicUploadOwnerFor(rootInfo)
		if err != nil {
			return http.StatusInternalServerError, err
		}
	}

	basePath := slashClean(link.Path)
	d.user.Fs = files.NewFs(d.user.Fs, basePath, d.server.FollowExternalSymlinks)
	d.checkerPrefix = basePath
	targetPath := relativePath
	if session.Folder != "" {
		targetPath = "/" + session.Folder + relativePath
	}
	if !d.Check(targetPath) {
		return http.StatusForbidden, nil
	}
	if exists, err := afero.Exists(d.user.Fs, targetPath); err != nil {
		return http.StatusInternalServerError, err
	} else if exists {
		return http.StatusConflict, nil
	}

	created := false
	folderCreated := false
	err = d.RunHook(func() error {
		if session.Folder != "" {
			var folderErr error
			folderCreated, folderErr = createVisitorSessionFolder(d.user.Fs, session.Folder, d.settings.DirMode)
			if folderErr != nil {
				return folderErr
			}
			if link.MatchFolderOwner {
				if folderErr = applyPublicUploadOwner(d.user.Fs, "/"+session.Folder, folderOwner); folderErr != nil {
					return folderErr
				}
			}
		}
		var writeErr error
		created, writeErr = writeNewPublicUpload(d.user.Fs, targetPath, r.Body, d.settings.FileMode)
		if writeErr != nil || !link.MatchFolderOwner {
			return writeErr
		}
		return applyPublicUploadOwner(d.user.Fs, targetPath, folderOwner)
	}, "upload", targetPath, "", d.user)
	if err != nil && created {
		_ = d.user.Fs.RemoveAll(targetPath)
	}
	if err != nil && folderCreated {
		// Remove only an empty folder. A concurrent upload for the same visitor
		// may already have put a file there, and must never be removed here.
		_ = d.user.Fs.Remove("/" + session.Folder)
	}
	if err == nil {
		publicUploadSessions.add(link.Hash, session, strings.TrimPrefix(targetPath, "/"))
	}
	return errToStatus(err), err
}

func uploadSessionErrorStatus(err error) int {
	if errors.Is(err, errTooManyPublicUploadSessions) {
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func createVisitorSessionFolder(afs afero.Fs, folder string, mode os.FileMode) (bool, error) {
	err := afs.Mkdir("/"+folder, mode)
	if err == nil {
		return true, nil
	}
	if os.IsExist(err) {
		return false, nil
	}
	return false, err
}

var publicDlHandler = withHashFile(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	file := d.raw.(*files.FileInfo)
	if d.rawShare.UploadOnly {
		return http.StatusForbidden, nil
	}
	if !file.IsDir {
		return rawFileHandler(w, r, file)
	}

	return rawDirHandler(w, r, d, file)
})

func visitorSessionFolderInfo(d *data, root *files.FileInfo, folder string) (*files.FileInfo, error) {
	folderPath := "/" + folder
	exists, err := afero.Exists(d.user.Fs, folderPath)
	if err != nil {
		return nil, err
	}
	if !exists {
		// JSON must contain [] rather than null: public clients always receive a
		// listing and safely call .map on it before the visitor's first upload.
		root.Items, root.NumFiles, root.NumDirs = []*files.FileInfo{}, 0, 0
		return root, nil
	}

	info, err := files.NewFileInfo(&files.FileOptions{
		Fs:      d.user.Fs,
		Path:    folderPath,
		Modify:  d.user.Perm.Modify,
		Expand:  true,
		Checker: d,
		Token:   d.rawShare.Token,
	})
	if err != nil {
		return nil, err
	}
	if !info.IsDir {
		return nil, errors.New("visitor session path is not a directory")
	}
	info.Name = root.Name
	info.Sorting = files.Sorting{By: "name", Asc: false}
	info.ApplySort()
	return info, nil
}

// writeNewPublicUpload creates exactly one new file. O_EXCL closes the gap
// between a pre-flight existence check and the write, so an upload can never
// overwrite a file created concurrently by the owner or another visitor.
func writeNewPublicUpload(afs afero.Fs, dst string, body io.Reader, mode os.FileMode) (bool, error) {
	file, err := afs.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return false, err
	}

	if _, err := io.Copy(file, body); err != nil {
		_ = file.Close()
		return true, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return true, err
	}
	if err := file.Close(); err != nil {
		return true, err
	}
	return true, nil
}

func authenticateShareRequest(r *http.Request, l *share.Link) (int, error) {
	if l.PasswordHash == "" {
		return 0, nil
	}

	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(l.Token)) == 1 {
		return 0, nil
	}

	password := r.Header.Get("X-SHARE-PASSWORD")
	password, err := url.QueryUnescape(password)
	if err != nil {
		return 0, err
	}
	if password == "" {
		return http.StatusUnauthorized, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(l.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return http.StatusUnauthorized, nil
		}
		return 0, err
	}

	return 0, nil
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"OK"}`))
}
