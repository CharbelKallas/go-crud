//go:build integration

// Package integration runs the repositories and the full HTTP API against
// a real MySQL database. Run with:
//
//	docker compose up -d mysql
//	INTEGRATION_ALLOW_DB_RESET=1 DB_USERNAME=... DB_PASSWORD=... DATABASE_ADDR=127.0.0.1:3307 \
//	  go test -tags integration ./integration/...
package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"go-crud/auth"
	"go-crud/config"
	"go-crud/database"
	"go-crud/handlers"
	"go-crud/models"
	"go-crud/repositories"
	"go-crud/router"
)

var testDB *sql.DB
var testCfg config.Config

func TestMain(m *testing.M) {
	// The tests delete every row in the user and album tables, so they
	// only run against a database explicitly marked as disposable.
	if os.Getenv("INTEGRATION_ALLOW_DB_RESET") != "1" {
		fmt.Println("skipping integration tests: set INTEGRATION_ALLOW_DB_RESET=1 to run them (they wipe the user and album tables)")
		os.Exit(0)
	}
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", strings.Repeat("i", config.MinJWTSecretLength))
	}
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	testCfg = cfg

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg)
	if err != nil {
		panic(err)
	}
	// Applying twice must be a no-op.
	for i := 0; i < 2; i++ {
		if err := database.Migrate(cfg); err != nil {
			panic(err)
		}
	}
	testDB = db
	code := m.Run()
	db.Close()
	os.Exit(code)
}

func reset(t *testing.T) {
	t.Helper()
	for _, table := range []string{"refresh_token", "album", "user"} {
		if _, err := testDB.Exec("DELETE FROM " + table); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAlbumRepository(t *testing.T) {
	reset(t)
	ctx := context.Background()
	repo := repositories.NewAlbumRepository(testDB)

	list, err := repo.List(ctx)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list = %#v, %v", list, err)
	}

	id, err := repo.Create(ctx, models.Album{Title: "Blue Train", Artist: "John Coltrane", Price: 56.99})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, id)
	if err != nil || got != (models.Album{ID: id, Title: "Blue Train", Artist: "John Coltrane", Price: 56.99}) {
		t.Fatalf("get = %+v, %v", got, err)
	}

	// Updating with identical values must not look like "not found".
	if err := repo.Update(ctx, id, got); err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	if err := repo.Update(ctx, id+1000, got); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	byArtist, err := repo.ListByArtist(ctx, "John Coltrane")
	if err != nil || len(byArtist) != 1 {
		t.Fatalf("by artist = %v, %v", byArtist, err)
	}

	if err := repo.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, id); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := repo.Get(ctx, id); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("get deleted: %v", err)
	}
}

