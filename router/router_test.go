package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"go-crud/auth"
	"go-crud/handlers"
	"go-crud/models"
	"go-crud/repositories"
	"go-crud/router"
	"go-crud/weather"
)

// --- fakes -----------------------------------------------------------------

type fakeUsers struct {
	mu    sync.Mutex
	users map[string]models.User
}

func (f *fakeUsers) Create(_ context.Context, u models.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[u.Email]; ok {
		return repositories.ErrDuplicateEmail
	}
	u.ID = int64(len(f.users) + 1)
	f.users[u.Email] = u
	return nil
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[email]
	if !ok {
		return u, repositories.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsers) UpdatePasswordHash(_ context.Context, id int64, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for email, u := range f.users {
		if u.ID == id {
			u.Password = hash
			f.users[email] = u
			return nil
		}
	}
	return repositories.ErrNotFound
}

type fakeAlbums struct {
	mu     sync.Mutex
	nextID int64
	albums map[int64]models.Album
	fail   error
}

func (f *fakeAlbums) List(context.Context) ([]models.Album, error) {
	return f.filter(func(models.Album) bool { return true })
}

func (f *fakeAlbums) ListByArtist(_ context.Context, artist string) ([]models.Album, error) {
	return f.filter(func(a models.Album) bool { return a.Artist == artist })
}

func (f *fakeAlbums) filter(keep func(models.Album) bool) ([]models.Album, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	out := []models.Album{}
	for _, a := range f.albums {
		if keep(a) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeAlbums) Get(_ context.Context, id int64) (models.Album, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.albums[id]
	if !ok {
		return a, repositories.ErrNotFound
	}
	return a, nil
}

func (f *fakeAlbums) Create(_ context.Context, a models.Album) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	a.ID = f.nextID
	f.albums[a.ID] = a
	return a.ID, nil
}

func (f *fakeAlbums) Update(_ context.Context, id int64, a models.Album) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.albums[id]; !ok {
		return repositories.ErrNotFound
	}
	a.ID = id
	f.albums[id] = a
	return nil
}

func (f *fakeAlbums) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.albums[id]; !ok {
		return repositories.ErrNotFound
	}
	delete(f.albums, id)
	return nil
}

type fakeWeather struct {
	lat, lon float64
	err      error
}

func (f *fakeWeather) Current(_ context.Context, lat, lon float64) (models.WeatherResponse, error) {
	f.lat, f.lon = lat, lon
	if f.err != nil {
		return models.WeatherResponse{}, f.err
	}
	return models.WeatherResponse{Location: models.LocationInfo{Name: "Beirut"}}, nil
}

type fakeRefresh struct {
	mu       sync.Mutex
	grace    bool // retries of a just-used token return its child, like the DB grace window
	tokens   map[string]*fakeRefreshRow
	failSave error
}

type fakeRefreshRow struct {
	userID        int64
	used, revoked bool
	replacedBy    string
	expiresAt     time.Time
}

func newFakeRefresh() *fakeRefresh { return &fakeRefresh{tokens: map[string]*fakeRefreshRow{}} }

func (f *fakeRefresh) Save(_ context.Context, id string, userID int64, exp time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[id] = &fakeRefreshRow{userID: userID, expiresAt: exp}
	return nil
}

func (f *fakeRefresh) Rotate(_ context.Context, oldID string, userID int64, newID string, exp time.Time) (repositories.Rotated, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.tokens[oldID]
	if !ok || row.userID != userID {
		return repositories.Rotated{}, repositories.ErrNotFound
	}
	switch {
	case !row.used && !row.revoked:
		if f.failSave != nil {
			return repositories.Rotated{}, f.failSave // transaction rolled back: old token untouched
		}
		row.used, row.replacedBy = true, newID
		f.tokens[newID] = &fakeRefreshRow{userID: userID, expiresAt: exp}
		return repositories.Rotated{ID: newID, ExpiresAt: exp}, nil
	case row.used && !row.revoked && f.grace:
		if child := f.tokens[row.replacedBy]; child != nil && !child.used && !child.revoked {
			return repositories.Rotated{ID: row.replacedBy, ExpiresAt: child.expiresAt}, nil
		}
	}
	for _, r := range f.tokens {
		if r.userID == userID {
			r.revoked = true
		}
	}
	return repositories.Rotated{}, repositories.ErrTokenReused
}

