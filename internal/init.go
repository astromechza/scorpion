package internal

import (
	"fmt"
	"regexp"
)

var (
	workloadProfileRegex     = regexp.MustCompile(`^(.+)\.([^(.]+)\(([^)]+)\)$`)
	supportedWorkloadModules = map[string]string{
		"debug": "github.com/astromechza/scorpion/lib/debug.New(Args)",
	}
)

func parseWorkloadProfileOneLiner(raw string) (ComponentEntry, bool) {
	if m := workloadProfileRegex.FindStringSubmatch(raw); m != nil {
		return ComponentEntry{
			Package:         m[1],
			ConstructorFunc: m[2],
			ArgsStruct:      m[3],
		}, true
	}
	return ComponentEntry{}, false
}

func BuildWorkloadComponentForProfile(raw string) (ComponentEntry, error) {
	if k, ok := supportedWorkloadModules[raw]; ok {
		raw = k
	}
	if e, ok := parseWorkloadProfileOneLiner(raw); !ok {
		return e, fmt.Errorf("failed to parse '%s' as workload profile", raw)
	} else {
		return e, nil
	}
}
