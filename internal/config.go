package internal

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/score-spec/score-go/types"
)

const ConfigFile = "config.json"

type ScoreConfig struct {
	Workloads []types.Workload `json:"workloads,omitzero"`
}

func LoadConfig() (ScoreConfig, bool, error) {
	if f, err := os.Open(ConfigFile); err != nil {
		if os.IsNotExist(err) {
			return ScoreConfig{}, false, nil
		}
		return ScoreConfig{}, false, fmt.Errorf("failed to open config file: %w", err)
	} else {
		defer func() {
			_ = f.Close()
		}()
		var cfg ScoreConfig
		d := json.NewDecoder(f)
		d.DisallowUnknownFields()
		if err := d.Decode(&cfg); err != nil {
			return ScoreConfig{}, false, fmt.Errorf("failed to decode config file: %w", err)
		}
		return cfg, true, nil
	}
}

func SaveConfig(cfg ScoreConfig) error {
	tempName := ConfigFile + ".tmp"
	defer func() {
		_ = os.Remove(tempName)
	}()
	if f, err := os.Create(tempName); err != nil {
		return fmt.Errorf("failed to create temporary config file: %w", err)
	} else {
		defer func() {
			_ = f.Close()
		}()
		e := json.NewEncoder(f)
		e.SetIndent("", "  ")
		if err := e.Encode(cfg); err != nil {
			return fmt.Errorf("failed to encode config file: %w", err)
		}
		return os.Rename(tempName, ConfigFile)
	}
}