type fakePinger struct {
	err     error
	calls   *atomic.Int32
	release chan struct{} // when set, pings block until it is closed
}

func (f fakePinger) PingContext(ctx context.Context) error {
	if f.calls != nil {
		f.calls.Add(1)
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.err
}

// --- harness ---------------------------------------------------------------

type harness struct {
	t       *testing.T
	engine  *gin.Engine
	tokens  *auth.TokenManager
	users   *fakeUsers
	refresh *fakeRefresh
	albums  *fakeAlbums
	weather *fakeWeather
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &harness{
		t:       t,
		tokens:  auth.NewTokenManager("test-secret-that-is-at-least-32-bytes-long", "test", 15*time.Minute, time.Hour),
		users:   &fakeUsers{users: map[string]models.User{}},
		refresh: newFakeRefresh(),
		albums:  &fakeAlbums{albums: map[int64]models.Album{}},
		weather: &fakeWeather{},
	}
	authHandler, err := handlers.NewAuthHandler(h.users, h.refresh, h.tokens, bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h.engine = router.New(router.Deps{
		Tokens:  h.tokens,
		Auth:    authHandler,
		Albums:  handlers.NewAlbumHandler(h.albums),
		Weather: handlers.NewWeatherHandler(h.weather),
		DB:      fakePinger{},
	})
	return h
}

func (h *harness) do(method, path, token string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var buf bytes.Buffer
	if s, ok := body.(string); ok {
		buf.WriteString(s)
	} else if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	return rec
}

func (h *harness) login() handlers.LoginResponse {
	h.t.Helper()
	if rec := h.do("POST", "/api/auth/signup", "", map[string]string{"name": "Ann", "email": "ann@example.com", "password": "correct horse"}); rec.Code != http.StatusCreated {
		h.t.Fatalf("signup: %d %s", rec.Code, rec.Body)
	}
	rec := h.do("POST", "/api/auth/login", "", map[string]string{"email": "ann@example.com", "password": "correct horse"})
	if rec.Code != http.StatusOK {
		h.t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	var resp handlers.LoginResponse
	decode(h.t, rec, &resp)
	return resp
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, status, rec.Body)
	}
}

// --- tests -----------------------------------------------------------------

func TestSignupLoginRefresh(t *testing.T) {
	h := newHarness(t)
	tokens := h.login()

	// Response keys are unchanged for existing clients.
	rec := h.do("POST", "/api/auth/login", "", map[string]string{"email": "ann@example.com", "password": "correct horse"})
	if !strings.Contains(rec.Body.String(), `"Token"`) || !strings.Contains(rec.Body.String(), `"RefreshToken"`) {
		t.Errorf("login body keys changed: %s", rec.Body)
	}

	// Access token works on protected routes, refresh token does not.
	expect(t, h.do("GET", "/api/albums", tokens.Token, nil), http.StatusOK)
	expect(t, h.do("GET", "/api/albums", tokens.RefreshToken, nil), http.StatusUnauthorized)

	// Refresh endpoint accepts only refresh tokens.
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": tokens.Token}), http.StatusUnauthorized)
	rec = h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": tokens.RefreshToken})
	expect(t, rec, http.StatusOK)
	var refreshed handlers.LoginResponse
	decode(t, rec, &refreshed)
	expect(t, h.do("GET", "/api/albums", refreshed.Token, nil), http.StatusOK)

	// A refresh token works once. Reusing it is treated as theft: it is
	// rejected and every refresh token of the user is revoked, including
	// the one just issued.
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": tokens.RefreshToken}), http.StatusUnauthorized)
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": refreshed.RefreshToken}), http.StatusUnauthorized)

	// A validly signed refresh token that was never issued is rejected.
	unknown, _ := h.tokens.Generate("ann@example.com", auth.RefreshToken)
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": unknown}), http.StatusUnauthorized)

	// A refresh token for a user that no longer exists is rejected.
	ghost, _ := h.tokens.Generate("ghost@example.com", auth.RefreshToken)
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": ghost}), http.StatusUnauthorized)
}

func TestSignupValidation(t *testing.T) {
	h := newHarness(t)
	cases := map[string]any{
		"empty body":     map[string]string{},
		"bad email":      map[string]string{"name": "A", "email": "nope", "password": "longenough"},
		"short password": map[string]string{"name": "A", "email": "a@example.com", "password": "short"},
		"long password":  map[string]string{"name": "A", "email": "a@example.com", "password": strings.Repeat("p", 73)},
		"invalid json":   `{"name":`,
		"blank name":     map[string]string{"name": "   ", "email": "a@example.com", "password": "longenough"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := h.do("POST", "/api/auth/signup", "", body)
			expect(t, rec, http.StatusBadRequest)
			var e struct{ Message string }
			decode(t, rec, &e)
			if e.Message == "" {
				t.Error("missing error message")
			}
		})
	}
}

