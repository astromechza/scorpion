package internal

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"slices"

	"github.com/score-spec/score-go/types"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

const ConfigFile = "score.config.yaml"

type ScoreConfig struct {
	Workloads                []types.Workload         `yaml:"workloads,omitempty"`
	DefaultWorkloadComponent ComponentEntry           `yaml:"default_workload_component"`
	ResourceComponents       []ResourceComponentEntry `yaml:"resource_components,omitempty"`
}

type ComponentEntry struct {
	Package         string                 `yaml:"package"`
	ConstructorFunc string                 `yaml:"constructor_func"`
	ArgsStruct      string                 `yaml:"args_struct"`
	FixedParams     map[string]interface{} `yaml:"fixed_params,omitempty"`
}

type ResourceComponentEntry struct {
	ComponentEntry     `yaml:",inline"`
	ResourceType       string `yaml:"resource_type"`
	ResourceClassRegex string `yaml:"resource_class_regex"`
	ResourceIdRegex    string `yaml:"resource_id_regex"`
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
		d := yaml.NewDecoder(f)
		d.KnownFields(true)
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
		e := yaml.NewEncoder(f)
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
