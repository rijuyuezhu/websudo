package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

type Config struct {
	WebAddr                string
	ApprovalTimeoutSeconds int
	TokenHashHex           string
	DatabasePath           string
	RootSocketPath         string
}

func Default() Config {
	return Config{
		WebAddr:                "127.0.0.1:17878",
		ApprovalTimeoutSeconds: 600,
		DatabasePath:           "./websudo.db",
		RootSocketPath:         "/run/websudo-rootd.sock",
	}
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
