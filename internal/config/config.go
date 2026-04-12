package config

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"os"
	"path/filepath"
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
	fileEnv := readEnvironmentFile(defaultEnvFilePath())

	rootSocketPath := defaultRootSocketPath()
	cfg := Config{
		WebAddr:                "127.0.0.1:17878",
		ApprovalTimeoutSeconds: 600,
		TokenHashHex:           MustHashToken("123456"),
		DatabasePath:           defaultDatabasePath(),
		RootSocketPath:         rootSocketPath,
		RootAllowedUID:         defaultRootAllowedUID(rootSocketPath),
	}
	if value, ok := envString(fileEnv, "WEBSUDO_WEB_ADDR"); ok {
		cfg.WebAddr = value
	}
	if value, ok := envInt(fileEnv, "WEBSUDO_APPROVAL_TIMEOUT_SECONDS"); ok {
		cfg.ApprovalTimeoutSeconds = value
	}
	if value, ok := envString(fileEnv, "WEBSUDO_TOKEN_HASH_HEX"); ok {
		cfg.TokenHashHex = value
	}
	if value, ok := envString(fileEnv, "WEBSUDO_DATABASE_PATH"); ok {
		cfg.DatabasePath = value
	}
	if value, ok := envString(fileEnv, "WEBSUDO_ROOT_SOCKET_PATH"); ok {
		cfg.RootSocketPath = value
		cfg.RootAllowedUID = defaultRootAllowedUID(value)
	}
	if value, ok := envInt(fileEnv, "WEBSUDO_ROOT_ALLOWED_UID"); ok {
		cfg.RootAllowedUID = value
	}
	return cfg
}

func defaultEnvFilePath() string {
	if value, ok := envString(nil, "WEBSUDO_ENV_FILE"); ok {
		return value
	}
	return "/etc/websudo/websudo.env"
}

func defaultDatabasePath() string {
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".websudo", "websudo.db")
	}
	return filepath.Join(".", ".websudo", "websudo.db")
}

func defaultRootSocketPath() string {
	if runtimeDir, ok := envString(nil, "XDG_RUNTIME_DIR"); ok {
		return filepath.Join(runtimeDir, "websudo-rootd.sock")
	}
	return "/run/websudo-rootd.sock"
}

func defaultRootAllowedUID(rootSocketPath string) int {
	parts := strings.Split(filepath.Clean(rootSocketPath), string(filepath.Separator))
	for index := 0; index+1 < len(parts); index++ {
		if parts[index] != "user" {
			continue
		}
		uid, err := strconv.Atoi(parts[index+1])
		if err == nil {
			return uid
		}
	}
	return os.Getuid()
}

func readEnvironmentFile(path string) map[string]string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		values[key] = value
	}
	return values
}

func envString(fileEnv map[string]string, key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if (!ok || strings.TrimSpace(value) == "") && fileEnv != nil {
		value, ok = fileEnv[key]
	}
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func envInt(fileEnv map[string]string, key string) (int, bool) {
	value, ok := envString(fileEnv, key)
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
