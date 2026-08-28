package fbhttp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

const uploadSessionTTL = 24 * time.Hour
const maxTrackedPublicUploadSessions = 10000

var errTooManyPublicUploadSessions = errors.New("too many active public upload sessions")

type uploadSession struct {
	ID      string
	Expires int64
	Folder  string
}

type uploadSessionFiles struct {
	expires time.Time
	files   map[string]struct{}
}

type uploadSessionStore struct {
	sync.Mutex
	sessions map[string]*uploadSessionFiles
}

var publicUploadSessions = uploadSessionStore{sessions: map[string]*uploadSessionFiles{}}

func uploadSessionCookieName(hash string) string { return "fb_upload_" + hash }

func (s *uploadSessionStore) session(w http.ResponseWriter, r *http.Request, hash string, key []byte, useFolder bool) (uploadSession, error) {
	if len(key) == 0 {
		return uploadSession{}, errors.New("missing signing key for public upload session")
	}

	if cookie, err := r.Cookie(uploadSessionCookieName(hash)); err == nil {
		if session, ok := verifyUploadSession(cookie.Value, hash, key); ok && (session.Folder != "") == useFolder {
			if session.Folder == "" {
				if err := s.ensureFiles(hash, session); err != nil {
					return uploadSession{}, err
				}
			}
			return session, nil
		}
	}

	id, err := randomUploadSessionID()
	if err != nil {
		return uploadSession{}, err
	}
	session := uploadSession{ID: id, Expires: time.Now().Add(uploadSessionTTL).Unix()}
	if useFolder {
		session.Folder = "upload_" + time.Now().Format("2006-01-02_03-04-05_PM") + "_" + id[:10]
	}

	if session.Folder == "" {
		if err := s.ensureFiles(hash, session); err != nil {
			return uploadSession{}, err
		}
	}
	cookie, err := signUploadSession(session, hash, key)
	if err != nil {
		return uploadSession{}, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     uploadSessionCookieName(hash),
		Value:    cookie,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(uploadSessionTTL.Seconds()),
	})
	return session, nil
}

func randomUploadSessionID() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func signUploadSession(session uploadSession, hash string, key []byte) (string, error) {
	payload, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(hash + "." + encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

func verifyUploadSession(value, hash string, key []byte) (uploadSession, bool) {
	parts := splitUploadSessionCookie(value)
	if len(parts) != 2 {
		return uploadSession{}, false
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(hash + "." + parts[0]))
	expected := mac.Sum(nil)
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || subtle.ConstantTimeCompare(expected, provided) != 1 {
		return uploadSession{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return uploadSession{}, false
	}
	var session uploadSession
	if err := json.Unmarshal(payload, &session); err != nil || session.ID == "" || session.Expires <= time.Now().Unix() {
		return uploadSession{}, false
	}
	return session, true
}

func splitUploadSessionCookie(value string) []string {
	for i := 0; i < len(value); i++ {
		if value[i] == '.' {
			return []string{value[:i], value[i+1:]}
		}
	}
	return nil
}

func (s *uploadSessionStore) ensureFiles(hash string, session uploadSession) error {
	s.Lock()
	defer s.Unlock()
	now := time.Now()
	for key, stored := range s.sessions {
		if !now.Before(stored.expires) {
			delete(s.sessions, key)
		}
	}
	key := hash + ":" + session.ID
	if s.sessions[key] == nil {
		if len(s.sessions) >= maxTrackedPublicUploadSessions {
			return errTooManyPublicUploadSessions
		}
		s.sessions[key] = &uploadSessionFiles{expires: time.Unix(session.Expires, 0), files: map[string]struct{}{}}
	}
	return nil
}

func (s *uploadSessionStore) add(hash string, session uploadSession, file string) {
	if err := s.ensureFiles(hash, session); err != nil {
		return
	}
	s.Lock()
	defer s.Unlock()
	if stored := s.sessions[hash+":"+session.ID]; stored != nil && time.Now().Before(stored.expires) {
		stored.files[file] = struct{}{}
	}
}

func (s *uploadSessionStore) files(hash string, session uploadSession) map[string]struct{} {
	s.Lock()
	defer s.Unlock()
	result := map[string]struct{}{}
	if stored := s.sessions[hash+":"+session.ID]; stored != nil && time.Now().Before(stored.expires) {
		for name := range stored.files {
			result[name] = struct{}{}
		}
	}
	return result
}
