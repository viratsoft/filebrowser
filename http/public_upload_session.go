package fbhttp

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const uploadSessionTTL = 24 * time.Hour

type uploadSession struct {
	expires time.Time
	files   map[string]struct{}
}

type uploadSessionStore struct {
	sync.Mutex
	sessions map[string]*uploadSession
}

var publicUploadSessions = uploadSessionStore{sessions: map[string]*uploadSession{}}

func uploadSessionCookieName(hash string) string { return "fb_upload_" + hash }

func (s *uploadSessionStore) session(w http.ResponseWriter, r *http.Request, hash string) string {
	cookie, err := r.Cookie(uploadSessionCookieName(hash))
	if err == nil && cookie.Value != "" {
		s.Lock()
		defer s.Unlock()
		if session := s.sessions[hash+":"+cookie.Value]; session != nil && time.Now().Before(session.expires) {
			return cookie.Value
		}
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	id := base64.RawURLEncoding.EncodeToString(b)
	s.Lock()
	s.sessions[hash+":"+id] = &uploadSession{expires: time.Now().Add(uploadSessionTTL), files: map[string]struct{}{}}
	s.Unlock()
	http.SetCookie(w, &http.Cookie{Name: uploadSessionCookieName(hash), Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(uploadSessionTTL.Seconds())})
	return id
}

func (s *uploadSessionStore) add(hash, id, file string) {
	s.Lock()
	defer s.Unlock()
	if session := s.sessions[hash+":"+id]; session != nil && time.Now().Before(session.expires) {
		session.files[file] = struct{}{}
	}
}

func (s *uploadSessionStore) files(hash, id string) map[string]struct{} {
	s.Lock()
	defer s.Unlock()
	result := map[string]struct{}{}
	if session := s.sessions[hash+":"+id]; session != nil && time.Now().Before(session.expires) {
		for name := range session.files {
			result[name] = struct{}{}
		}
	}
	return result
}
