package middleware

import (
	"errors"
	"maps"
	"strings"
	"time"

	"github.com/goichi-dev/goichi/core"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims is the decoded payload of a JSON Web Token.
type JWTClaims map[string]any

// JWTConfig configures HS256 token verification.
type JWTConfig struct {
	// Secret is the HMAC signing key. Verification is disabled while it is empty.
	Secret string

	// TokenLookup names where to read the token, as "source:key" — one of
	// "header:Authorization", "query:token" or "cookie:session". Defaults to
	// "header:Authorization".
	TokenLookup  string
	ErrorHandler func(c *core.Context, reason string) error
}

// JWT rejects requests without a valid token and stores the claims on the
// context, where Context.JWT reads them.
func JWT(cfg JWTConfig) core.Middleware {
	if cfg.TokenLookup == "" {
		cfg.TokenLookup = "header:Authorization"
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(c *core.Context, reason string) error {
			c.RequestCtx.SetStatusCode(core.StatusUnauthorized)
			return core.WriteJSONError(c.RequestCtx, reason)
		}
	}

	if cfg.Secret == "" {
		return func(next core.Handler) core.Handler {
			return func(c *core.Context) error {
				return cfg.ErrorHandler(c, "server misconfiguration: missing JWT secret")
			}
		}
	}

	// A TokenLookup without a "source:key" separator is a configuration error;
	// fall back to the default rather than panicking at request time.
	if !strings.Contains(cfg.TokenLookup, ":") {
		cfg.TokenLookup = "header:Authorization"
	}

	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			raw := ExtractToken(c, cfg.TokenLookup)

			if raw == "" {
				return cfg.ErrorHandler(c, "missing or empty token")
			}

			claimsMap, errMsg := ValidateJWT(raw, cfg.Secret)
			if errMsg != "" {
				return cfg.ErrorHandler(c, errMsg)
			}
			c.SetJWT(claimsMap)
			return next(c)
		}
	}
}

// JWTEncode signs whatever claims the handler left on the context and returns
// them in the Authorization response header.
func JWTEncode(cfg JWTConfig) core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			if err := next(c); err != nil {
				return err
			}
			claims := c.JWT()
			if claims == nil {
				return nil
			}
			s, err := EncodeJWT(claims, cfg.Secret)
			if err != nil {
				return err
			}
			c.RequestCtx.Response.Header.Set("Authorization", "Bearer "+s)
			return nil
		}
	}
}

// ValidateJWT parses and validates an HMAC-signed token, returning the claims
// or a non-empty error message. The signing method is pinned to HMAC (rejecting
// "alg":"none" and asymmetric-key confusion attacks) and a valid expiration is
// required. This is the single source of truth used by every protocol.
func ValidateJWT(raw, secret string) (JWTClaims, string) {
	if secret == "" {
		return nil, "server misconfiguration: missing JWT secret"
	}
	if raw == "" {
		return nil, "missing or empty token"
	}
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}), jwt.WithExpirationRequired())
	if err != nil || !tok.Valid {
		return nil, "invalid token"
	}
	claimsMap := make(JWTClaims)
	if mc, ok := tok.Claims.(jwt.MapClaims); ok {
		maps.Copy(claimsMap, mc)
	}
	return claimsMap, ""
}

// ExtractToken pulls a raw token from a request following a "source:key" lookup
// spec (e.g. "header:Authorization" or "query:token"). The "Bearer " prefix is
// stripped for header lookups.
func ExtractToken(c *core.Context, lookup string) string {
	if lookup == "" {
		lookup = "header:Authorization"
	}
	parts := strings.SplitN(lookup, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	switch parts[0] {
	case "header":
		val := string(c.RequestCtx.Request.Header.Peek(parts[1]))
		return strings.TrimSpace(strings.TrimPrefix(val, "Bearer "))
	case "query":
		return string(c.RequestCtx.QueryArgs().Peek(parts[1]))
	case "cookie":
		return string(c.RequestCtx.Request.Header.Cookie(parts[1]))
	}
	return ""
}

// DefaultTokenTTL is the lifetime given to tokens minted by EncodeJWT when the
// claims carry no "exp" of their own. ValidateJWT rejects tokens without an
// expiry, so a default is required for the two to round-trip.
const DefaultTokenTTL = time.Hour

// EncodeJWT signs claims with HS256. An "exp" claim is added when absent so the
// result passes ValidateJWT; supply your own "exp" to override the lifetime.
func EncodeJWT(claims JWTClaims, secret string) (string, error) {
	return EncodeJWTWithTTL(claims, secret, DefaultTokenTTL)
}

// EncodeJWTWithTTL signs claims with HS256, expiring after ttl. A non-positive
// ttl falls back to DefaultTokenTTL. An "exp" already present in claims wins.
func EncodeJWTWithTTL(claims JWTClaims, secret string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("jwt: missing secret")
	}
	if ttl <= 0 {
		ttl = DefaultTokenTTL
	}

	mc := jwt.MapClaims{}
	maps.Copy(mc, claims)
	now := time.Now()
	if _, ok := mc["exp"]; !ok {
		mc["exp"] = jwt.NewNumericDate(now.Add(ttl))
	}
	if _, ok := mc["iat"]; !ok {
		mc["iat"] = jwt.NewNumericDate(now)
	}

	t := jwt.NewWithClaims(jwt.SigningMethodHS256, mc)
	return t.SignedString([]byte(secret))
}

// DecodeJWT validates tokenStr and returns its claims, or a non-empty message
// describing why it was rejected.
func DecodeJWT(tokenStr, secret string) (JWTClaims, string) {
	return ValidateJWT(tokenStr, secret)
}
