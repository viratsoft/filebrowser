package fbhttp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asdine/storm/v3"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/storage/bolt"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
	"golang.org/x/crypto/bcrypt"
)

func TestPublicShareHandlerAuthentication(t *testing.T) {
	t.Parallel()

	const passwordBcrypt = "$2y$10$TFAmdCbyd/mEZDe5fUeZJu.MaJQXRTwdqb/IQV.eTn6dWrF58gCSe"
	testCases := map[string]struct {
		share              *share.Link
		req                *http.Request
		sharePerm          bool
		downloadPerm       bool
		expectedStatusCode int
	}{
		"Public share, no auth required": {
			share:              &share.Link{Hash: "h", UserID: 1},
			req:                newHTTPRequest(t),
			sharePerm:          true,
			downloadPerm:       true,
			expectedStatusCode: 200,
		},
		"Private share, no auth provided, 401": {
			share:              &share.Link{Hash: "h", UserID: 1, PasswordHash: passwordBcrypt, Token: "123"},
			req:                newHTTPRequest(t),
			sharePerm:          true,
			downloadPerm:       true,
			expectedStatusCode: 401,
		},
		"Private share, authentication via token": {
			share:              &share.Link{Hash: "h", UserID: 1, PasswordHash: passwordBcrypt, Token: "123"},
			req:                newHTTPRequest(t, func(r *http.Request) { r.URL.RawQuery = "token=123" }),
			sharePerm:          true,
			downloadPerm:       true,
			expectedStatusCode: 200,
		},
		"Private share, authentication via invalid token, 401": {
			share:              &share.Link{Hash: "h", UserID: 1, PasswordHash: passwordBcrypt, Token: "123"},
			req:                newHTTPRequest(t, func(r *http.Request) { r.URL.RawQuery = "token=1234" }),
			sharePerm:          true,
			downloadPerm:       true,
			expectedStatusCode: 401,
		},
		"Private share, authentication via password": {
			share:              &share.Link{Hash: "h", UserID: 1, PasswordHash: passwordBcrypt, Token: "123"},
			req:                newHTTPRequest(t, func(r *http.Request) { r.Header.Set("X-SHARE-PASSWORD", "password") }),
			sharePerm:          true,
			downloadPerm:       true,
			expectedStatusCode: 200,
		},
		"Private share, authentication via invalid password, 401": {
			share:              &share.Link{Hash: "h", UserID: 1, PasswordHash: passwordBcrypt, Token: "123"},
			req:                newHTTPRequest(t, func(r *http.Request) { r.Header.Set("X-SHARE-PASSWORD", "wrong-password") }),
			sharePerm:          true,
			downloadPerm:       true,
			expectedStatusCode: 401,
		},
		"Share owner lost share permission, 403": {
			share:              &share.Link{Hash: "h", UserID: 1},
			req:                newHTTPRequest(t),
			sharePerm:          false,
			downloadPerm:       true,
			expectedStatusCode: 403,
		},
		"Share owner lost download permission, 403": {
			share:              &share.Link{Hash: "h", UserID: 1},
			req:                newHTTPRequest(t),
			sharePerm:          true,
			downloadPerm:       false,
			expectedStatusCode: 403,
		},
	}

	for name, tc := range testCases {
		for handlerName, handler := range map[string]handleFunc{"public share handler": publicShareHandler, "public dl handler": publicDlHandler} {
			name, tc, handlerName, handler := name, tc, handlerName, handler
			t.Run(fmt.Sprintf("%s: %s", handlerName, name), func(t *testing.T) {
				t.Parallel()

				dbPath := filepath.Join(t.TempDir(), "db")
				db, err := storm.Open(dbPath)
				if err != nil {
					t.Fatalf("failed to open db: %v", err)
				}

				t.Cleanup(func() {
					if err := db.Close(); err != nil {
						t.Errorf("failed to close db: %v", err)
					}
				})

				storage, err := bolt.NewStorage(db)
				if err != nil {
					t.Fatalf("failed to get storage: %v", err)
				}
				if err := storage.Share.Save(tc.share); err != nil {
					t.Fatalf("failed to save share: %v", err)
				}
				if err := storage.Users.Save(&users.User{
					Username: "username",
					Password: "pw",
					Perm: users.Permissions{
						Share:    tc.sharePerm,
						Download: tc.downloadPerm,
					},
				}); err != nil {
					t.Fatalf("failed to save user: %v", err)
				}
				if err := storage.Settings.Save(&settings.Settings{Key: []byte("key")}); err != nil {
					t.Fatalf("failed to save settings: %v", err)
				}

				storage.Users = &customFSUser{
					Store: storage.Users,
					fs:    &afero.MemMapFs{},
				}

				recorder := httptest.NewRecorder()
				handler := handle(handler, "", storage, &settings.Server{})

				handler.ServeHTTP(recorder, tc.req)
				result := recorder.Result()
				defer result.Body.Close()
				if result.StatusCode != tc.expectedStatusCode {
					t.Errorf("expected status code %d, got status code %d", tc.expectedStatusCode, result.StatusCode)
				}
			})
		}
	}
}

