package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/models"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

var (
	OIDCProvider *oidc.Provider
	OAuth2Config *oauth2.Config
)

// IDToken is an interface for ID tokens (both standard OIDC and HS256)
type IDToken interface {
	Claims(v interface{}) error
}

// InitializeOIDC sets up the OIDC provider and OAuth2 config
func InitializeOIDC(ctx context.Context) error {
	if config.AppConfig.OIDCProviderURL == "" {
		return fmt.Errorf("OIDC provider URL not configured")
	}

	var err error
	OIDCProvider, err = oidc.NewProvider(ctx, config.AppConfig.OIDCProviderURL)
	if err != nil {
		return fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	OAuth2Config = &oauth2.Config{
		ClientID:     config.AppConfig.OIDCClientID,
		ClientSecret: config.AppConfig.OIDCClientSecret,
		RedirectURL:  config.AppConfig.OIDCRedirectURL,
		Endpoint:     OIDCProvider.Endpoint(),
		Scopes:       config.AppConfig.OIDCScopes,
	}

	return nil
}

// GenerateStateToken creates a random state token for OIDC flow
func GenerateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GetAuthCodeURL generates the authorization URL for OIDC
func GetAuthCodeURL(state string) string {
	return OAuth2Config.AuthCodeURL(state)
}

// ExchangeCodeForToken exchanges authorization code for token
func ExchangeCodeForToken(ctx context.Context, code string) (*oauth2.Token, error) {
	return OAuth2Config.Exchange(ctx, code)
}

// VerifyIDToken verifies the ID token from OIDC provider. The verification
// path is never chosen from the (untrusted) token header: the standard OIDC
// verifier runs first, and the HS256 fallback is attempted only when it fails
// and OIDC_ALLOW_HS256 is enabled.
func VerifyIDToken(ctx context.Context, rawIDToken string) (IDToken, error) {
	// Standard OIDC verification (signature via JWKS, audience, issuer, expiry).
	verifyErr := fmt.Errorf("OIDC provider not initialized")
	if OIDCProvider != nil {
		verifier := OIDCProvider.Verifier(&oidc.Config{
			ClientID: config.AppConfig.OIDCClientID,
		})
		idToken, err := verifier.Verify(ctx, rawIDToken)
		if err == nil {
			return idToken, nil
		}
		verifyErr = err
	}
	if !config.AppConfig.OIDCAllowHS256 {
		return nil, fmt.Errorf("ID token verification failed: %w", verifyErr)
	}

	return verifyHS256(rawIDToken, discoveredIssuer())
}

// discoveredIssuer returns the issuer from the provider's discovery document,
// falling back to the configured provider URL when discovery is unavailable.
// This keeps the HS256 issuer check aligned with what the provider actually
// stamps into tokens (e.g. trailing-slash differences from the config value).
func discoveredIssuer() string {
	if OIDCProvider != nil {
		var claims struct {
			Issuer string `json:"issuer"`
		}
		if err := OIDCProvider.Claims(&claims); err == nil && claims.Issuer != "" {
			return claims.Issuer
		}
	}
	return config.AppConfig.OIDCProviderURL
}

// verifyHS256 verifies an HMAC-signed ID token with the client secret. The
// algorithm is pinned to HS256 in the keyfunc (and via WithValidMethods), and
// audience, issuer, and expiry are validated to match the OIDC path.
func verifyHS256(rawIDToken, issuer string) (IDToken, error) {
	parsedToken, err := jwt.Parse(
		rawIDToken,
		func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != "HS256" {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
			}
			return []byte(config.AppConfig.OIDCClientSecret), nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithAudience(config.AppConfig.OIDCClientID),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsedToken.Valid {
		return nil, fmt.Errorf("HS256 token verification failed: %w", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to get claims from token")
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal claims: %w", err)
	}

	return &mockIDToken{claims: claimsJSON}, nil
}

// mockIDToken implements IDToken interface for HS256 tokens
type mockIDToken struct {
	claims json.RawMessage
}

// Claims unmarshals the claims into the provided structure
func (m *mockIDToken) Claims(v interface{}) error {
	return json.Unmarshal(m.claims, v)
}

// IDTokenClaims represents the claims from an ID token
type IDTokenClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}

// GetOrCreateUser retrieves or creates a user based on OIDC claims
func GetOrCreateUser(ctx context.Context, idToken IDToken) (*models.User, error) {
	var claims IDTokenClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Try to find existing user by OIDC subject
	var user models.User
	err := database.DB.QueryRow(
		`SELECT id, username, role, oidc_subject, email, created_at, last_login
		 FROM users WHERE oidc_subject = ?`,
		claims.Subject,
	).Scan(&user.ID, &user.Username, &user.Role, &user.OIDCSubject, &user.Email, &user.CreatedAt, &user.LastLogin)

	if err == nil {
		// User exists, update last login
		now := time.Now()
		_, err = database.DB.Exec(
			`UPDATE users SET last_login = ? WHERE id = ?`,
			now, user.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update last login: %w", err)
		}
		user.LastLogin = &now
		return &user, nil
	}

	// User doesn't exist, create new one
	username := claims.Email
	if username == "" {
		username = claims.Name
	}
	if username == "" {
		username = claims.Subject
	}

	// Determine role based on admin email list
	role := string(models.RoleUser)
	if config.AppConfig.IsAdminEmail(claims.Email) {
		role = string(models.RoleAdmin)
	}

	result, err := database.DB.Exec(
		`INSERT INTO users (username, oidc_subject, email, role) VALUES (?, ?, ?, ?)`,
		username, claims.Subject, claims.Email, role,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	// Fetch the newly created user
	err = database.DB.QueryRow(
		`SELECT id, username, role, oidc_subject, email, created_at, last_login
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&user.ID, &user.Username, &user.Role, &user.OIDCSubject, &user.Email, &user.CreatedAt, &user.LastLogin)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch new user: %w", err)
	}

	return &user, nil
}
