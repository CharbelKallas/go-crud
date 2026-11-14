package weather

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
)

// SigningTransport is an http.RoundTripper that adds a client-id header
// and, for requests with a body, an HMAC-SHA256 signature of the body
// (base64, in the "signature" header).
type SigningTransport struct {
	Secret   string
	ClientID string
	// Base is the underlying transport; http.DefaultTransport when nil.
	Base http.RoundTripper
}

func (t *SigningTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A RoundTripper must not modify the caller's request.
	req = req.Clone(req.Context())

	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		mac := hmac.New(sha256.New, []byte(t.Secret))
		mac.Write(body)
		req.Header.Set("signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}
	req.Header.Set("client-id", t.ClientID)

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
