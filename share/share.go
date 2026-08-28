package share

type CreateBody struct {
	Password            string `json:"password"`
	Expires             string `json:"expires"`
	Unit                string `json:"unit"`
	AllowUpload         bool   `json:"allowUpload"`
	UploadOnly          bool   `json:"uploadOnly"`
	SessionUploadFolder bool   `json:"sessionUploadFolder"`
	Name                string `json:"name"`
}

// Link is the information needed to build a shareable link.
type Link struct {
	Hash   string `json:"hash" storm:"id,index"`
	Path   string `json:"path" storm:"index"`
	UserID uint   `json:"userID"`
	Expire int64  `json:"expire"`
	// AllowUpload permits unauthenticated uploads to a shared directory.
	// It is opt-in so existing public shares remain read-only.
	AllowUpload bool `json:"allowUpload"`
	UploadOnly  bool `json:"uploadOnly"`
	// SessionUploadFolder puts each upload-only visitor session into an isolated,
	// server-generated subdirectory below the shared folder.
	SessionUploadFolder bool   `json:"sessionUploadFolder"`
	Name                string `json:"name"`
	PasswordHash        string `json:"password_hash,omitempty"`
	// Token is a random value that will only be set when PasswordHash is set. It is
	// URL-Safe and is used to download links in password-protected shares via a
	// query arg.
	Token string `json:"token,omitempty"`
}
