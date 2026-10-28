package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Addr          string `yaml:"addr"`
	DataDir       string `yaml:"data_dir"`
	JWTDays       int    `yaml:"jwt_days"`
	UploadMB      int64  `yaml:"upload_mb"`
	LogLevel      string `yaml:"log_level"`
	MasterSecret  string `yaml:"-"`
	AdminPassword string `yaml:"-"`
}

func Default() Config {
	return Config{
		Addr:     ":8080",
		DataDir:  "./data",
		JWTDays:  30,
		UploadMB: 20,
		LogLevel: "info",
	}
}

func Load(args []string) (Config, error) {
	cfg := Default()

	var (
		addr, dataDir, logLevel, configPath string
		jwtDays, uploadMB                   int64
	)
	jwtDays = -1
	uploadMB = -1

	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.StringVar(&addr, "addr", "", "listen address, e.g. :8080")
	fs.StringVar(&dataDir, "data-dir", "", "data directory")
	fs.StringVar(&logLevel, "log-level", "", "debug|info|warn|error")
	fs.StringVar(&configPath, "config", "", "config yaml path")
	fs.Int64Var(&jwtDays, "jwt-days", -1, "jwt validity in days")
	fs.Int64Var(&uploadMB, "upload-mb", -1, "max upload size in MB")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	path := configPath
	if path == "" {
		path = os.Getenv("ONLINETEST_CONFIG")
	}
	if path == "" {
		path = "./config.yaml"
	}
	if b, err := os.ReadFile(path); err == nil {
		var fromFile Config
		if err := yaml.Unmarshal(b, &fromFile); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
		if fromFile.Addr != "" {
			cfg.Addr = fromFile.Addr
		}
		if fromFile.DataDir != "" {
			cfg.DataDir = fromFile.DataDir
		}
		if fromFile.JWTDays > 0 {
			cfg.JWTDays = fromFile.JWTDays
		}
		if fromFile.UploadMB > 0 {
			cfg.UploadMB = fromFile.UploadMB
		}
		if fromFile.LogLevel != "" {
			cfg.LogLevel = fromFile.LogLevel
		}
	} else if path != "./config.yaml" && !os.IsNotExist(err) {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	if v := os.Getenv("ONLINETEST_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("ONLINETEST_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("ONLINETEST_JWT_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.JWTDays = n
		}
	}
	if v := os.Getenv("ONLINETEST_UPLOAD_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.UploadMB = n
		}
	}
	if v := os.Getenv("ONLINETEST_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	cfg.MasterSecret = os.Getenv("ONLINETEST_SECRET")
	cfg.AdminPassword = os.Getenv("ONLINETEST_ADMIN_PASSWORD")

	if set["addr"] {
		cfg.Addr = addr
	}
	if set["data-dir"] {
		cfg.DataDir = dataDir
	}
	if set["log-level"] {
		cfg.LogLevel = logLevel
	}
	if set["jwt-days"] && jwtDays > 0 {
		cfg.JWTDays = int(jwtDays)
	}
	if set["upload-mb"] && uploadMB > 0 {
		cfg.UploadMB = uploadMB
	}

	cfg.DataDir = filepath.Clean(cfg.DataDir)
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("addr is required")
	}
	if c.JWTDays < 1 || c.JWTDays > 3650 {
		return fmt.Errorf("jwt-days must be between 1 and 3650")
	}
	if c.UploadMB < 1 || c.UploadMB > 200 {
		return fmt.Errorf("upload-mb must be between 1 and 200")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level %q", c.LogLevel)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data-dir is required")
	}
	return nil
}
