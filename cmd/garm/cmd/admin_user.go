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
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/cloudbase/garm-provider-common/util"
	"github.com/cloudbase/garm/auth"
	"github.com/cloudbase/garm/config"
	dbCommon "github.com/cloudbase/garm/database/common"
	"github.com/cloudbase/garm/params"
)

// Service users are non-admin local users meant for automation (CI jobs and
// the like). Non-admin users can only make read-only (GET) API calls, see
// auth.AdminRequiredMiddleware. There is no API to create local users, so
// these commands work directly against the database, like the other admin
// commands, and are run from inside the GARM pod.

// serviceUserPrefix is required on every service user name. Usernames that
// OIDC logins create are stripped to letters and digits, so a name with this
// prefix can never collide with a person's account, and the prefix is how
// the token commands tell service users from people.
const serviceUserPrefix = "svc-"

// serviceUserEmailDomain is the domain of the placeholder email every service
// user gets; the users table requires a unique email. OIDC logins are matched
// to users by email, so a service user must never carry an address a real
// person could log in with. The .invalid TLD is reserved (RFC 2606) and can
// never resolve.
const serviceUserEmailDomain = "service-account.invalid"

// Bounds for "token --ttl". The lower bound is the minimum the JWT code
// honors (appdefaults.DefaultJWTTTL); shorter values would be raised to it
// silently. The upper bound keeps a typo from minting a token that outlives
// the deployment.
const (
	minServiceTokenTTL = 24 * time.Hour
	maxServiceTokenTTL = 366 * 24 * time.Hour
)

// 64 characters in total, the width of the username column.
var serviceUsernameRx = regexp.MustCompile(`^svc-[a-z0-9][a-z0-9._-]{0,59}$`)

var (
	serviceUserFullName string
	serviceUserTokenTTL string
)

var adminUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage non-admin users and service tokens",
	Long: `Manage non-admin users and the API tokens they authenticate with.

Non-admin users can read everything through the API but cannot change
anything. That includes the live log and event streams, the list of users
with their emails, and stored file objects, so treat a token as sensitive
even though it is read-only.

Service users are named "svc-<name>". Use "create" and "token" to give an
automated client read-only access, "revoke-tokens" to invalidate every token
issued to a service user, and "disable" to block any non-admin user
entirely.`,
}

var adminUserListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List all users",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		db, err := openDatabase(ctx)
		if err != nil {
			return err
		}
		users, err := db.ListUsers(ctx)
		if err != nil {
			return fmt.Errorf("listing users: %w", err)
		}
		t := table.NewWriter()
		t.AppendHeader(table.Row{"ID", "Username", "Email", "Admin", "Enabled", "Generation"})
		for _, u := range users {
			t.AppendRow(table.Row{u.ID, u.Username, u.Email, u.IsAdmin, u.Enabled, u.Generation})
		}
		fmt.Println(t.Render())
		return nil
	},
}

var adminUserCreateCmd = &cobra.Command{
	Use:   "create [flags] svc-<name>",
	Short: "Create a non-admin service user",
	Long: `Create a non-admin service user. The name must start with "svc-".

The user gets a placeholder email (svc-<name>@` + serviceUserEmailDomain + `) that
nobody can log in with, and a random password nobody knows. It can only
authenticate with tokens issued by "token".`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		db, err := openDatabase(ctx)
		if err != nil {
			return err
		}
		user, err := createServiceUser(ctx, db, args[0], serviceUserFullName)
		if err != nil {
			return err
		}
		fmt.Printf("Created non-admin user %q.\n", user.Username)
		return nil
	},
}