func TestPublicUploadHandler(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}

	key := []byte("test-signing-key")
	perm := users.Permissions{Share: true, Download: true, Create: true}
	st := scopedUserStorage(t, root, perm, key)
	if err := st.Share.Save(&share.Link{Hash: "upload", UserID: 1, Path: "/shared", AllowUpload: true}); err != nil {
		t.Fatal(err)
	}

	handler := handle(publicUploadHandler, "/api/public/upload/", st, &settings.Server{})
	request := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := request("/api/public/upload/upload/new.txt", "guest upload"); rec.Code != http.StatusOK {
		t.Fatalf("expected upload success, got %d: %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(shared, "new.txt"))
	if err != nil || string(data) != "guest upload" {
		t.Fatalf("uploaded file mismatch: data=%q err=%v", data, err)
	}
	if rec := request("/api/public/upload/upload/new.txt", "overwrite"); rec.Code != http.StatusConflict {
		t.Fatalf("expected no-overwrite conflict, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request("/api/public/upload/upload/nested/file.txt", "escape"); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected nested path rejection, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := st.Share.Save(&share.Link{Hash: "readonly", UserID: 1, Path: "/shared"}); err != nil {
		t.Fatal(err)
	}
	if rec := request("/api/public/upload/readonly/blocked.txt", "blocked"); rec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled-upload rejection, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPublicTusUploadRequiresShareAuthorizationForEveryRequest(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true, Create: true}, []byte("test-signing-key"))
	if err := st.Share.Save(&share.Link{Hash: "resumable", UserID: 1, Path: "/shared", AllowUpload: true, UploadOnly: true, SessionUploadFolder: true, PasswordHash: string(passwordHash)}); err != nil {
		t.Fatal(err)
	}
	cache := newMemoryUploadCache()
	post := handle(publicTusPostHandler(cache), "/api/public/tus/", st, &settings.Server{})
	patch := handle(publicTusPatchHandler(cache), "/api/public/tus/", st, &settings.Server{})
	shareHandler := handle(publicShareHandler, "/api/public/share/", st, &settings.Server{})
	downloadHandler := handle(publicDlHandler, "/api/public/dl/", st, &settings.Server{})

	postReq := httptest.NewRequest(http.MethodPost, "/api/public/tus/resumable/report.txt", nil)
	postReq.Header.Set("Upload-Length", "6")
	postReq.Header.Set("X-SHARE-PASSWORD", "correct%20password")
	postRec := httptest.NewRecorder()
	post.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("expected TUS create, got %d: %s", postRec.Code, postRec.Body.String())
	}
	cookies := postRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected visitor cookie")
	}

	unauthorized := httptest.NewRequest(http.MethodPatch, "/api/public/tus/resumable/report.txt", strings.NewReader("secret"))
	unauthorized.Header.Set("Content-Type", "application/offset+octet-stream")
	unauthorized.Header.Set("Upload-Offset", "0")
	unauthorizedRec := httptest.NewRecorder()
	patch.ServeHTTP(unauthorizedRec, unauthorized)
	if unauthorizedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected password on every PATCH, got %d", unauthorizedRec.Code)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/public/tus/resumable/report.txt", strings.NewReader("secret"))
	patchReq.Header.Set("Content-Type", "application/offset+octet-stream")
	patchReq.Header.Set("Upload-Offset", "0")
	patchReq.Header.Set("X-SHARE-PASSWORD", "correct%20password")
	for _, cookie := range cookies {
		patchReq.AddCookie(cookie)
	}
	patchRec := httptest.NewRecorder()
	patch.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusNoContent {
		t.Fatalf("expected TUS patch, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	entries, err := os.ReadDir(shared)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("expected exactly one visitor folder, entries=%v err=%v", entries, err)
	}
	if data, err := os.ReadFile(filepath.Join(shared, entries[0].Name(), "report.txt")); err != nil || string(data) != "secret" {
		t.Fatalf("completed data mismatch: %q, %v", data, err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/public/share/resumable/", nil)
	listReq.Header.Set("X-SHARE-PASSWORD", "correct%20password")
	for _, cookie := range cookies {
		listReq.AddCookie(cookie)
	}
	listRec := httptest.NewRecorder()
	shareHandler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "report.txt") {
		t.Fatalf("expected completed visitor file only, got %d: %s", listRec.Code, listRec.Body.String())
	}
	dlReq := httptest.NewRequest(http.MethodGet, "/api/public/dl/resumable/report.txt", nil)
	dlReq.Header.Set("X-SHARE-PASSWORD", "correct%20password")
	for _, cookie := range cookies {
		dlReq.AddCookie(cookie)
	}
	dlRec := httptest.NewRecorder()
	downloadHandler.ServeHTTP(dlRec, dlReq)
	if dlRec.Code != http.StatusForbidden {
		t.Fatalf("upload-only file must remain unreadable, got %d", dlRec.Code)
	}
}

func TestUploadOnlyFileShareIsRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true, Create: true}, []byte("test-signing-key"))
	if err := st.Share.Save(&share.Link{Hash: "invalid-upload-only", UserID: 1, Path: "/file.txt", AllowUpload: true, UploadOnly: true}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handle(publicShareHandler, "/api/public/share/", st, &settings.Server{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/share/invalid-upload-only", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected malformed upload-only file share to be forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadOnlyShareLimitsVisitorsToTheirOwnUploads(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "existing.txt"), []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true, Create: true}, []byte("test-signing-key"))
	if err := st.Share.Save(&share.Link{Hash: "upload-only", UserID: 1, Path: "/shared", AllowUpload: true, UploadOnly: true}); err != nil {
		t.Fatal(err)
	}

	shareHandler := handle(publicShareHandler, "/api/public/share/", st, &settings.Server{})
	uploadHandler := handle(publicUploadHandler, "/api/public/upload/", st, &settings.Server{})
	downloadHandler := handle(publicDlHandler, "/api/public/dl/", st, &settings.Server{})
	request := func(handler http.Handler, method, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	listing := request(shareHandler, http.MethodGet, "/api/public/share/upload-only/", "", nil)
	if listing.Code != http.StatusOK || strings.Contains(listing.Body.String(), "existing.txt") {
		t.Fatalf("existing files must be hidden, got %d: %s", listing.Code, listing.Body.String())
	}
	if listing.Header().Get("Cache-Control") != "private, no-store" || listing.Header().Get("Vary") != "Cookie" {
		t.Fatalf("upload-only listing must not be shared through caches: headers=%v", listing.Header())
	}
	cookies := listing.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected an upload-only visitor session cookie")
	}

	if rec := request(downloadHandler, http.MethodGet, "/api/public/dl/upload-only/existing.txt", "", cookies); rec.Code != http.StatusForbidden {
		t.Fatalf("expected existing file download to be forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(downloadHandler, http.MethodGet, "/api/public/dl/upload-only/", "", cookies); rec.Code != http.StatusForbidden {
		t.Fatalf("expected archive download to be forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(uploadHandler, http.MethodPost, "/api/public/upload/upload-only/new.txt", "guest upload", cookies); rec.Code != http.StatusOK {
		t.Fatalf("expected upload success, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(shareHandler, http.MethodGet, "/api/public/share/upload-only/new.txt", "", cookies); rec.Code != http.StatusForbidden {
		t.Fatalf("expected uploaded file metadata to be forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(downloadHandler, http.MethodGet, "/api/public/dl/upload-only/new.txt", "", cookies); rec.Code != http.StatusForbidden {
		t.Fatalf("expected uploaded file download to be forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
	listing = request(shareHandler, http.MethodGet, "/api/public/share/upload-only/", "", cookies)
	if listing.Code != http.StatusOK || !strings.Contains(listing.Body.String(), "new.txt") || strings.Contains(listing.Body.String(), "existing.txt") {
		t.Fatalf("expected only the visitor's uploaded file in the listing, got %d: %s", listing.Code, listing.Body.String())
	}
}

func TestWriteNewPublicUploadNeverOverwrites(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/existing.txt", []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	created, err := writeNewPublicUpload(fs, "/existing.txt", strings.NewReader("replacement"), 0o644)
	if created || !os.IsExist(err) {
		t.Fatalf("expected exclusive create to reject an existing file, created=%v err=%v", created, err)
	}
	contents, err := afero.ReadFile(fs, "/existing.txt")
	if err != nil || string(contents) != "original" {
		t.Fatalf("existing file was changed: contents=%q err=%v", contents, err)
	}
}

func TestUploadOnlySessionFolderPersistsAndSeparatesVisitors(t *testing.T) {
	publicUploadSessions.Lock()
	originalSessions := publicUploadSessions.sessions
	publicUploadSessions.sessions = map[string]*uploadSessionFiles{}
	publicUploadSessions.Unlock()
	t.Cleanup(func() {
		publicUploadSessions.Lock()
		publicUploadSessions.sessions = originalSessions
		publicUploadSessions.Unlock()
	})

	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "existing.txt"), []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := scopedUserStorage(t, root, users.Permissions{Share: true, Download: true, Create: true}, []byte("test-signing-key"))
	if err := st.Share.Save(&share.Link{Hash: "session-folder", UserID: 1, Path: "/shared", AllowUpload: true, UploadOnly: true, SessionUploadFolder: true}); err != nil {
		t.Fatal(err)
	}

	shareHandler := handle(publicShareHandler, "/api/public/share/", st, &settings.Server{})
	uploadHandler := handle(publicUploadHandler, "/api/public/upload/", st, &settings.Server{})
	request := func(handler http.Handler, method, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	firstListing := request(shareHandler, http.MethodGet, "/api/public/share/session-folder/", "", nil)
	if firstListing.Code != http.StatusOK || strings.Contains(firstListing.Body.String(), "existing.txt") {
		t.Fatalf("expected an empty private visitor listing, got %d: %s", firstListing.Code, firstListing.Body.String())
	}
	if !strings.Contains(firstListing.Body.String(), `"items":[]`) {
		t.Fatalf("expected an empty array for a new visitor folder, got %s", firstListing.Body.String())
	}
	firstCookies := firstListing.Result().Cookies()
	if len(firstCookies) == 0 {
		t.Fatal("expected a signed visitor-session cookie")
	}
	if rec := request(uploadHandler, http.MethodPost, "/api/public/upload/session-folder/one.txt", "one", firstCookies); rec.Code != http.StatusOK {
		t.Fatalf("expected first upload to succeed, got %d: %s", rec.Code, rec.Body.String())
	}

	entries, err := os.ReadDir(shared)
	if err != nil || len(entries) != 2 || !entries[1].IsDir() {
		t.Fatalf("expected one isolated visitor folder, entries=%v err=%v", entries, err)
	}
	visitorFolder := entries[1].Name()
	if !strings.HasPrefix(visitorFolder, "upload_") {
		t.Fatalf("unexpected server-generated visitor folder name %q", visitorFolder)
	}
	if contents, err := os.ReadFile(filepath.Join(shared, visitorFolder, "one.txt")); err != nil || string(contents) != "one" {
		t.Fatalf("first upload is not in its visitor folder: contents=%q err=%v", contents, err)
	}

	// A restart clears the in-memory display cache, but the signed cookie keeps
	// the same server-generated folder for the full session lifetime.
	publicUploadSessions.Lock()
	publicUploadSessions.sessions = map[string]*uploadSessionFiles{}
	publicUploadSessions.Unlock()
	if rec := request(shareHandler, http.MethodGet, "/api/public/share/session-folder/", "", firstCookies); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "one.txt") {
		t.Fatalf("expected the first visitor's folder after restart, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(uploadHandler, http.MethodPost, "/api/public/upload/session-folder/two.txt", "two", firstCookies); rec.Code != http.StatusOK {
		t.Fatalf("expected returning visitor upload to succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(shared, visitorFolder, "two.txt")); err != nil {
		t.Fatalf("returning visitor did not reuse its folder: %v", err)
	}

	secondListing := request(shareHandler, http.MethodGet, "/api/public/share/session-folder/", "", nil)
	secondCookies := secondListing.Result().Cookies()
	if secondListing.Code != http.StatusOK || strings.Contains(secondListing.Body.String(), "one.txt") || len(secondCookies) == 0 {
		t.Fatalf("expected a separate empty visitor listing, got %d: %s", secondListing.Code, secondListing.Body.String())
	}
	if rec := request(uploadHandler, http.MethodPost, "/api/public/upload/session-folder/other.txt", "other", secondCookies); rec.Code != http.StatusOK {
		t.Fatalf("expected second visitor upload to succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	entries, err = os.ReadDir(shared)
	if err != nil || len(entries) != 3 {
		t.Fatalf("expected two isolated visitor folders, entries=%v err=%v", entries, err)
	}
}

func TestPasswordProtectedEmptySessionFolderReturnsEmptyItemsArray(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}

	perm := users.Permissions{Share: true, Download: true, Create: true}
	st := scopedUserStorage(t, root, perm, []byte("test-signing-key"))
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Share.Save(&share.Link{
		Hash:                "password-session-folder",
		UserID:              1,
		Path:                "/shared",
		AllowUpload:         true,
		UploadOnly:          true,
		SessionUploadFolder: true,
		PasswordHash:        string(passwordHash),
	}); err != nil {
		t.Fatal(err)
	}

	shareHandler := handle(publicShareHandler, "/api/public/share/", st, &settings.Server{})
	listing := httptest.NewRequest(http.MethodGet, "/api/public/share/password-session-folder/", nil)
	listing.Header.Set("X-SHARE-PASSWORD", "correct%20password")
	listingRec := httptest.NewRecorder()
	shareHandler.ServeHTTP(listingRec, listing)
	if listingRec.Code != http.StatusOK {
		t.Fatalf("expected password-protected session folder listing to succeed, got %d: %s", listingRec.Code, listingRec.Body.String())
	}
	if !strings.Contains(listingRec.Body.String(), `"items":[]`) {
		t.Fatalf("expected an empty items array, got %s", listingRec.Body.String())
	}
}

// TestPublicShareHandlerRules ensures that owner rules keep applying to paths
// below a shared directory, even though the share rebases the filesystem onto
// that directory. A deny rule relative to the owner's scope must not be
// bypassable by requesting the blocked path through the public share.
func TestPublicShareHandlerRules(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		handler            handleFunc
		path               string
		expectedStatusCode int
	}{
		"blocked file via dl handler, 403": {
			handler:            publicDlHandler,
			path:               "h/private/secret.txt",
			expectedStatusCode: 403,
		},
		"blocked dir listing via share handler, 403": {
			handler:            publicShareHandler,
			path:               "h/private/",
			expectedStatusCode: 403,
		},
		"blocked dir download via dl handler, 403": {
			handler:            publicDlHandler,
			path:               "h/private/",
			expectedStatusCode: 403,
		},
		"allowed file via dl handler, 200": {
			handler:            publicDlHandler,
			path:               "h/public/readme.txt",
			expectedStatusCode: 200,
		},
		"allowed dir listing via share handler, 200": {
			handler:            publicShareHandler,
			path:               "h/public/",
			expectedStatusCode: 200,
		},
	}

	for name, tc := range testCases {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbPath := filepath.Join(t.TempDir(), "db")
			db, err := storm.Open(dbPath)
			if err != nil {
				t.Fatalf("failed to open db: %v", err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Errorf("failed to close db: %v", err)
				}
			})

			storage, err := bolt.NewStorage(db)
			if err != nil {
				t.Fatalf("failed to get storage: %v", err)
			}
			if err := storage.Share.Save(&share.Link{Hash: "h", UserID: 1, Path: "/projects"}); err != nil {
				t.Fatalf("failed to save share: %v", err)
			}
			if err := storage.Users.Save(&users.User{
				Username: "username",
				Password: "pw",
				Perm:     users.Permissions{Share: true, Download: true},
				Rules: []rules.Rule{
					{Allow: false, Path: "/projects/private"},
				},
			}); err != nil {
				t.Fatalf("failed to save user: %v", err)
			}
			if err := storage.Settings.Save(&settings.Settings{Key: []byte("key")}); err != nil {
				t.Fatalf("failed to save settings: %v", err)
			}

			fs := files.NewScopedFs(afero.NewOsFs(), t.TempDir())
			if err := fs.MkdirAll("/projects/private", 0o755); err != nil {
				t.Fatalf("failed to create private dir: %v", err)
			}
			if err := fs.MkdirAll("/projects/public", 0o755); err != nil {
				t.Fatalf("failed to create public dir: %v", err)
			}
			if err := afero.WriteFile(fs, "/projects/private/secret.txt", []byte("top secret"), 0o600); err != nil {
				t.Fatalf("failed to write secret file: %v", err)
			}
			if err := afero.WriteFile(fs, "/projects/public/readme.txt", []byte("hello"), 0o600); err != nil {
				t.Fatalf("failed to write public file: %v", err)
			}

			storage.Users = &customFSUser{
				Store: storage.Users,
				fs:    fs,
			}

			req := newHTTPRequest(t, func(r *http.Request) { r.URL.Path = tc.path })

			recorder := httptest.NewRecorder()
			handler := handle(tc.handler, "", storage, &settings.Server{})

			handler.ServeHTTP(recorder, req)
			result := recorder.Result()
			defer result.Body.Close()
			if result.StatusCode != tc.expectedStatusCode {
				t.Errorf("expected status code %d, got status code %d", tc.expectedStatusCode, result.StatusCode)
			}
		})
	}
}

func newHTTPRequest(t *testing.T, requestModifiers ...func(*http.Request)) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, "h", http.NoBody)
	if err != nil {
		t.Fatalf("failed to construct request: %v", err)
	}
	for _, modify := range requestModifiers {
		modify(r)
	}
	return r
}

type customFSUser struct {
	users.Store
	fs afero.Fs
	// followExternal mirrors Server.FollowExternalSymlinks: when set, the
	// provided fs is used as-is (a bare BasePathFs that follows symlinks);
	// otherwise it is wrapped in a symlink-confining ScopedFs.
	followExternal bool
}

func (cu *customFSUser) Get(baseScope string, followExternalSymlinks bool, id interface{}) (*users.User, error) {
	user, err := cu.Store.Get(baseScope, followExternalSymlinks, id)
	if err != nil {
		return nil, err
	}
	// Inject a filesystem rooted at the test's temp scope, standing in for the
	// one users.User.Clean would build in production.
	if cu.followExternal {
		user.Fs = cu.fs
	} else {
		user.Fs = files.NewScopedFs(cu.fs, "/")
	}

	return user, nil
}
