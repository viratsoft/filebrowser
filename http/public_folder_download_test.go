package fbhttp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
)

func TestPublicFolderDownloadUsesCurrentTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "shared/nested/deep"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"shared/main.jpg", "shared/nested/child.jpg", "shared/nested/deep/item.jpg", "shared/nested/.filebrowser-upload-pending", "private.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true}, []byte("folder-download"))
	if err := st.Share.Save(&share.Link{Hash: "tree", Path: "/shared", UserID: 1}); err != nil {
		t.Fatal(err)
	}
	handler := handle(publicFolderDownloadHandler, "/api/public/folder-download/", st, &settings.Server{})
	request := func(suffix string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/folder-download/tree/"+suffix, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request returned %d: %s", rec.Code, rec.Body.String())
		}
		return rec
	}
	for _, tc := range []struct {
		folder string
		prefix string
		names  []string
	}{
		{"", "/", []string{"main.jpg", "nested/child.jpg", "nested/deep/item.jpg"}},
		{"nested/", "/nested/", []string{"child.jpg", "deep/item.jpg"}},
	} {
		var plan publicFolderDownloadPlan
		if err := json.Unmarshal(request(tc.folder+"?manifest=1").Body.Bytes(), &plan); err != nil {
			t.Fatal(err)
		}
		rec := request(tc.folder)
		archive, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		var zipped, expectedPaths []string
		for _, entry := range archive.File {
			if !entry.FileInfo().IsDir() {
				zipped = append(zipped, entry.Name)
			}
		}
		for _, name := range tc.names {
			expectedPaths = append(expectedPaths, tc.prefix+name)
		}
		slices.Sort(zipped)
		slices.Sort(plan.Paths)
		if !slices.Equal(zipped, tc.names) || !slices.Equal(plan.Paths, expectedPaths) {
			t.Fatalf("scope %q: archive=%v, manifest=%v", tc.folder, zipped, plan.Paths)
		}
	}
}