func TestUserRepository(t *testing.T) {
	reset(t)
	ctx := context.Background()
	repo := repositories.NewUserRepository(testDB)

	u := models.User{Name: "Ann", Email: "ann@example.com", Password: "hash"}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, u); !errors.Is(err, repositories.ErrDuplicateEmail) {
		t.Fatalf("duplicate: %v", err)
	}
	got, err := repo.GetByEmail(ctx, "ann@example.com")
	if err != nil || got.Name != "Ann" || got.Password != "hash" || got.ID == 0 {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if err := repo.UpdatePasswordHash(ctx, got.ID, "hash2"); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.GetByEmail(ctx, "ann@example.com"); again.Password != "hash2" {
		t.Fatalf("password not updated: %+v", again)
	}
	if _, err := repo.GetByEmail(ctx, "nobody@example.com"); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

// TestEndToEnd drives the real router over HTTP with the real repositories.
func TestEndToEnd(t *testing.T) {
	reset(t)
	gin.SetMode(gin.TestMode)
	tokens := auth.NewTokenManager(testCfg.JWTSecret, testCfg.JWTIssuer, time.Minute, time.Hour)
	authHandler, err := handlers.NewAuthHandler(repositories.NewUserRepository(testDB), repositories.NewRefreshTokenRepository(testDB), tokens, bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router.New(router.Deps{
		Tokens:  tokens,
		Auth:    authHandler,
		Albums:  handlers.NewAlbumHandler(repositories.NewAlbumRepository(testDB)),
		Weather: handlers.NewWeatherHandler(nil),
		DB:      testDB,
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	call := func(method, path, token string, body any, wantStatus int, out any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != wantStatus {
			var b bytes.Buffer
			b.ReadFrom(resp.Body)
			t.Fatalf("%s %s = %d, want %d: %s", method, path, resp.StatusCode, wantStatus, b.String())
		}
		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
	}

	call("GET", "/healthz", "", nil, 200, nil)

	signup := map[string]string{"name": "Ann", "email": "ann@example.com", "password": "correct horse"}
	call("POST", "/api/auth/signup", "", signup, 201, nil)
	call("POST", "/api/auth/signup", "", signup, 409, nil)

	var login handlers.LoginResponse
	call("POST", "/api/auth/login", "", map[string]string{"email": "ann@example.com", "password": "nope-nope"}, 401, nil)
	call("POST", "/api/auth/login", "", map[string]string{"email": "ann@example.com", "password": "correct horse"}, 200, &login)

	var empty []models.Album
	call("GET", "/api/albums", login.Token, nil, 200, &empty)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v", empty)
	}

	var id int64
	call("POST", "/api/albums", login.Token, map[string]any{"title": "Blue Train", "artist": "John Coltrane", "price": 56.99}, 201, &id)
	var album models.Album
	call("GET", "/api/albums/"+itoa(id), login.Token, nil, 200, &album)
	if album.Title != "Blue Train" || album.Price != 56.99 {
		t.Fatalf("album = %+v", album)
	}
	call("PUT", "/api/albums/"+itoa(id), login.Token, map[string]any{"title": "Blue Train", "artist": "John Coltrane", "price": 56.99}, 200, nil)
	call("PUT", "/api/albums/"+itoa(id), login.Token, map[string]any{"title": "Blue Train", "artist": "John Coltrane", "price": 60}, 200, nil)
	var byArtist []models.Album
	call("GET", "/api/albums/artist/John%20Coltrane", login.Token, nil, 200, &byArtist)
	if len(byArtist) != 1 || byArtist[0].Price != 60 {
		t.Fatalf("by artist = %+v", byArtist)
	}
	call("DELETE", "/api/albums/"+itoa(id), login.Token, nil, 200, nil)
	call("GET", "/api/albums/"+itoa(id), login.Token, nil, 404, nil)

	// Refresh flow.
	call("GET", "/api/albums", login.RefreshToken, nil, 401, nil)
	var refreshed handlers.LoginResponse
	call("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": login.RefreshToken}, 200, &refreshed)
	call("GET", "/api/albums", refreshed.Token, nil, 200, nil)

	// Reusing a refresh token revokes the whole family.
	call("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": login.RefreshToken}, 401, nil)
	call("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": refreshed.RefreshToken}, 401, nil)

	// A fresh login works again.
	var again handlers.LoginResponse
	call("POST", "/api/auth/login", "", map[string]string{"email": "ann@example.com", "password": "correct horse"}, 200, &again)
	call("POST", "/api/auth/refresh", "", map[string]string{"RefreshToken": again.RefreshToken}, 200, nil)
}

func TestRefreshTokenRepository(t *testing.T) {
	reset(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(testDB)
	repo := repositories.NewRefreshTokenRepository(testDB)
	if err := users.Create(ctx, models.User{Name: "A", Email: "a@example.com", Password: "h"}); err != nil {
		t.Fatal(err)
	}
	u, _ := users.GetByEmail(ctx, "a@example.com")

	future := time.Now().Add(time.Hour)
	for _, id := range []string{"t1", "t2"} {
		if err := repo.Save(ctx, id, u.ID, future); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Consume(ctx, "t1", u.ID+1); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("other user's token: %v", err)
	}
	if err := repo.Consume(ctx, "t1", u.ID); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if err := repo.Consume(ctx, "t1", u.ID); !errors.Is(err, repositories.ErrTokenReused) {
		t.Fatalf("second use: %v", err)
	}
	if err := repo.Consume(ctx, "t2", u.ID); !errors.Is(err, repositories.ErrTokenReused) {
		t.Fatalf("sibling not revoked: %v", err)
	}
	if err := repo.Consume(ctx, "nope", u.ID); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}

	// Expired rows are pruned on the next save.
	if err := repo.Save(ctx, "old", u.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, "t3", u.ID, future); err != nil {
		t.Fatal(err)
	}
	var n int
	testDB.QueryRow("SELECT COUNT(*) FROM refresh_token WHERE jti = 'old'").Scan(&n)
	if n != 0 {
		t.Fatal("expired token not pruned")
	}

	// Deleting the user deletes their tokens.
	if _, err := testDB.Exec("DELETE FROM user WHERE id = ?", u.ID); err != nil {
		t.Fatal(err)
	}
	testDB.QueryRow("SELECT COUNT(*) FROM refresh_token").Scan(&n)
	if n != 0 {
		t.Fatalf("%d tokens left after user deletion", n)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
