package main

import (
	"cmp"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/astromechza/score-pulumi/internal"

	"github.com/score-spec/score-go/loader"
	"github.com/score-spec/score-go/schema"
	"github.com/score-spec/score-go/types"
	"gopkg.in/yaml.v3"
)

var cmpDesc = map[int]string{
	-1: "less than",
	0:  "exactly",
	1:  "more than",
}

func requireNArgs(n int, c int) bool {
	n -= c
	if cmp.Compare(flag.NArg(), n) != c {
		_, _ = os.Stderr.WriteString(fmt.Sprintf("expected %s %d arguments but got %d\n", cmpDesc[c], n, flag.NArg()))
		os.Exit(2)
		return false
	}
	return true
}

func init() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, `Usage: scorpion [subcommand] [options]

Basic commands:
  init				initialise a new scorpion project directory
  generate			add or update a Score workload in the project and regenerate the output code
`)
		flag.PrintDefaults()
	}
}

func main() {
	dirFlag := flag.String("dir", "", "change to this directory before doing anything else")
	flag.Parse()

	if *dirFlag != "" {
		if err := os.Chdir(*dirFlag); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
	}

	if requireNArgs(1, 1) {
		var err error
		if subcommand := flag.Arg(0); subcommand == "init" && requireNArgs(2, 0) {
			err = scoreInit(flag.Arg(1))
		} else if subcommand == "generate" && requireNArgs(2, -1) {
			err = scoreGenerate(flag.Arg(1))
		} else {
			err = fmt.Errorf("unknown subcommand: '%s'", subcommand)
		}
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
	}
}

func scoreInit(profile string) error {
	if cfg, ok, err := internal.LoadConfig(); err != nil {
		return err
	} else if !ok {
		profile, err := internal.BuildWorkloadComponentForProfile(profile)
		if err != nil {
			return err
		}
		if err := internal.SaveConfig(internal.ScoreConfig{
			Workloads:                make([]types.Workload, 0),
			DefaultWorkloadComponent: profile,
		}); err != nil {
			return err
		}
	} else {
		profile, err := internal.BuildWorkloadComponentForProfile(profile)
		if err != nil {
			return err
		}
		if profile != cfg.DefaultWorkloadComponent {
			cfg.DefaultWorkloadComponent = profile
			if err := internal.SaveConfig(cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func scoreGenerate(fileName string) error {
	var cfg internal.ScoreConfig
	var err error
	if fileName == "" {
		if cfg, _, err = internal.LoadConfig(); err != nil {
			return err
		}
	} else if f, err := os.Open(fileName); err != nil {
		return err
	} else {
		defer func() {
			_ = f.Close()
		}()
		var srcMap map[string]interface{}
		if err := yaml.NewDecoder(f).Decode(&srcMap); err != nil {
			return err
		}
		if err := schema.Validate(srcMap); err != nil {
			return err
		}
		var spec types.Workload
		if err := loader.MapSpec(&spec, srcMap); err != nil {
			return err
		} else if err := loader.Normalize(&spec, "."); err != nil {
			return err
		}

		if cfg, _, err = internal.LoadConfig(); err != nil {
			return err
		}
		if i := slices.IndexFunc(cfg.Workloads, func(other types.Workload) bool {
			return other.Metadata["name"].(string) == spec.Metadata["name"].(string)
		}); i >= 0 {
			cfg.Workloads[i] = spec
		} else {
			cfg.Workloads = append(cfg.Workloads, spec)
		}
	}

	if err := internal.ValidateComponentEntry(cfg.DefaultWorkloadComponent); err != nil {
		return fmt.Errorf("config contains an invalid default workload component spec: %v", err)
	} else {
		for i, entry := range cfg.ResourceComponents {
			if err := internal.ValidateComponentEntry(entry.ComponentEntry); err != nil {
				return fmt.Errorf("config contains an invalid resource component spec (%d): %v", i, err)
			}
		}
	}

	c, err := cfg.GenerateComponentGraph()
	if err != nil {
		return err
	}

	f, err := internal.BuildJenFile(c)
	if err != nil {
		return err
	}
	if err := f.Render(os.Stdout); err != nil {
		return err
	}

	if err := internal.SaveConfig(cfg); err != nil {
		return err
	}
	return nil
}