func TestSignupStoresHashAndRejectsDuplicates(t *testing.T) {
	h := newHarness(t)
	body := map[string]string{"name": "Ann", "email": "ann@example.com", "password": "correct horse"}
	expect(t, h.do("POST", "/api/auth/signup", "", body), http.StatusCreated)
	if u := h.users.users["ann@example.com"]; u.Password == "correct horse" || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte("correct horse")) != nil {
		t.Fatal("password not stored as a bcrypt hash")
	}
	expect(t, h.do("POST", "/api/auth/signup", "", body), http.StatusConflict)
}

func TestSignupAndLoginTrimWhitespace(t *testing.T) {
	h := newHarness(t)
	expect(t, h.do("POST", "/api/auth/signup", "", map[string]string{"name": "  Ann ", "email": " ann@example.com ", "password": "correct horse"}), http.StatusCreated)
	u, ok := h.users.users["ann@example.com"]
	if !ok || u.Name != "Ann" {
		t.Fatalf("stored user = %+v (found %v)", u, ok)
	}
	expect(t, h.do("POST", "/api/auth/login", "", map[string]string{"email": "ann@example.com  ", "password": "correct horse"}), http.StatusOK)
}

func TestLoginFailures(t *testing.T) {
	h := newHarness(t)
	h.login()
	wrongPw := h.do("POST", "/api/auth/login", "", map[string]string{"email": "ann@example.com", "password": "wrong password"})
	unknown := h.do("POST", "/api/auth/login", "", map[string]string{"email": "bob@example.com", "password": "whatever1"})
	expect(t, wrongPw, http.StatusUnauthorized)
	expect(t, unknown, http.StatusUnauthorized)
	if wrongPw.Body.String() != unknown.Body.String() {
		t.Errorf("responses reveal whether the email exists: %s vs %s", wrongPw.Body, unknown.Body)
	}
	expect(t, h.do("POST", "/api/auth/login", "", map[string]string{}), http.StatusBadRequest)
}

func TestLoginUpgradesLegacyHashCost(t *testing.T) {
	h := newHarness(t)
	legacy, _ := bcrypt.GenerateFromPassword([]byte("old password"), bcrypt.MinCost+1)
	h.users.users["old@example.com"] = models.User{ID: 42, Name: "Old", Email: "old@example.com", Password: string(legacy)}

	expect(t, h.do("POST", "/api/auth/login", "", map[string]string{"email": "old@example.com", "password": "old password"}), http.StatusOK)

	stored := h.users.users["old@example.com"].Password
	if cost, _ := bcrypt.Cost([]byte(stored)); cost != bcrypt.MinCost {
		t.Fatalf("hash cost = %d, want %d", cost, bcrypt.MinCost)
	}
	expect(t, h.do("POST", "/api/auth/login", "", map[string]string{"email": "old@example.com", "password": "old password"}), http.StatusOK)
}

