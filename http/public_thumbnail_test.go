package fbhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/img"
	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
	"golang.org/x/crypto/bcrypt"
)

type publicThumbnailTestCache struct{ data map[string][]byte }

func (c *publicThumbnailTestCache) Load(_ context.Context, key string) ([]byte, bool, error) {
	data, ok := c.data[key]
	return data, ok, nil
}
func (c *publicThumbnailTestCache) Store(_ context.Context, key string, data []byte) error {
	c.data[key] = append([]byte(nil), data...)
	return nil
}
func (c *publicThumbnailTestCache) Delete(_ context.Context, key string) error {
	delete(c.data, key)
	return nil
}

func TestPublicThumbnailsHonorSharePrivacyAndPreferences(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "shared", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"shared/photo.png", "shared/nested/photo.png", "shared/denied.png", "shared/.filebrowser-upload-private.png", "secret.png"} {
		if err := os.WriteFile(filepath.Join(root, name), original.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "shared", "unsafe.svg"), []byte(`<svg onload="alert(1)"></svg>`), 0o600); err != nil {
		t.Fatal(err)
	}
	key := []byte("thumbnail-test-key")
	perm := users.Permissions{Share: true, Download: true, Create: true}
	st := scopedUserStorage(t, root, perm, key)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	link := &share.Link{Hash: "thumb-share", Path: "/shared", UserID: 1, Token: "thumbnail-token", PasswordHash: string(passwordHash)}
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	config, err := st.Settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	config.Rules = []rules.Rule{{Path: "/shared/denied.png", Allow: false}}
	if err := st.Settings.Save(config); err != nil {
		t.Fatal(err)
	}
	server := &settings.Server{EnableThumbnails: true}
	cache := &publicThumbnailTestCache{data: map[string][]byte{}}
	handler := handle(publicThumbnailHandler(img.New(1), cache), "/api/public/thumb/", st, server)
	request := func(path, token, password string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/public/thumb/thumb-share/"+path, nil)
		if token != "" {
			q := req.URL.Query()
			q.Set("token", token)
			req.URL.RawQuery = q.Encode()
		}
		if password != "" {
			req.Header.Set("X-SHARE-PASSWORD", password)
		}
		// Being logged in must not bypass share-password checks.
		req.Header.Set("X-Auth", signToken(t, perm, key))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	for _, token := range []string{"", "wrong"} {
		if rec := request("photo.png", token, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("password bypass: %d", rec.Code)
		}
	}
	checkImage := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("thumbnail status=%d body=%s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("unsafe headers: %v", rec.Header())
		}
		if bytes.Equal(rec.Body.Bytes(), original.Bytes()) {
			t.Fatal("thumbnail returned the raw original")
		}
		decoded, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
		if err != nil || decoded.Bounds().Dx() != 256 || decoded.Bounds().Dy() != 256 {
			t.Fatalf("invalid resized JPEG: %v", err)
		}
	}
	checkImage(request("photo.png", link.Token, ""))
	checkImage(request("nested/photo.png", "", "secret"))
	link.PasswordHash = ""
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	checkImage(request("photo.png", "", ""))
	link.PasswordHash = string(passwordHash)
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	listing := handle(publicShareHandler, "/api/public/share/", st, server)
	checkListing := func(want bool) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/public/share/thumb-share/?token="+link.Token, nil)
		rec := httptest.NewRecorder()
		listing.ServeHTTP(rec, req)
		var body struct {
			ShowThumbnails bool `json:"showThumbnails"`
		}
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.ShowThumbnails != want {
			t.Fatalf("listing flag status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	checkListing(true)
	if len(cache.data) == 0 {
		t.Fatal("thumbnail was not internally cached")
	}
	if err := os.Symlink(filepath.Join(root, "secret.png"), filepath.Join(root, "shared", "outside.png")); err == nil {
		if rec := request("outside.png", link.Token, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("symlink escaped share scope: %d", rec.Code)
		}
	}
	for path, status := range map[string]int{"": 404, "denied.png": 403, ".filebrowser-upload-private.png": 404, "../secret.png": 404, "unsafe.svg": 415} {
		if rec := request(path, link.Token, ""); rec.Code != status {
			t.Fatalf("path %q got %d want %d", path, rec.Code, status)
		}
	}
	if err := st.Users.Update(&users.User{ID: 1, HideShareThumbnails: true}, "HideShareThumbnails"); err != nil {
		t.Fatal(err)
	}
	if rec := request("photo.png", link.Token, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("disabled cached thumbnail: %d", rec.Code)
	}
	checkListing(false)
	if err := st.Users.Update(&users.User{ID: 1}, "HideShareThumbnails"); err != nil {
		t.Fatal(err)
	}
	link.UploadOnly, link.AllowUpload = true, true
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "photo.png"} {
		if rec := request(path, link.Token, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("upload-only preview %q: %d", path, rec.Code)
		}
	}
	checkListing(false)
	link.UploadOnly = false
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	server.EnableThumbnails = false
	if rec := request("photo.png", link.Token, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("server thumbnail disable ignored: %d", rec.Code)
	}
	server.EnableThumbnails = true
	if err := st.Users.Update(&users.User{ID: 1, Perm: users.Permissions{Share: true}}, "Perm"); err != nil {
		t.Fatal(err)
	}
	if rec := request("photo.png", link.Token, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("owner download permission ignored: %d", rec.Code)
	}
	if err := st.Users.Update(&users.User{ID: 1, Perm: perm}, "Perm"); err != nil {
		t.Fatal(err)
	}
	link.Expire = time.Now().Add(-time.Hour).Unix()
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	if rec := request("photo.png", link.Token, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("expired share preview: %d", rec.Code)
	}
}

