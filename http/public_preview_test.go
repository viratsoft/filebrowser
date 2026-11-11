package fbhttp

import (
	"bytes"
	"image"
	"image/jpeg"

	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/filebrowser/filebrowser/v2/img"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
	"golang.org/x/crypto/bcrypt"
)

func TestPublicViewerPreviewAndOriginalRespectSharePolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "shared"), 0o700); err != nil {
		t.Fatal(err)
	}
	var original bytes.Buffer
	if err := jpeg.Encode(&original, image.NewGray(image.Rect(0, 0, 9000, 6000)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shared", "photo.JPG"), original.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	key := []byte("public-preview-key")
	perm := users.Permissions{Share: true, Download: true}
	st := scopedUserStorage(t, root, perm, key)
	password, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	link := &share.Link{Hash: "viewer", Path: "/shared", UserID: 1, Token: "token", PasswordHash: string(password)}
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	server := &settings.Server{EnableThumbnails: true, ResizePreview: true}
	cache := &publicThumbnailTestCache{data: map[string][]byte{}}
	preview := handle(publicPreviewHandler(img.New(1), cache), "/api/public/preview/", st, server)
	thumb := handle(publicThumbnailHandler(img.New(1), cache), "/api/public/thumb/", st, server)
	raw := handle(publicDlHandler, "/api/public/dl/", st, server)
	request := func(handler http.Handler, endpoint, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/public/"+endpoint+"/viewer/photo.JPG?inline=true&token="+token, nil)
		req.Header.Set("X-Auth", signToken(t, perm, key))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(preview, "preview", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("login bypassed password: %d", rec.Code)
	}
	result := request(preview, "preview", link.Token)
	decoded, err := jpeg.Decode(bytes.NewReader(result.Body.Bytes()))
	if result.Code != http.StatusOK || err != nil {
		t.Fatalf("viewer preview: %d %v", result.Code, err)
	}
	if decoded.Bounds().Dx() != 1080 || decoded.Bounds().Dy() != 720 {
		t.Fatalf("preview was cropped like a list thumbnail: %v", decoded.Bounds())
	}
	if rec := request(thumb, "thumb", link.Token); rec.Code != http.StatusOK {
		t.Fatalf("list thumb: %d", rec.Code)
	}
	if len(cache.data) != 2 {
		t.Fatal("viewer preview and square list thumbnail share a cache entry")
	}
	if err := st.Users.Update(&users.User{ID: 1, HideShareThumbnails: true}, "HideShareThumbnails"); err != nil {
		t.Fatal(err)
	}
	if rec := request(thumb, "thumb", link.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("hidden list thumbnail accessible: %d", rec.Code)
	}
	if rec := request(preview, "preview", link.Token); rec.Code != http.StatusOK {
		t.Fatalf("appearance preference incorrectly removed existing image read access: %d", rec.Code)
	}
	if rec := request(raw, "dl", link.Token); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), original.Bytes()) {
		t.Fatalf("original changed or unreadable: %d", rec.Code)
	}
	server.ResizePreview = false
	if rec := request(preview, "preview", link.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("disabled preview fell back to raw data: %d", rec.Code)
	}
	server.ResizePreview = true
	link.UploadOnly, link.AllowUpload = true, true
	if err := st.Share.Save(link); err != nil {
		t.Fatal(err)
	}
	if rec := request(preview, "preview", link.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("upload-only preview leak: %d", rec.Code)
	}
	if rec := request(raw, "dl", link.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("upload-only original leak: %d", rec.Code)
	}
}
