package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
)

type folderDetailsCountingFs struct {
	afero.Fs
	stats int
}

func (f *folderDetailsCountingFs) Stat(name string) (os.FileInfo, error) {
	f.stats++
	return f.Fs.Stat(name)
}

type folderDetailsErrorFs struct {
	afero.Fs
	openPath string
	openErr  error
	readErr  error
}

func (f *folderDetailsErrorFs) Open(name string) (afero.File, error) {
	if f.openErr != nil && filepath.Clean(name) == filepath.Clean(f.openPath) {
		return nil, f.openErr
	}
	file, err := f.Fs.Open(name)
	if err == nil && f.readErr != nil {
		return &folderDetailsErrorFile{File: file, err: f.readErr}, nil
	}
	return file, err
}

type folderDetailsErrorFile struct {
	afero.File
	err error
}

func (f *folderDetailsErrorFile) Readdir(count int) ([]os.FileInfo, error) {
	entries, err := f.File.Readdir(count)
	if err != nil && len(entries) == 0 {
		return entries, err
	}
	return entries, f.err
}

func TestFolderDetailsScanMarksUnreadableDescendantsPartial(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := base.MkdirAll("/target/unreadable", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(base, "/target/normal.txt", []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(base, "/target/unreadable/missing.txt", []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, cause := range []error{os.ErrPermission, os.ErrNotExist, errors.New("mount unavailable")} {
		t.Run(cause.Error(), func(t *testing.T) {
			failed := &folderDetailsErrorFs{Fs: base, openPath: "/target/unreadable", openErr: &os.PathError{Op: "open", Path: "/target/unreadable", Err: cause}}
			result, err := scanFolderDetails(context.Background(), failed, "/target", func(string) bool { return true })
			if err != nil || result.Size != 3 || result.NumFiles != 1 || !result.Incomplete {
				t.Fatalf("partial result=%+v err=%v", result, err)
			}
			// Root access errors are not disguised as a successful partial result.
			failed.openPath = "/target"
			if _, err := scanFolderDetails(context.Background(), failed, "/target", func(string) bool { return true }); !errors.Is(err, cause) {
				t.Fatalf("expected root error %v, got %v", cause, err)
			}
		})
	}
}

func TestFolderDetailsScanPreservesEntriesReturnedWithReadError(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := base.MkdirAll("/target", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(base, "/target/normal.txt", []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	failed := &folderDetailsErrorFs{Fs: base, readErr: os.ErrPermission}
	result, err := scanFolderDetails(context.Background(), failed, "/target", func(string) bool { return true })
	if err != nil || result.Size != 3 || result.NumFiles != 1 || !result.Incomplete {
		t.Fatalf("read-error result=%+v err=%v", result, err)
	}
	failed.readErr = errors.New("unexpected internal failure")
	if _, err := scanFolderDetails(context.Background(), failed, "/target", func(string) bool { return true }); err == nil {
		t.Fatal("unexpected internal errors must not be hidden")
	}
}

func TestFolderDetailsScanBatchesWithoutPerFileStat(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := base.MkdirAll("/target/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		if err := afero.WriteFile(base, fmt.Sprintf("/target/nested/%d.txt", i), []byte("abc"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	counting := &folderDetailsCountingFs{Fs: base}
	result, err := scanFolderDetails(context.Background(), counting, "/target", func(string) bool { return true })
	if err != nil || result.Size != 1800 || result.NumFiles != 600 || result.Incomplete {
		t.Fatalf("batch result=%+v err=%v", result, err)
	}
	if counting.stats != 0 {
		t.Fatalf("scan made %d redundant Stats", counting.stats)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanFolderDetails(ctx, counting, "/target", func(string) bool { return true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan error=%v", err)
	}
	if _, err := scanFolderDetails(context.Background(), counting, "/missing", func(string) bool { return true }); err == nil {
		t.Fatal("missing folder must not return a successful zero total")
	}
}

func TestFolderDetailsScanSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "normal.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "normal.txt"), filepath.Join(root, "duplicate")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	result, err := scanFolderDetails(context.Background(), files.NewFs(afero.NewOsFs(), root, false), "/", func(string) bool { return true })
	if err != nil || result.Size != 3 || result.NumFiles != 1 {
		t.Fatalf("symlink scan result=%+v err=%v", result, err)
	}
}

func TestResourceSizeIncludesRecursiveFileCount(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"target/nested", "target/empty", "target/blocked"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		"target/top.txt": "abc", "target/nested/child.txt": "12345",
		"target/nested/zero.txt": "", "target/blocked/private.txt": "secret",
		"outside.txt": "not counted",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	key := []byte("size-test-signing-key")
	perm := users.Permissions{Download: true}
	st := scopedUserStorage(t, root, perm, key)
	config, err := st.Settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	config.Rules = []rules.Rule{{Path: "/target/blocked", Allow: false}}
	if err := st.Settings.Save(config); err != nil {
		t.Fatal(err)
	}
	handler := handle(resourceSizeHandler, "/api/resources/size", st, &settings.Server{})
	token := signToken(t, perm, key)
	request := func(path, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/resources/size"+path, nil)
		req.Header.Set("X-Auth", auth)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	check := func(path string, wantSize, wantFiles int64) {
		t.Helper()
		rec := request(path, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var result resourceSizeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Size != wantSize || result.NumFiles != wantFiles {
			t.Fatalf("got %+v, want size=%d files=%d", result, wantSize, wantFiles)
		}
	}
	check("/target", 8, 3)
	check("/target/empty", 0, 0)
	if err := os.WriteFile(filepath.Join(root, "target", "new.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	check("/target", 10, 4)
	if rec := request("/target", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated details status=%d", rec.Code)
	}
	for _, path := range []string{"/target/blocked", "/target/top.txt"} {
		if rec := request(path, token); rec.Code != http.StatusForbidden {
			t.Fatalf("forbidden path %s returned %d", path, rec.Code)
		}
	}
}