var adminUserTokenCmd = &cobra.Command{
	Use:   "token [flags] svc-<name>",
	Short: "Issue an API token for a service user",
	Long: `Issue an API token for a service user and print it to stdout. The
expiry is printed to stderr.

The token is valid until it expires, the user is disabled, or the user's
tokens are revoked with "revoke-tokens". It is not stored anywhere, so
issuing a new one does not invalidate earlier ones.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cfg, err := config.NewConfig(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		db, err := openDatabase(ctx)
		if err != nil {
			return err
		}
		jwtCfg := cfg.JWTAuth
		if serviceUserTokenTTL != "" {
			if err := validateServiceTokenTTL(serviceUserTokenTTL); err != nil {
				return err
			}
			if err := jwtCfg.TimeToLive.UnmarshalText([]byte(serviceUserTokenTTL)); err != nil {
				return fmt.Errorf("parsing --ttl: %w", err)
			}
		}
		token, expiresAt, err := issueServiceUserToken(ctx, db, jwtCfg, args[0])
		if err != nil {
			return err
		}
		// Only the token goes to stdout, so it can be captured directly.
		fmt.Fprintf(os.Stderr, "Token for %q expires at %s.\n", args[0], expiresAt.UTC().Format(time.RFC3339))
		fmt.Println(token)
		return nil
	},
}

var adminUserRevokeTokensCmd = &cobra.Command{
	Use:          "revoke-tokens svc-<name>",
	Short:        "Invalidate every token issued to a service user",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		db, err := openDatabase(ctx)
		if err != nil {
			return err
		}
		if err := revokeServiceUserTokens(ctx, db, args[0]); err != nil {
			return err
		}
		fmt.Printf("All tokens issued to %q so far are now invalid.\n", args[0])
		return nil
	},
}

var adminUserDisableCmd = &cobra.Command{
	Use:          "disable <username>",
	Short:        "Disable a non-admin user, rejecting all of its tokens",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetUserEnabled(cmd.Context(), args[0], false)
	},
}

var adminUserEnableCmd = &cobra.Command{
	Use:          "enable <username>",
	Short:        "Re-enable a disabled non-admin user",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetUserEnabled(cmd.Context(), args[0], true)
	},
}

func runSetUserEnabled(ctx context.Context, username string, enabled bool) error {
	db, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	if err := setServiceUserEnabled(ctx, db, username, enabled); err != nil {
		return err
	}
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Printf("User %q is now %s.\n", username, state)
	return nil
}

// getNonAdminUser fetches a user and refuses admins. The admin account is
// managed with "garm admin password-reset"; these commands must not be able
// to mint an admin token or lock the admin out.
func getNonAdminUser(ctx context.Context, db dbCommon.Store, username string) (params.User, error) {
	user, err := db.GetUser(ctx, username)
	if err != nil {
		return params.User{}, fmt.Errorf("fetching user %q: %w", username, err)
	}
	if user.IsAdmin {
		return params.User{}, fmt.Errorf("user %q is an admin; these commands only manage non-admin users", username)
	}
	return user, nil
}

// getServiceUser fetches a service user. Tokens are only issued to, and
// revoked for, service users: a token for a person's account would act as
// that person without their knowledge, and revoking sets a password, which
// users who log in through OIDC do not have.
func getServiceUser(ctx context.Context, db dbCommon.Store, username string) (params.User, error) {
	if !strings.HasPrefix(username, serviceUserPrefix) {
		return params.User{}, fmt.Errorf("user %q is not a service user; this command only works on %s<name> users", username, serviceUserPrefix)
	}
	return getNonAdminUser(ctx, db, username)
}

// validateServiceTokenTTL rejects lifetimes outside the supported range
// instead of letting them be adjusted silently.
func validateServiceTokenTTL(ttl string) error {
	d, err := time.ParseDuration(ttl)
	if err != nil {
		return fmt.Errorf("parsing --ttl: %w", err)
	}
	if d < minServiceTokenTTL || d > maxServiceTokenTTL {
		return fmt.Errorf("--ttl %s is out of range: use between %s and %s", ttl, minServiceTokenTTL, maxServiceTokenTTL)
	}
	return nil
}

func createServiceUser(ctx context.Context, db dbCommon.Store, username, fullName string) (params.User, error) {
	if !serviceUsernameRx.MatchString(username) {
		return params.User{}, fmt.Errorf("invalid username %q: use %s followed by 1-60 lowercase letters, digits, '.', '_' or '-', starting with a letter or digit", username, serviceUserPrefix)
	}
	email := fmt.Sprintf("%s@%s", username, serviceUserEmailDomain)
	if fullName == "" {
		fullName = username
	}

	// Service users authenticate with issued tokens only. Give them a random
	// password nobody knows so that password login is impossible.
	password, err := util.GetRandomString(48)
	if err != nil {
		return params.User{}, fmt.Errorf("generating password: %w", err)
	}
	hashed, err := util.PaswsordToBcrypt(password)
	if err != nil {
		return params.User{}, fmt.Errorf("hashing password: %w", err)
	}

	user, err := db.CreateUser(ctx, params.NewUserParams{
		Username: username,
		Email:    email,
		FullName: fullName,
		Password: hashed,
		IsAdmin:  false,
		Enabled:  true,
	})
	if err != nil {
		return params.User{}, fmt.Errorf("creating user %q: %w", username, err)
	}
	return user, nil
}

// issueServiceUserToken returns a token for the service user and the time it
// expires at.
func issueServiceUserToken(ctx context.Context, db dbCommon.Store, jwtCfg config.JWTAuth, username string) (string, time.Time, error) {
	user, err := getServiceUser(ctx, db, username)
	if err != nil {
		return "", time.Time{}, err
	}
	if !user.Enabled {
		return "", time.Time{}, fmt.Errorf("user %q is disabled", username)
	}
	// The same claims a login produces, so the API's JWT middleware treats
	// the token exactly like one from /auth/login.
	userCtx := auth.PopulateContext(ctx, user, nil)
	token, err := auth.NewAuthenticator(jwtCfg, db).GetJWTToken(userCtx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issuing token for %q: %w", username, err)
	}
	// Report the expiry the token actually carries, not the one we asked for.
	claims := &auth.JWTClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return "", time.Time{}, fmt.Errorf("reading expiry of the token issued for %q: %w", username, err)
	}
	if claims.ExpiresAt == nil {
		return "", time.Time{}, fmt.Errorf("token issued for %q carries no expiry", username)
	}
	return token, claims.ExpiresAt.Time, nil
}

// revokeServiceUserTokens bumps the user's generation by setting a new random
// password. The JWT middleware rejects tokens whose generation no longer
// matches, so every token issued before this call stops working.
func revokeServiceUserTokens(ctx context.Context, db dbCommon.Store, username string) error {
	if _, err := getServiceUser(ctx, db, username); err != nil {
		return err
	}
	password, err := util.GetRandomString(48)
	if err != nil {
		return fmt.Errorf("generating password: %w", err)
	}
	hashed, err := util.PaswsordToBcrypt(password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	if _, err := db.UpdateUser(ctx, username, params.UpdateUserParams{Password: hashed}); err != nil {
		return fmt.Errorf("revoking tokens for %q: %w", username, err)
	}
	return nil
}

func setServiceUserEnabled(ctx context.Context, db dbCommon.Store, username string, enabled bool) error {
	if _, err := getNonAdminUser(ctx, db, username); err != nil {
		return err
	}
	if _, err := db.UpdateUser(ctx, username, params.UpdateUserParams{Enabled: &enabled}); err != nil {
		return fmt.Errorf("updating user %q: %w", username, err)
	}
	return nil
}

func init() {
	adminUserCreateCmd.Flags().StringVar(&serviceUserFullName, "full-name", "", "display name (default: the username)")
	adminUserTokenCmd.Flags().StringVar(&serviceUserTokenTTL, "ttl", "", "token lifetime, e.g. 720h, between 24h and 8784h (default: jwt_auth.time_to_live)")

	adminUserCmd.AddCommand(adminUserListCmd)
	adminUserCmd.AddCommand(adminUserCreateCmd)
	adminUserCmd.AddCommand(adminUserTokenCmd)
	adminUserCmd.AddCommand(adminUserRevokeTokensCmd)
	adminUserCmd.AddCommand(adminUserDisableCmd)
	adminUserCmd.AddCommand(adminUserEnableCmd)
	adminCmd.AddCommand(adminUserCmd)
}
