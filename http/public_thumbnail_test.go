package fbhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
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
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

func testPublicThumbnailHandler(cache FileCache) handleFunc {
	service := img.New(1)
	return publicThumbnailHandler(service, newImageRenderer(service, cache, PreviewSizeThumb))
}

func TestGenericUserSavePreservesPrivateShareThumbnailPreference(t *testing.T) {
	key := []byte("preserve-thumbnail-preference")
	perm := users.Permissions{Admin: true, Share: true, Download: true}
	st := scopedUserStorage(t, t.TempDir(), perm, key)
	stale, err := st.Users.Get("", false, uint(1))
	if err != nil {
		t.Fatal(err)
	}
	stale.Password = ""
	if err := st.Users.Update(&users.User{ID: 1, HideShareThumbnails: true}, "HideShareThumbnails"); err != nil {
		t.Fatal(err)
	}
	requestBody, err := json.Marshal(modifyUserRequest{
		modifyRequest: modifyRequest{What: "user", Which: []string{"all"}}, Data: stale,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/users/1", bytes.NewReader(requestBody))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req.Header.Set("X-Auth", signToken(t, perm, key))
	rec := httptest.NewRecorder()
	handle(userPutHandler, "", st, &settings.Server{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("user save: %d %s", rec.Code, rec.Body.String())
	}
	stored, err := st.Users.Get("", false, uint(1))
	if err != nil {
		t.Fatal(err)
	}
	if !stored.HideShareThumbnails {
		t.Fatal("generic user save re-enabled public thumbnails")
	}
}

func TestLoginThumbnailWarmsFirstShareRequest(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "shared"), 0o700); err != nil {
		t.Fatal(err)
	}
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shared", "photo.png"), source.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	key := []byte("login-warmer")
	perm := users.Permissions{Share: true, Download: true}
	st := scopedUserStorage(t, root, perm, key)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Share.Save(&share.Link{
		Hash: "warm", Path: "/shared", UserID: 1,
		PasswordHash: string(passwordHash), Token: "share-token",
	}); err != nil {
		t.Fatal(err)
	}
	server := &settings.Server{EnableThumbnails: true}
	service := img.New(1)
	cache := &publicThumbnailTestCache{data: map[string][]byte{}}
	render := newImageRenderer(service, cache, PreviewSizeThumb)
	login := handle(previewHandler(service, cache, render, true, true), "/api/preview", st, server)
	public := handle(publicThumbnailHandler(service, render), "/api/public/thumb/", st, server)

	req := httptest.NewRequest(http.MethodGet, "/api/preview/thumb/shared/photo.png?inline=true", nil)
	req = mux.SetURLVars(req, map[string]string{"size": "thumb", "path": "shared/photo.png"})
	req.Header.Set("X-Auth", signToken(t, perm, key))
	first := httptest.NewRecorder()
	login.ServeHTTP(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("authenticated thumbnail: %d %s", first.Code, first.Body.String())
	}

	// Force the public request to use the shared RAM cache or regenerate.
	clear(cache.data)
	unauthorized := httptest.NewRecorder()
	public.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/public/thumb/warm/photo.png", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("cached thumbnail bypassed share password: %d", unauthorized.Code)
	}

	guest := httptest.NewRecorder()
	public.ServeHTTP(guest, httptest.NewRequest(http.MethodGet, "/api/public/thumb/warm/photo.png?token=share-token", nil))
	if guest.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), guest.Body.Bytes()) || len(cache.data) != 0 {
		t.Fatalf("first public view failed to reuse login thumbnail: status=%d disk writes=%d", guest.Code, len(cache.data))
	}

	if err := st.Users.Update(&users.User{ID: 1, HideShareThumbnails: true}, "HideShareThumbnails"); err != nil {
		t.Fatal(err)
	}
	blocked := httptest.NewRecorder()
	public.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/api/public/thumb/warm/photo.png?token=share-token", nil))
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("cached thumbnail bypassed share setting: %d", blocked.Code)
	}
}

func TestPublicThumbnailCacheDoesNotReuseReplacedImageWithSameMetadata(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "photo.png")
	writePhoto := func(c color.RGBA) {
		t.Helper()
		photo := image.NewRGBA(image.Rect(0, 0, 4, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				photo.SetRGBA(x, y, c)
			}
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, photo); err != nil {
			t.Fatal(err)
		}
		// PNG permits trailing data. Keep the byte length identical for both versions.
		padded := make([]byte, 4096)
		copy(padded, encoded.Bytes())
		if err := os.WriteFile(filename, padded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writePhoto(color.RGBA{R: 255, A: 255})
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(filename, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	key := []byte("same-metadata-cache")
	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true}, key)
	if err := st.Share.Save(&share.Link{Hash: "photo-share", Path: "/", UserID: 1}); err != nil {
		t.Fatal(err)
	}
	cache := &publicThumbnailTestCache{data: map[string][]byte{}}
	handler := handle(testPublicThumbnailHandler(cache), "/api/public/thumb/", st, &settings.Server{EnableThumbnails: true})
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/public/thumb/photo-share/photo.png", nil)
		req.Header.Set("If-Modified-Since", stamp.Format(http.TimeFormat))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("thumbnail: %d %s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Last-Modified") != "" {
			t.Fatal("mtime-only HTTP validator permits stale pixels")
		}
		return rec
	}
	before := request().Body.Bytes()
	writePhoto(color.RGBA{B: 255, A: 255})
	if err := os.Chtimes(filename, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	after := request().Body.Bytes()
	if bytes.Equal(before, after) {
		t.Fatal("replacement image reused the previous thumbnail")
	}
	if len(cache.data) != 2 {
		t.Fatalf("expected two content versions, got %d", len(cache.data))
	}
	clear(cache.data)
	if !bytes.Equal(after, request().Body.Bytes()) || len(cache.data) != 0 {
		t.Fatal("unchanged content did not reuse its in-memory thumbnail")
	}
}

