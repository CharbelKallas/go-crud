package weather

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCurrentBuildsEscapedRequest(t *testing.T) {
	var gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/v1/current.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		io.WriteString(w, `{"location":{"name":"Beirut","lat":33.89,"lon":35.5},"current":{"temp_c":21.5,"condition":{"text":"Sunny"}}}`)
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL+"/v1/", "k&ey", nil).Current(context.Background(), 33.8938, 35.5018)
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s", gotMethod)
	}
	if gotQuery != "key=k%26ey&q=33.8938%2C35.5018" {
		t.Errorf("query = %s", gotQuery)
	}
	if got.Location.Name != "Beirut" || got.Current.TemperatureC != 21.5 || got.Current.Condition.Text != "Sunny" {
		t.Errorf("unexpected response %+v", got)
	}
}

func TestCurrentUpstreamErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"API key is invalid"}}`, http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "secret-key", nil).Current(context.Background(), 1, 2)
	if !errors.Is(err, ErrUpstream) || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
}

func TestCurrentErrorDoesNotLeakAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // connection refused

	_, err := NewClient(srv.URL, "super-secret-key", nil).Current(context.Background(), 1, 2)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Fatalf("error leaks the API key: %v", err)
	}
}

func TestSigningTransport(t *testing.T) {
	var gotSig, gotClient, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("signature")
		gotClient = r.Header.Get("client-id")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &SigningTransport{Secret: "s3cret", ClientID: "client-1"}}
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"a":1}`))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte(`{"a":1}`))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); gotSig != want {
		t.Errorf("signature = %q, want %q", gotSig, want)
	}
	if gotClient != "client-1" || gotBody != `{"a":1}` {
		t.Errorf("client-id = %q body = %q", gotClient, gotBody)
	}
	if req.Header.Get("signature") != "" {
		t.Error("transport modified the caller's request")
	}

	// GET without a body: client-id only.
	resp, err = client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotSig != "" || gotClient != "client-1" {
		t.Errorf("GET: signature = %q client-id = %q", gotSig, gotClient)
	}
}