func TestAuthMiddleware(t *testing.T) {
	h := newHarness(t)
	tokens := h.login()
	expired := auth.NewTokenManager("test-secret-that-is-at-least-32-bytes-long", "test", -time.Minute, time.Hour)
	expiredTok, _ := expired.Generate("ann@example.com", auth.AccessToken)
	forged := auth.NewTokenManager("undersecretary-undersecretary-undersecretary", "test", time.Hour, time.Hour)
	forgedTok, _ := forged.Generate("ann@example.com", auth.AccessToken)

	for name, header := range map[string]string{
		"missing":       "",
		"no scheme":     tokens.Token,
		"basic scheme":  "Basic " + tokens.Token,
		"empty bearer":  "Bearer ",
		"garbage":       "Bearer not-a-jwt",
		"expired":       "Bearer " + expiredTok,
		"forged secret": "Bearer " + forgedTok,
		"refresh token": "Bearer " + tokens.RefreshToken,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/albums", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			h.engine.ServeHTTP(rec, req)
			expect(t, rec, http.StatusUnauthorized)
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("missing WWW-Authenticate header")
			}
		})
	}

	req := httptest.NewRequest("GET", "/api/albums", nil)
	req.Header.Set("Authorization", "bearer "+tokens.Token) // scheme is case-insensitive
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	expect(t, rec, http.StatusOK)
}

func TestAlbumCRUD(t *testing.T) {
	h := newHarness(t)
	tok := h.login().Token

	// Empty list is [] not null, with or without trailing slash.
	for _, p := range []string{"/api/albums", "/api/albums/"} {
		rec := h.do("GET", p, tok, nil)
		expect(t, rec, http.StatusOK)
		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Errorf("GET %s = %s, want []", p, rec.Body)
		}
	}

	// Create (lowercase keys accepted, as before).
	rec := h.do("POST", "/api/albums", tok, map[string]any{"title": "Blue Train", "artist": "John Coltrane", "price": 56.99})
	expect(t, rec, http.StatusCreated)
	var id int64
	decode(t, rec, &id)
	expect(t, h.do("POST", "/api/albums/", tok, map[string]any{"Title": "Giant Steps", "Artist": "John Coltrane", "Price": 63.99}), http.StatusCreated)

	// Read.
	rec = h.do("GET", "/api/albums/1", tok, nil)
	expect(t, rec, http.StatusOK)
	var got map[string]any
	decode(t, rec, &got)
	if got["ID"] != float64(1) || got["Title"] != "Blue Train" || got["Artist"] != "John Coltrane" || got["Price"] != 56.99 {
		t.Errorf("album = %v", got)
	}
	rec = h.do("GET", "/api/albums/artist/John%20Coltrane", tok, nil)
	expect(t, rec, http.StatusOK)
	var list []models.Album
	decode(t, rec, &list)
	if len(list) != 2 {
		t.Errorf("by artist = %v", list)
	}

	// Update.
	rec = h.do("PUT", "/api/albums/1", tok, map[string]any{"title": "Blue Train (Remastered)", "artist": "John Coltrane", "price": 60})
	expect(t, rec, http.StatusOK)
	decode(t, rec, &id)
	if id != 1 || h.albums.albums[1].Title != "Blue Train (Remastered)" {
		t.Errorf("update: id=%d album=%v", id, h.albums.albums[1])
	}

	// Delete.
	expect(t, h.do("DELETE", "/api/albums/1", tok, nil), http.StatusOK)
	expect(t, h.do("GET", "/api/albums/1", tok, nil), http.StatusNotFound)
	expect(t, h.do("DELETE", "/api/albums/1", tok, nil), http.StatusNotFound)
	expect(t, h.do("PUT", "/api/albums/1", tok, map[string]any{"title": "x", "artist": "y", "price": 1}), http.StatusNotFound)
}

func TestAlbumValidation(t *testing.T) {
	h := newHarness(t)
	tok := h.login().Token

	for name, body := range map[string]any{
		"missing title":  map[string]any{"artist": "a", "price": 1},
		"negative price": map[string]any{"title": "t", "artist": "a", "price": -1},
		"price too high": map[string]any{"title": "t", "artist": "a", "price": 1000},
		"title too long": map[string]any{"title": strings.Repeat("t", 129), "artist": "a", "price": 1},
		"not json":       "nope",
	} {
		t.Run(name, func(t *testing.T) {
			expect(t, h.do("POST", "/api/albums", tok, body), http.StatusBadRequest)
		})
	}
	for _, p := range []string{"/api/albums/abc", "/api/albums/0", "/api/albums/-3"} {
		expect(t, h.do("GET", p, tok, nil), http.StatusBadRequest)
	}

	big := `{"title":"` + strings.Repeat("x", 2<<20) + `"}`
	expect(t, h.do("POST", "/api/albums", tok, big), http.StatusBadRequest)
}

