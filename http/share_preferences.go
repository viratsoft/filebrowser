package fbhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/filebrowser/filebrowser/v2/users"
)

type sharePreferencesResponse struct {
	ShowThumbnails bool `json:"showThumbnails"`
}

// Serialize preference writes with generic user updates, which may carry an
// older snapshot of the user from another tab or an older client.
var sharePreferencesWriteMu sync.Mutex

// Match this exact path for every method so unsupported requests cannot fall
// through to the legacy /share path-prefix create/delete handlers.
var sharePreferencesHandler handleFunc = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	switch r.Method {
	case http.MethodGet:
		return sharePreferencesGetHandler(w, r, d)
	case http.MethodPut:
		return sharePreferencesPutHandler(w, r, d)
	default:
		w.Header().Set("Allow", "GET, PUT")
		return http.StatusMethodNotAllowed, nil
	}
}

var sharePreferencesGetHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	return renderJSON(w, r, sharePreferencesResponse{ShowThumbnails: !d.user.HideShareThumbnails})
})

// The owner is always the authenticated user, never a client-supplied ID.
// Update only this preference so saving cannot reset permissions or other data.
var sharePreferencesPutHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	var body struct {
		ShowThumbnails *bool `json:"showThumbnails"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.ShowThumbnails == nil {
		return http.StatusBadRequest, nil
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return http.StatusBadRequest, nil
	}
	sharePreferencesWriteMu.Lock()
	defer sharePreferencesWriteMu.Unlock()
	err := d.store.Users.Update(&users.User{ID: d.user.ID, HideShareThumbnails: !*body.ShowThumbnails}, "HideShareThumbnails")
	if err != nil {
		return errToStatus(err), err
	}
	return renderJSON(w, r, sharePreferencesResponse{ShowThumbnails: *body.ShowThumbnails})
})