func TestPublicThumbnailFingerprintUsesSnapshotOnlyAtEOF(t *testing.T) {
	original := []byte("source image bytes")
	expected := sha256.Sum256(original)
	for _, tc := range []struct {
		name     string
		size     int64
		buffered bool
	}{
		{name: "unchanged", size: int64(len(original)), buffered: true},
		{name: "shrunk", size: int64(len(original)) + 1, buffered: true},
		{name: "grew", size: int64(len(original)) - 1},
		{name: "streamed", size: maxPublicThumbnailSnapshotBytes + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fingerprint, snapshot, err := fingerprintPublicThumbnail(context.Background(), bytes.NewReader(original), tc.size)
			if err != nil || !bytes.Equal(fingerprint, expected[:]) {
				t.Fatalf("incorrect content fingerprint: %x, %v", fingerprint, err)
			}
			if tc.buffered && !bytes.Equal(snapshot, original) || !tc.buffered && snapshot != nil {
				t.Fatalf("incorrect snapshot: %q", snapshot)
			}
		})
	}
}

func TestPublicThumbnailSourceReadIsBoundedAndCancelable(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "oversized.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(maxPublicThumbnailSourceBytes + 1)
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true}, []byte("bounded-thumbnail"))
	if err := st.Share.Save(&share.Link{Hash: "large-share", Path: "/", UserID: 1}); err != nil {
		t.Fatal(err)
	}
	cache := &publicThumbnailTestCache{data: map[string][]byte{}}
	rec := httptest.NewRecorder()
	handle(testPublicThumbnailHandler(cache), "/api/public/thumb/", st, &settings.Server{EnableThumbnails: true}).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/public/thumb/large-share/oversized.png", nil))
	if rec.Code != http.StatusRequestEntityTooLarge || len(cache.data) != 0 {
		t.Fatalf("oversized image was read/cached: %d", rec.Code)
	}
	// A valid PNG header can advertise enormous dimensions in just 33 bytes.
	// Reject it before a full decoder attempts to allocate its pixel buffer.
	header := make([]byte, 33)
	copy(header, "\x89PNG\r\n\x1a\n")
	binary.BigEndian.PutUint32(header[8:12], 13)
	copy(header[12:16], "IHDR")
	binary.BigEndian.PutUint32(header[16:20], 11000)
	binary.BigEndian.PutUint32(header[20:24], 11000)
	header[24], header[25] = 8, 2
	binary.BigEndian.PutUint32(header[29:33], crc32.ChecksumIEEE(header[12:29]))
	if err := os.WriteFile(filepath.Join(root, "huge-pixels.png"), header, 0o600); err != nil {
		t.Fatal(err)
	}
	large := httptest.NewRecorder()
	handle(testPublicThumbnailHandler(cache), "/api/public/thumb/", st, &settings.Server{EnableThumbnails: true}).ServeHTTP(large,
		httptest.NewRequest(http.MethodGet, "/api/public/thumb/large-share/huge-pixels.png", nil))
	if large.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("pixel allocation not bounded: %d", large.Code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := publicThumbnailSourceReader{ctx: ctx, reader: strings.NewReader("private image")}
	if n, err := r.Read(make([]byte, 32)); n != 0 || err != context.Canceled {
		t.Fatalf("canceled source read returned %d, %v", n, err)
	}
}

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
	for _, name := range []string{"unsafe.svg", "spoofed.png"} {
		if err := os.WriteFile(filepath.Join(root, "shared", name), []byte(`<svg onload="alert(1)"></svg>`), 0o600); err != nil {
			t.Fatal(err)
		}
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
	handler := handle(testPublicThumbnailHandler(cache), "/api/public/thumb/", st, server)
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
	oldToken := link.Token
	link.Token = "rotated-thumbnail-token"
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	if rec := request("photo.png", oldToken, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old token reused a cached thumbnail: %d", rec.Code)
	}
	checkImage(request("photo.png", link.Token, ""))
	if err := os.Symlink(filepath.Join(root, "secret.png"), filepath.Join(root, "shared", "outside.png")); err == nil {
		if rec := request("outside.png", link.Token, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("symlink escaped share scope: %d", rec.Code)
		}
	}
	for path, status := range map[string]int{"": 404, "denied.png": 403, ".filebrowser-upload-private.png": 404, "../secret.png": 404, "unsafe.svg": 415, "spoofed.png": 415} {
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
	if err := st.Users.Save(&users.User{ID: 2, Username: "other", Password: "pw", Perm: perm, HideShareThumbnails: true}); err != nil {
		t.Fatal(err)
	}
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
		other, err := st.Users.Get("", false, uint(2))
		if err != nil {
			t.Fatal(err)
		}
		if !other.HideShareThumbnails {
			t.Fatal("preference update changed another owner")
		}
		after, err := st.Users.Get("", false, uint(1))
		if err != nil {
			t.Fatal(err)
		}
		if after.Username != before.Username || after.Password != before.Password || after.Perm != before.Perm || after.Scope != before.Scope {
			t.Fatal("preference update changed other user fields")
		}
	}
}