func TestValidationMessages(t *testing.T) {
	h := newHarness(t)
	tok := h.login().Token
	for body, want := range map[string]string{
		`{"title":"` + strings.Repeat("t", 129) + `","artist":"a","price":1}`: "Title must be at most 128 characters",
		`{"title":"t","artist":"a","price":1000}`:                             "Price must be at most 999.99",
	} {
		rec := h.do("POST", "/api/albums", tok, body)
		var e struct{ Message string }
		decode(t, rec, &e)
		if e.Message != want {
			t.Errorf("message = %q, want %q", e.Message, want)
		}
	}
}

func TestInternalErrorsAreHidden(t *testing.T) {
	h := newHarness(t)
	tok := h.login().Token
	h.albums.fail = errors.New("dial tcp 10.0.0.5:3306: connection refused")
	rec := h.do("GET", "/api/albums", tok, nil)
	expect(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("internal error leaked: %s", rec.Body)
	}
}

func TestWeather(t *testing.T) {
	h := newHarness(t)
	tok := h.login().Token

	rec := h.do("GET", "/api/weather/33.8938/35.5018", tok, nil)
	expect(t, rec, http.StatusOK)
	if h.weather.lat != 33.8938 || h.weather.lon != 35.5018 {
		t.Errorf("coordinates = %v,%v", h.weather.lat, h.weather.lon)
	}

	for _, p := range []string{"/api/weather/91/0", "/api/weather/0/181", "/api/weather/NaN/0", "/api/weather/abc/0", "/api/weather/1&key=x/2"} {
		expect(t, h.do("GET", p, tok, nil), http.StatusBadRequest)
	}

	h.weather.err = weather.ErrUpstream
	expect(t, h.do("GET", "/api/weather/1/2", tok, nil), http.StatusBadGateway)

	expect(t, h.do("GET", "/api/weather/1/2", "", nil), http.StatusUnauthorized)
}

func TestWeatherNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := auth.NewTokenManager("test-secret-that-is-at-least-32-bytes-long", "test", time.Minute, time.Hour)
	authHandler, _ := handlers.NewAuthHandler(&fakeUsers{users: map[string]models.User{}}, newFakeRefresh(), tokens, bcrypt.MinCost)
	engine := router.New(router.Deps{
		Tokens: tokens, Auth: authHandler, DB: fakePinger{},
		Albums:  handlers.NewAlbumHandler(&fakeAlbums{albums: map[int64]models.Album{}}),
		Weather: handlers.NewWeatherHandler(nil),
	})
	tok, _ := tokens.Generate("a@example.com", auth.AccessToken)
	req := httptest.NewRequest("GET", "/api/weather/1/2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	expect(t, rec, http.StatusServiceUnavailable)
}

func TestAuthRoutesAreRateLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := auth.NewTokenManager("test-secret-that-is-at-least-32-bytes-long", "test", time.Minute, time.Hour)
	authHandler, _ := handlers.NewAuthHandler(&fakeUsers{users: map[string]models.User{}}, newFakeRefresh(), tokens, bcrypt.MinCost)
	engine := router.New(router.Deps{
		Tokens: tokens, Auth: authHandler, DB: fakePinger{},
		Albums:                 handlers.NewAlbumHandler(&fakeAlbums{albums: map[int64]models.Album{}}),
		Weather:                handlers.NewWeatherHandler(nil),
		AuthRateLimitPerMinute: 2,
	})
	var codes []int
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"a@example.com","password":"x"}`)))
		codes = append(codes, rec.Code)
	}
	if codes[0] != http.StatusUnauthorized || codes[1] != http.StatusUnauthorized || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v", codes)
	}
}

func TestFailedRefreshCanBeRetried(t *testing.T) {
	h := newHarness(t)
	tokens := h.login()
	h.refresh.failSave = errors.New("db down")
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": tokens.RefreshToken}), http.StatusInternalServerError)
	h.refresh.failSave = nil
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": tokens.RefreshToken}), http.StatusOK)
}

func TestRefreshRetryWithinGraceReturnsSameToken(t *testing.T) {
	h := newHarness(t)
	h.refresh.grace = true
	tokens := h.login()
	refresh := func() (int, handlers.LoginResponse) {
		rec := h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": tokens.RefreshToken})
		var r handlers.LoginResponse
		if rec.Code == http.StatusOK {
			decode(t, rec, &r)
		}
		return rec.Code, r
	}
	_, first := refresh()
	// The response was "lost" (or the token was replayed): the retry gets
	// the same child token (same jti), not an independent one.
	code, retry := refresh()
	if code != http.StatusOK {
		t.Fatalf("retry = %d", code)
	}
	a, _ := h.tokens.Validate(first.RefreshToken, auth.RefreshToken)
	b, _ := h.tokens.Validate(retry.RefreshToken, auth.RefreshToken)
	if a == nil || b == nil || a.ID != b.ID {
		t.Fatalf("retry issued a different refresh token: %v vs %v", a, b)
	}
	// Once the child is used, the parent can no longer be replayed, and
	// the attempt revokes everything.
	expect(t, h.do("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": first.RefreshToken}), http.StatusOK)
	if code, _ := refresh(); code != http.StatusUnauthorized {
		t.Fatalf("replay after child used = %d", code)
	}
}

func TestRateLimitUsesTrustedProxyHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newEngine := func(proxies []string) *gin.Engine {
		tokens := auth.NewTokenManager("test-secret-that-is-at-least-32-bytes-long", "test", time.Minute, time.Hour)
		authHandler, _ := handlers.NewAuthHandler(&fakeUsers{users: map[string]models.User{}}, newFakeRefresh(), tokens, bcrypt.MinCost)
		return router.New(router.Deps{
			Tokens: tokens, Auth: authHandler, DB: fakePinger{},
			Albums:                 handlers.NewAlbumHandler(&fakeAlbums{albums: map[int64]models.Album{}}),
			Weather:                handlers.NewWeatherHandler(nil),
			AuthRateLimitPerMinute: 1,
			TrustedProxies:         proxies,
		})
	}
	// Two different clients behind the same proxy (httptest's peer is 192.0.2.1).
	codes := func(engine *gin.Engine) []int {
		var out []int
		for _, client := range []string{"203.0.113.10", "203.0.113.20"} {
			req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"a@example.com","password":"x"}`))
			req.Header.Set("X-Forwarded-For", client)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			out = append(out, rec.Code)
		}
		return out
	}
	if got := codes(newEngine([]string{"192.0.2.1"})); got[1] != http.StatusUnauthorized {
		t.Errorf("trusted proxy: second client was limited: %v", got)
	}
	// Untrusted: the header is ignored, both requests share the peer's bucket
	// (and a client cannot dodge the limit by forging the header).
	if got := codes(newEngine(nil)); got[1] != http.StatusTooManyRequests {
		t.Errorf("untrusted proxy: forged header bypassed the limit: %v", got)
	}
}

func TestHealthCachesDatabaseCheck(t *testing.T) {
	var calls atomic.Int32
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", handlers.Health(fakePinger{calls: &calls}))
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
		expect(t, rec, http.StatusOK)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("database pinged %d times, want 1", n)
	}
}

func TestHealthConcurrentRequestsShareOnePing(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", handlers.Health(fakePinger{calls: &calls, release: release}))

	var wg sync.WaitGroup
	codes := make([]int, 10)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
			codes[i] = rec.Code
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	for _, c := range codes {
		if c != http.StatusOK {
			t.Fatalf("codes = %v", codes)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("database pinged %d times, want 1", n)
	}
}

func TestHealthIgnoresCancelledClient(t *testing.T) {
	release := make(chan struct{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", handlers.Health(fakePinger{release: release}))

	// The first client gives up while the ping is still running...
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil).WithContext(ctx))
	expect(t, rec, http.StatusServiceUnavailable)

	// ...but the ping itself is not cancelled, so the next client sees the
	// real (healthy) result rather than a cached failure.
	close(release)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	expect(t, rec, http.StatusOK)
}

func TestHealth(t *testing.T) {
	h := newHarness(t)
	expect(t, h.do("GET", "/healthz", "", nil), http.StatusOK)
	expect(t, h.do("GET", "/", "", nil), http.StatusOK)
}
