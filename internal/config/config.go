package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	WebAddr         string
	ApprovalTimeout time.Duration
	SudoPath        string
}

const (
	filePath        = "/etc/websudo/websudo.env"
	DefaultSudoPath = "/usr/bin/sudo"
)

func Load() (Config, error) {
	return load(filePath)
}

func load(path string) (Config, error) {
	values, err := readConfigFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		WebAddr:         "127.0.0.1:17878",
		ApprovalTimeout: 10 * time.Minute,
		SudoPath:        DefaultSudoPath,
	}
	if value, ok := values["WEBSUDO_WEB_ADDR"]; ok {
		if err := validateWebAddr(value); err != nil {
			return Config{}, err
		}
		cfg.WebAddr = value
	}
	if value, ok := values["WEBSUDO_APPROVAL_TIMEOUT_SECONDS"]; ok {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds <= 0 || seconds > int64((1<<63-1)/time.Second) {
			return Config{}, fmt.Errorf("WEBSUDO_APPROVAL_TIMEOUT_SECONDS must be a positive integer that fits a duration: %q", value)
		}
		cfg.ApprovalTimeout = time.Duration(seconds) * time.Second
	}
	if value, ok := values["WEBSUDO_SUDO_PATH"]; ok {
		if !filepath.IsAbs(value) {
			return Config{}, fmt.Errorf("WEBSUDO_SUDO_PATH must be an absolute path: %q", value)
		}
		cfg.SudoPath = value
	}
	return cfg, nil
}

func validateWebAddr(value string) error {
	addr, err := netip.ParseAddrPort(value)
	if err != nil || !addr.Addr().IsLoopback() || addr.Port() == 0 {
		return fmt.Errorf("WEBSUDO_WEB_ADDR must be an explicit loopback IP with a non-zero port: %q", value)
	}
	return nil
}

func AskpassSocketPath() string {
	return filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "websudo", "askpass.sock")
}

func readConfigFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open websudo config %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid websudo config line %d: expected KEY=VALUE", lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !knownConfigKey(key) {
			return nil, fmt.Errorf("unknown websudo config key on line %d: %q", lineNumber, key)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read websudo config %s: %w", path, err)
	}
	return values, nil
}

func knownConfigKey(key string) bool {
	switch key {
	case "WEBSUDO_WEB_ADDR", "WEBSUDO_APPROVAL_TIMEOUT_SECONDS", "WEBSUDO_SUDO_PATH":
		return true
	default:
		return false
	}
}
