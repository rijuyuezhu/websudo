package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	WebAddr                string
	ApprovalTimeoutSeconds int
	TokenHashHex           string
	DatabasePath           string
	RootSocketPath         string
	RootAllowedUID         int
}

func Default() Config {
	cfg := Config{
		WebAddr:                "127.0.0.1:17878",
		ApprovalTimeoutSeconds: 600,
		TokenHashHex:           MustHashToken("123456"),
		DatabasePath:           "./websudo.db",
		RootSocketPath:         "/run/websudo-rootd.sock",
		RootAllowedUID:         os.Getuid(),
	}
	if value, ok := envString("WEBSUDO_WEB_ADDR"); ok {
		cfg.WebAddr = value
	}
	if value, ok := envInt("WEBSUDO_APPROVAL_TIMEOUT_SECONDS"); ok {
		cfg.ApprovalTimeoutSeconds = value
	}
	if value, ok := envString("WEBSUDO_TOKEN_HASH_HEX"); ok {
		cfg.TokenHashHex = value
	}
	if value, ok := envString("WEBSUDO_DATABASE_PATH"); ok {
		cfg.DatabasePath = value
	}
	if value, ok := envString("WEBSUDO_ROOT_SOCKET_PATH"); ok {
		cfg.RootSocketPath = value
	}
	if value, ok := envInt("WEBSUDO_ROOT_ALLOWED_UID"); ok {
		cfg.RootAllowedUID = value
	}
	return cfg
}

func envString(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func envInt(key string) (int, bool) {
	value, ok := envString(key)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func MustHashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func VerifyToken(hashHex, token string) bool {
	decodedHash, err := hex.DecodeString(strings.TrimSpace(hashHex))
	if err != nil {
		return false
	}

	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(decodedHash, sum[:]) == 1
}
