package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	WebAddr                string
	ApprovalTimeoutSeconds int
	SudoPath               string
	AskpassPath            string
}

func Default() Config {
	fileEnv := readEnvironmentFile(defaultEnvFilePath())

	cfg := Config{
		WebAddr:                "127.0.0.1:17878",
		ApprovalTimeoutSeconds: 600,
		SudoPath:               "/usr/bin/sudo",
		AskpassPath:            "",
	}
	if value, ok := envString(fileEnv, "WEBSUDO_WEB_ADDR"); ok {
		cfg.WebAddr = value
	}
	if value, ok := envInt(fileEnv, "WEBSUDO_APPROVAL_TIMEOUT_SECONDS"); ok {
		cfg.ApprovalTimeoutSeconds = value
	}
	if value, ok := envString(fileEnv, "WEBSUDO_SUDO_PATH"); ok {
		cfg.SudoPath = value
	}
	if value, ok := envString(fileEnv, "WEBSUDO_ASKPASS_PATH"); ok {
		cfg.AskpassPath = value
	}
	return cfg
}

func defaultEnvFilePath() string {
	if value, ok := envString(nil, "WEBSUDO_ENV_FILE"); ok {
		return value
	}
	return "/etc/websudo/websudo.env"
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
