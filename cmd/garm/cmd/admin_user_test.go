// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/cloudbase/garm/auth"
	"github.com/cloudbase/garm/config"
	"github.com/cloudbase/garm/database"
	dbCommon "github.com/cloudbase/garm/database/common"
	"github.com/cloudbase/garm/database/watcher"
	garmTesting "github.com/cloudbase/garm/internal/testing"
	"github.com/cloudbase/garm/params"
)

const testJWTSecret = "aChRw8Lr3zyX2pVq9TbKs5mNe7Gd4Hj6" //nolint:gosec

func newTestStore(t *testing.T) dbCommon.Store {
	t.Helper()
	ctx := context.Background()
	watcher.InitWatcher(ctx)
	t.Cleanup(func() { _ = watcher.CloseWatcher() })
	db, err := database.NewDatabase(ctx, garmTesting.GetTestSqliteDBConfig(t))
	require.NoError(t, err)

	_, err = db.CreateUser(ctx, params.NewUserParams{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "x",
		IsAdmin:  true,
		Enabled:  true,
	})
	require.NoError(t, err)
	return db
}

func testJWTConfig(t *testing.T, ttl string) config.JWTAuth {
	t.Helper()
	cfg := config.JWTAuth{Secret: testJWTSecret}
	require.NoError(t, cfg.TimeToLive.UnmarshalText([]byte(ttl)))
	return cfg
}

