package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Principal struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"sub"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
}
type contextKey string

const tokenKey contextKey = "knowledge-token"

func Verify(secret, token string) (Principal, error) {
	parts := strings.Split(token, ".")
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	if len(secret) < 32 || len(parts) != 3 || parts[0] != header {
		return Principal{}, errors.New("令牌格式无效")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Principal{}, err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return Principal{}, errors.New("签名无效")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, err
	}
	var p Principal
	if json.Unmarshal(body, &p) != nil || p.TenantID == "" || p.UserID == "" || p.ExpiresAt <= time.Now().Unix() {
		return Principal{}, errors.New("身份已失效")
	}
	return p, nil
}
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey, token)
}
func Token(ctx context.Context) string { v, _ := ctx.Value(tokenKey).(string); return v }
