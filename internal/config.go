package internal

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"slices"

	"github.com/score-spec/score-go/types"
	"golang.org/x/mod/module"
)

const ConfigFile = "score.config.json"

type ScoreConfig struct {
	Workloads                []types.Workload         `json:"workloads"`
	DefaultWorkloadComponent ComponentEntry           `json:"default_workload_component"`
	ResourceComponents       []ResourceComponentEntry `json:"resource_components"`
}

type ComponentEntry struct {
	Package         string `json:"package"`
	ConstructorFunc string `json:"constructor_func"`
	ArgsStruct      string `json:"args_struct"`
}

type ResourceComponentEntry struct {
	ComponentEntry
	ResourceType       string `json:"resource_type"`
	ResourceClassRegex string `json:"resource_class_regex"`
	ResourceIdRegex    string `json:"resource_id_regex"`
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

// buildResourceComponentMatcher builds a function that returns true if the entry matches the requested type, class, and id
func buildResourceComponentMatcher(resourceType, resourceClass, resourceId string) func(entry ResourceComponentEntry) bool {
	return func(candidate ResourceComponentEntry) bool {
		if candidate.ResourceType != resourceType {
			return false
		} else if cr, err := regexp.Compile(candidate.ResourceClassRegex); err != nil {
			slog.Error("failed to compile resource class regex", slog.String("pattern", candidate.ResourceClassRegex), slog.String("err", err.Error()))
			return false
		} else if !cr.MatchString(resourceClass) {
			return false
		} else if ir, err := regexp.Compile(candidate.ResourceIdRegex); err != nil {
			slog.Error("failed to compile resource id regex", slog.String("pattern", candidate.ResourceIdRegex), slog.String("err", err.Error()))
			return false
		} else if !ir.MatchString(resourceId) {
			return false
		}
		return true
	}
}

// FindResourceComponent returns the FIRST entry in the library which matches the requested type, class, and id
func FindResourceComponent(library []ResourceComponentEntry, resourceType, resourceClass, resourceId string) (ResourceComponentEntry, bool) {
	if i := slices.IndexFunc(library, buildResourceComponentMatcher(resourceType, resourceClass, resourceId)); i >= 0 {
		return library[i], true
	}
	return ResourceComponentEntry{}, false
}

var (
	validPublicGoIdentifierPattern = regexp.MustCompile(`^[A-Z][a-zA-Z0-9_]*$`)
)

// ValidateComponentEntry returns an error if a component library entry is invalid.
func ValidateComponentEntry(entry ComponentEntry) error {
	if err := module.CheckImportPath(entry.Package); err != nil {
		return fmt.Errorf("component contains an invalid package path '%s'", entry.Package)
	} else if !validPublicGoIdentifierPattern.MatchString(entry.ConstructorFunc) {
		return fmt.Errorf("component contains an invalid constructor func identifier '%s'", entry.ConstructorFunc)
	} else if !validPublicGoIdentifierPattern.MatchString(entry.ArgsStruct) {
		return fmt.Errorf("component contains an invalid args struct identifier '%s'", entry.ArgsStruct)
	}
	return nil
}