// apiStatus sends a request with the token through the same middleware chain
// the API router uses and returns the resulting status code.
func apiStatus(t *testing.T, db dbCommon.Store, jwtCfg config.JWTAuth, method, token string) int {
	t.Helper()
	jwtMw, err := auth.NewjwtMiddleware(db, jwtCfg)
	require.NoError(t, err)
	handler := jwtMw.Middleware(auth.AdminRequiredMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(method, "/api/v1/pools", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

func TestCreateServiceUser(t *testing.T) {
	ctx := context.Background()
	db := newTestStore(t)

	user, err := createServiceUser(ctx, db, "svc-reader", "")
	require.NoError(t, err)
	require.Equal(t, "svc-reader@"+serviceUserEmailDomain, user.Email)
	require.Equal(t, "svc-reader", user.FullName)
	require.False(t, user.IsAdmin)
	require.True(t, user.Enabled)

	_, err = createServiceUser(ctx, db, "svc-reader", "")
	require.Error(t, err, "duplicate username")

	// 64 characters, the longest name that fits.
	_, err = createServiceUser(ctx, db, "svc-"+strings.Repeat("a", 60), "")
	require.NoError(t, err)

	for _, bad := range []string{
		"", "reader", "svc-", "svc--leading", "svc-Upper", "svc-has space", "svc-a@b.com",
		"svc-" + strings.Repeat("a", 61),
	} {
		_, err = createServiceUser(ctx, db, bad, "")
		require.ErrorContains(t, err, "invalid username", bad)
	}
}

func TestValidateServiceTokenTTL(t *testing.T) {
	for _, good := range []string{"24h", "720h", "8784h"} {
		require.NoError(t, validateServiceTokenTTL(good), good)
	}
	for _, bad := range []string{"1h", "23h59m", "-720h", "8785h", "876000h"} {
		require.ErrorContains(t, validateServiceTokenTTL(bad), "out of range", bad)
	}
	require.ErrorContains(t, validateServiceTokenTTL("30d"), "parsing --ttl")
}

func TestServiceUserTokenLifecycle(t *testing.T) {
	ctx := context.Background()
	db := newTestStore(t)
	jwtCfg := testJWTConfig(t, "8760h")

	_, err := createServiceUser(ctx, db, "svc-reader", "")
	require.NoError(t, err)

	token, _, err := issueServiceUserToken(ctx, db, jwtCfg, "svc-reader")
	require.NoError(t, err)

	// Read-only: GET passes, mutations are rejected.
	require.Equal(t, http.StatusOK, apiStatus(t, db, jwtCfg, http.MethodGet, token))
	require.Equal(t, http.StatusUnauthorized, apiStatus(t, db, jwtCfg, http.MethodPost, token))
	require.Equal(t, http.StatusUnauthorized, apiStatus(t, db, jwtCfg, http.MethodDelete, token))

	// Disabling rejects existing tokens and refuses new ones; enabling restores them.
	require.NoError(t, setServiceUserEnabled(ctx, db, "svc-reader", false))
	require.Equal(t, http.StatusUnauthorized, apiStatus(t, db, jwtCfg, http.MethodGet, token))
	_, _, err = issueServiceUserToken(ctx, db, jwtCfg, "svc-reader")
	require.ErrorContains(t, err, "disabled")
	require.NoError(t, setServiceUserEnabled(ctx, db, "svc-reader", true))
	require.Equal(t, http.StatusOK, apiStatus(t, db, jwtCfg, http.MethodGet, token))

	// Revoking kills earlier tokens; tokens issued afterwards work.
	require.NoError(t, revokeServiceUserTokens(ctx, db, "svc-reader"))
	require.Equal(t, http.StatusUnauthorized, apiStatus(t, db, jwtCfg, http.MethodGet, token))
	fresh, _, err := issueServiceUserToken(ctx, db, jwtCfg, "svc-reader")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, apiStatus(t, db, jwtCfg, http.MethodGet, fresh))
}

func TestServiceUserTokenTTL(t *testing.T) {
	ctx := context.Background()
	db := newTestStore(t)
	_, err := createServiceUser(ctx, db, "svc-reader", "")
	require.NoError(t, err)

	token, expiresAt, err := issueServiceUserToken(ctx, db, testJWTConfig(t, "720h"), "svc-reader")
	require.NoError(t, err)

	claims := &auth.JWTClaims{}
	_, err = jwt.ParseWithClaims(token, claims, func(*jwt.Token) (interface{}, error) {
		return []byte(testJWTSecret), nil
	})
	require.NoError(t, err)
	require.InDelta(t, 720*3600, time.Until(claims.ExpiresAt.Time).Seconds(), 60)
	require.True(t, expiresAt.Equal(claims.ExpiresAt.Time), "reported expiry matches the token")
	require.False(t, claims.IsAdmin)
	require.Equal(t, "svc-reader", claims.Username)
}

func TestServiceUserCommandsRefuseAdmin(t *testing.T) {
	ctx := context.Background()
	db := newTestStore(t)
	jwtCfg := testJWTConfig(t, "24h")

	_, _, err := issueServiceUserToken(ctx, db, jwtCfg, "admin")
	require.ErrorContains(t, err, "not a service user")
	require.ErrorContains(t, revokeServiceUserTokens(ctx, db, "admin"), "not a service user")
	require.ErrorContains(t, setServiceUserEnabled(ctx, db, "admin", false), "is an admin")

	// An admin stays off limits even if it carries the service prefix.
	_, err = db.CreateUser(ctx, params.NewUserParams{
		Username: "svc-root", Email: "svc-root@example.com", Password: "x", IsAdmin: true, Enabled: true,
	})
	require.NoError(t, err)
	_, _, err = issueServiceUserToken(ctx, db, jwtCfg, "svc-root")
	require.ErrorContains(t, err, "is an admin")
	require.ErrorContains(t, revokeServiceUserTokens(ctx, db, "svc-root"), "is an admin")

	_, _, err = issueServiceUserToken(ctx, db, jwtCfg, "svc-nobody")
	require.Error(t, err)
}

// People (e.g. users created by OIDC logins) can be disabled and enabled, but
// never get tokens issued or revoked through these commands.
func TestServiceUserCommandsOnPeople(t *testing.T) {
	ctx := context.Background()
	db := newTestStore(t)
	jwtCfg := testJWTConfig(t, "24h")

	person, err := db.CreateUser(ctx, params.NewUserParams{
		Username: "jane", Email: "jane@example.com", IsSSOUser: true, Enabled: true,
	})
	require.NoError(t, err)

	_, _, err = issueServiceUserToken(ctx, db, jwtCfg, "jane")
	require.ErrorContains(t, err, "not a service user")
	require.ErrorContains(t, revokeServiceUserTokens(ctx, db, "jane"), "not a service user")
	after, err := db.GetUser(ctx, "jane")
	require.NoError(t, err)
	require.Equal(t, person.Generation, after.Generation, "revoke left the person untouched")

	require.NoError(t, setServiceUserEnabled(ctx, db, "jane", false))
	after, err = db.GetUser(ctx, "jane")
	require.NoError(t, err)
	require.False(t, after.Enabled)
}
