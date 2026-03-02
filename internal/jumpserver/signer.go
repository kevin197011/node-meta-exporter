package jumpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	dateFormat = "Mon, 02 Jan 2006 15:04:05 GMT"
)

// Signer implements HTTP Signature authentication for JumpServer access keys.
// It follows draft-cavage-http-signatures with HMAC-SHA256.
type Signer struct {
	KeyID  string
	Secret string
}

// NewSigner creates a new HTTP Signature signer.
func NewSigner(keyID, secret string) *Signer {
	return &Signer{KeyID: keyID, Secret: secret}
}

// Sign adds the required Date, Authorization (Signature) headers to the request.
// JumpServer expects: (request-target) accept date
func (s *Signer) Sign(req *http.Request) error {
	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", time.Now().UTC().Format(dateFormat))
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	signedHeaders := []string{"(request-target)", "accept", "date"}
	signingString := s.buildSigningString(req, signedHeaders)

	mac := hmac.New(sha256.New, []byte(s.Secret))
	mac.Write([]byte(signingString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	authHeader := fmt.Sprintf(
		`Signature keyId="%s",algorithm="hmac-sha256",headers="%s",signature="%s"`,
		s.KeyID,
		strings.Join(signedHeaders, " "),
		signature,
	)
	req.Header.Set("Authorization", authHeader)

	return nil
}

// buildSigningString constructs the string to sign per draft-cavage-http-signatures.
func (s *Signer) buildSigningString(req *http.Request, headers []string) string {
	var lines []string
	for _, h := range headers {
		switch h {
		case "(request-target)":
			path := req.URL.Path
			if req.URL.RawQuery != "" {
				path = path + "?" + req.URL.RawQuery
			}
			lines = append(lines, fmt.Sprintf("(request-target): %s %s",
				strings.ToLower(req.Method), path))
		default:
			lines = append(lines, fmt.Sprintf("%s: %s",
				strings.ToLower(h), req.Header.Get(h)))
		}
	}
	return strings.Join(lines, "\n")
}