func TestShareThumbnailPreferenceDefaultsAndPersistsWithoutChangingUser(t *testing.T) {
	key := []byte("preference-key")
	perm := users.Permissions{Share: true, Download: true}
	st := scopedUserStorage(t, t.TempDir(), perm, key)
	get := handle(sharePreferencesGetHandler, "", st, &settings.Server{})
	put := handle(sharePreferencesPutHandler, "", st, &settings.Server{})
	request := func(handler http.Handler, method, body string, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/shares/preferences", strings.NewReader(body))
		if authenticated {
			req.Header.Set("X-Auth", signToken(t, perm, key))
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	check := func(want bool) {
		t.Helper()
		rec := request(get, http.MethodGet, "", true)
		var result sharePreferencesResponse
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &result) != nil || result.ShowThumbnails != want {
			t.Fatalf("preference response: %d %s", rec.Code, rec.Body.String())
		}
	}
	check(true) // No new field in an existing database means default enabled.
	routed := handle(sharePreferencesHandler, "", st, &settings.Server{})
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if rec := request(routed, method, "{}", true); rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("unexpected preferences method %s: %d", method, rec.Code)
		}
	}
	for _, body := range []string{`{}`, `{"showThumbnails":null}`, `{"showThumbnails":true,"userID":2}`, `{"showThumbnails":true} {}`} {
		if rec := request(put, http.MethodPut, body, true); rec.Code != 400 {
			t.Fatalf("invalid preference accepted: %s (%d)", body, rec.Code)
		}
	}
	if rec := request(put, http.MethodPut, `{"showThumbnails":false}`, false); rec.Code != 401 {
		t.Fatalf("anonymous preference update: %d", rec.Code)
	}
	before, err := st.Users.Get("", false, uint(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		body, err := json.Marshal(sharePreferencesResponse{ShowThumbnails: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if rec := request(put, http.MethodPut, string(body), true); rec.Code != 200 {
			t.Fatalf("save preference: %d %s", rec.Code, rec.Body.String())
		}
		check(enabled)
		after, err := st.Users.Get("", false, uint(1))
		if err != nil {
			t.Fatal(err)
		}
		if after.Username != before.Username || after.Password != before.Password || after.Perm != before.Perm || after.Scope != before.Scope {
			t.Fatal("preference update changed other user fields")
		}
	}
}
