package orchestrator

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds orchestration settings loaded from .ralphex/orchestrate.yml.
type Config struct {
	Source      string       `yaml:"source"`       // "plans" or "issues"
	PlansDir   string       `yaml:"plans_dir"`     // path to plans directory
	Issues     IssuesConfig `yaml:"issues"`        // GitHub issues config
	MaxParallel int         `yaml:"max_parallel"`
	MaxRetries  int         `yaml:"max_retries"`
	RetryDelay  Duration    `yaml:"retry_delay"`
	FailFast    bool        `yaml:"fail_fast"`
}

// IssuesConfig holds GitHub issues settings.
type IssuesConfig struct {
	Label    string `yaml:"label"`
	AutoPlan bool   `yaml:"auto_plan"`
	Repo     string `yaml:"repo"`
}

// Duration wraps time.Duration for YAML unmarshalling.
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses duration strings like "30s", "5m", "1h".
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// LoadConfig reads orchestration config from a YAML file.
// returns zero-value Config (with defaults applied) if file doesn't exist.
func LoadConfig(path string) (Config, error) {
	cfg := Config{
		Source:      "plans",
		PlansDir:    "docs/plans",
		MaxParallel: 4,
		MaxRetries:  2,
		RetryDelay:  Duration{30 * time.Second},
		FailFast:    false,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}
