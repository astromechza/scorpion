package main

import (
	"cmp"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/score-spec/score-go/loader"
	"github.com/score-spec/score-go/schema"
	"github.com/score-spec/score-go/types"
	"gopkg.in/yaml.v3"
)

func requireNArgs(n int, c int) bool {
	n -= c
	if cmp.Compare(flag.NArg(), n) != c {
		flag.Usage()
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
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}
	}
	if requireNArgs(1, 1) {
		var err error
		if subcommand := flag.Arg(0); subcommand == "init" && requireNArgs(1, 0) {
			err = scoreInit()
		} else if subcommand == "generate" && requireNArgs(2, -1) {
			err = scoreGenerate(flag.Arg(1))
		} else {
			err = fmt.Errorf("unknown subcommand: '%s'", subcommand)
		}
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}
	}
}

func scoreInit() error {
	if _, ok, err := LoadConfig(); err != nil {
		return err
	} else if !ok {
		if err := SaveConfig(ScoreConfig{Workloads: make([]types.Workload, 0)}); err != nil {
			return err
		}
	}
	return nil
}

func scoreGenerate(fileName string) error {
	var cfg ScoreConfig
	var err error
	if fileName == "" {
		if cfg, _, err = LoadConfig(); err != nil {
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

		if cfg, _, err = LoadConfig(); err != nil {
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

	c, err := cfg.GenerateComponentGraph()
	if err != nil {
		return err
	}

	f, err := BuildJenFile(c)
	if err != nil {
		return err
	}
	if err := f.Render(os.Stdout); err != nil {
		return err
	}
	
	if err := SaveConfig(cfg); err != nil {
		return err
	}
	return nil
}
