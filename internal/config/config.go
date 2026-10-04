package config

import (
	"bufio"
	"errors"
	"fmt"
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
	if value, ok := configString(values, "WEBSUDO_WEB_ADDR"); ok {
		cfg.WebAddr = value
	}
	if value, ok := values["WEBSUDO_APPROVAL_TIMEOUT_SECONDS"]; ok {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds <= 0 || seconds > int64((1<<63-1)/time.Second) {
			return Config{}, fmt.Errorf("WEBSUDO_APPROVAL_TIMEOUT_SECONDS must be a positive integer that fits a duration: %q", value)
		}
		cfg.ApprovalTimeout = time.Duration(seconds) * time.Second
	}
	if value, ok := configString(values, "WEBSUDO_SUDO_PATH"); ok {
		if !filepath.IsAbs(value) {
			return Config{}, fmt.Errorf("WEBSUDO_SUDO_PATH must be an absolute path: %q", value)
		}
		cfg.SudoPath = value
	}
	return cfg, nil
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
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read websudo config %s: %w", path, err)
	}
	return values, nil
}

func configString(values map[string]string, key string) (string, bool) {
	value, ok := values[key]
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}
