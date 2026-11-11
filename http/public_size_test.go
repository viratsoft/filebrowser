package fbhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
)

func TestPublicFolderSizeOnlyCountsReadableShareFiles(t *testing.T) {
	root := t.TempDir()
	for _, folder := range []string{"shared/nested", "shared/blocked/deeper", "outside/deeper"} {
		if err := os.MkdirAll(filepath.Join(root, folder), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{
		"shared/visible":                     "abc",
		"shared/nested/child":                "12345",
		"shared/blocked/deeper/private":      "private data",
		"shared/.filebrowser-upload-pending": "unfinished upload",
		"outside/deeper/private":             "outside the share",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true}, []byte("share-size"))
	config, err := st.Settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	config.Rules = []rules.Rule{{Path: "/shared/blocked", Allow: false}}
	if err := st.Settings.Save(config); err != nil {
		t.Fatal(err)
	}
	for _, link := range []*share.Link{
		{Hash: "size", Path: "/shared", UserID: 1, PasswordHash: "protected", Token: "token"},
		{Hash: "upload", Path: "/shared", UserID: 1, AllowUpload: true, UploadOnly: true},
		{Hash: "expired", Path: "/shared", UserID: 1, Expire: 1},
	} {
		if err := st.Share.Save(link); err != nil {
			t.Fatal(err)
		}
	}
	handler := handle(publicFolderSizeHandler, "/api/public/size/", st, &settings.Server{FollowExternalSymlinks: true})
	request := func(suffix string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/size/"+suffix, nil))
		return rec
	}
	checkDetails := func(suffix string, size, numFiles int64) {
		t.Helper()
		rec := request(suffix)
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
		}
		var result map[string]int64
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || len(result) != 2 || result["size"] != size || result["numFiles"] != numFiles {
			t.Fatalf("expected size=%d and numFiles=%d, got %s", size, numFiles, rec.Body.String())
		}
	}
	checkDetails("size/?token=token", 8, 2)
	checkDetails("size/nested/?token=token", 5, 1)
	if err := os.WriteFile(filepath.Join(root, "shared", "extra"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkDetails("size/?token=token", 10, 3)

	for suffix, want := range map[string]int{
		"size/":                            http.StatusUnauthorized,
		"size/blocked/deeper/?token=token": http.StatusForbidden,
		"size/visible?token=token":         http.StatusForbidden,
		"upload/":                          http.StatusForbidden,
		"expired/":                         http.StatusNotFound,
	} {
		if rec := request(suffix); rec.Code != want {
			t.Errorf("%s returned %d, want %d", suffix, rec.Code, want)
		}
	}

	t.Run("symlink aliases", func(t *testing.T) {
		for alias, target := range map[string]string{"external": "outside", "internal": "shared/blocked"} {
			if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, "shared", alias)); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if rec := request("size/" + alias + "/deeper/?token=token"); rec.Code != http.StatusForbidden {
				t.Errorf("linked directory returned %d", rec.Code)
			}
		}
	})
	checkDetails("size/?token=token", 10, 3)
}
