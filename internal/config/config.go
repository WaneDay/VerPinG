package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config mirrors config.json (same keys as the original Python version).
type Config struct {
	CheckIntervalMinutes    int                 `json:"check_interval_minutes"`
	LatencyThresholdMs      int                 `json:"latency_threshold_ms"`
	TargetGroup             string              `json:"target_group"`
	PreferSameCountry       bool                `json:"prefer_same_country"`
	ControllerURL           string              `json:"controller_url"`
	Secret                  string              `json:"secret"`
	TestURL                 string              `json:"test_url"`
	DelayTimeoutMs          int                 `json:"delay_timeout_ms"`
	HTTPTimeoutMs           int                 `json:"http_timeout_ms"`
	MaxConcurrentDelayTests int                 `json:"max_concurrent_delay_tests"`
	CoreReadyRetries        int                 `json:"core_ready_retries"`
	CoreReadyIntervalMs     int                 `json:"core_ready_interval_ms"`
	TestMode                bool                `json:"test_mode"`
	FallbackProfiles        []string            `json:"fallback_profiles"`
	CountryAliases          map[string][]string `json:"country_aliases"`
}

// Default returns the built-in default configuration.
func Default() Config {
	return Config{
		CheckIntervalMinutes:    5,
		LatencyThresholdMs:      220,
		TargetGroup:             "自动选择",
		PreferSameCountry:       true,
		ControllerURL:           "http://127.0.0.1:9090",
		Secret:                  "set-your-secret",
		TestURL:                 "https://cp.cloudflare.com/generate_204",
		DelayTimeoutMs:          2000,
		HTTPTimeoutMs:           5000,
		MaxConcurrentDelayTests: 8,
		CoreReadyRetries:        7,
		CoreReadyIntervalMs:     1800,
		TestMode:                true,
		FallbackProfiles:        []string{},
		CountryAliases:          DefaultAliases(),
	}
}

// DefaultAliases is the built-in country keyword table.
func DefaultAliases() map[string][]string {
	return map[string][]string{
		"hk": {"hk", "hkg", "hongkong", "香港"},
		"tw": {"tw", "twn", "taiwan", "台湾", "台北"},
		"sg": {"sg", "sgp", "singapore", "新加坡", "狮城"},
		"jp": {"jp", "jpn", "japan", "东京", "大阪", "日本"},
		"kr": {"kr", "kor", "korea", "首尔", "韩国"},
		"us": {"us", "usa", "unitedstates", "america", "洛杉矶", "硅谷", "纽约", "美国"},
		"ca": {"ca", "can", "canada", "加拿大"},
		"uk": {"uk", "gb", "gbr", "england", "london", "英国"},
		"au": {"au", "aus", "australia", "悉尼", "墨尔本", "澳洲", "澳大利亚"},
		"de": {"de", "ger", "germany", "德国", "法兰克福"},
		"fr": {"fr", "fra", "france", "法国", "巴黎"},
		"ru": {"ru", "rus", "russia", "俄罗斯"},
		"th": {"th", "tha", "thailand", "泰国"},
		"vn": {"vn", "vnm", "vietnam", "越南"},
		"my": {"my", "mys", "malaysia", "马来西亚"},
		"id": {"id", "idn", "indonesia", "印尼"},
	}
}

func applyDefaults(c *Config) {
	if c.CheckIntervalMinutes < 1 {
		c.CheckIntervalMinutes = 5
	}
	if c.LatencyThresholdMs <= 0 {
		c.LatencyThresholdMs = 220
	}
	if c.TargetGroup == "" {
		c.TargetGroup = "自动选择"
	}
	if c.ControllerURL == "" {
		c.ControllerURL = "http://127.0.0.1:9090"
	}
	if c.TestURL == "" {
		c.TestURL = "https://cp.cloudflare.com/generate_204"
	}
	if c.DelayTimeoutMs <= 0 {
		c.DelayTimeoutMs = 2000
	}
	if c.HTTPTimeoutMs <= 0 {
		c.HTTPTimeoutMs = 5000
	}
	if c.MaxConcurrentDelayTests < 1 {
		c.MaxConcurrentDelayTests = 8
	}
	if c.CoreReadyRetries <= 0 {
		c.CoreReadyRetries = 7
	}
	if c.CoreReadyIntervalMs <= 0 {
		c.CoreReadyIntervalMs = 1800
	}
	if c.CountryAliases == nil || len(c.CountryAliases) == 0 {
		c.CountryAliases = DefaultAliases()
	}
	if c.FallbackProfiles == nil {
		c.FallbackProfiles = []string{}
	}
}

// DataDir returns the folder holding config.json and logs/.
// Prefers the executable directory (portable); falls back to %APPDATA% when
// the executable directory is not writable (e.g. installed under Program Files).
func DataDir(exeDir string) string {
	if exeDir != "" {
		probe := filepath.Join(exeDir, ".write_test")
		if f, err := os.Create(probe); err == nil {
			f.Close()
			_ = os.Remove(probe)
			return exeDir
		}
	}
	d := filepath.Join(os.Getenv("APPDATA"), "VerPinG")
	_ = os.MkdirAll(d, 0755)
	return d
}

// Load reads config.json from dir, creating a default file when missing.
func Load(dir string) (Config, error) {
	path := filepath.Join(dir, "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		c := Default()
		_ = Save(dir, c)
		return c, nil
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), err
	}
	applyDefaults(&c)
	return c, nil
}

// Save writes config.json into dir.
func Save(dir string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), b, 0644)
}
