package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/dave/jennifer/jen"
	"github.com/score-spec/score-go/framework"
)

/**
THOUGHTS

- build a simple graph of the resources and workload
- for each node, generate a struct literal, and for each placeholder, generate an edge to another node
- detect cycles
- do a depth first iteration and generate the expressions




*/

type ComponentInstance struct {
	// Package is the import path such as github.com/pulumi/pulumi-aws/sdk/v6/go/aws/rds
	// The alias is assumed to be the same as the package name unless overridden at generation time
	Package string
	// Constructor is the name of the constructor, such as NewInstance
	Constructor string
	// ArgsType is the type of the arguments to the constructor, such as InstanceArgs
	ArgsType string

	// Name is the name of the pulumi resource
	Name string

	// Params are the resource parameters
	Params map[string]interface{}
	// ParamsDefinedBy is the component that defines the params
	ParamsDefinedBy ComponentGoIdentifier
}

type ComponentGoIdentifier string

type LocalAlias string

type ComponentGraph struct {
	Nodes        map[ComponentGoIdentifier]ComponentInstance
	Dependencies map[ComponentGoIdentifier]map[LocalAlias]ComponentGoIdentifier
}

func (g *ComponentGraph) VisitInDependencyOrder(visit func(ComponentGoIdentifier) error) error {
	visited := make(map[ComponentGoIdentifier]bool)
	visiting := make(map[ComponentGoIdentifier]bool)

	var visitNode func(ComponentGoIdentifier) error
	visitNode = func(node ComponentGoIdentifier) error {
		if visiting[node] {
			return fmt.Errorf("cycle detected at node %v", node)
		}
		if visited[node] {
			return nil
		}
		visiting[node] = true
		for _, alias := range slices.Sorted(maps.Keys(g.Dependencies[node])) {
			if err := visitNode(g.Dependencies[node][alias]); err != nil {
				return err
			}
		}
		visiting[node] = false
		visited[node] = true

		return visit(node)
	}

	for _, node := range slices.Sorted(maps.Keys(g.Nodes)) {
		if err := visitNode(node); err != nil {
			return err
		}
	}
	return nil
}

func GenerateGoVar(raw string) ComponentGoIdentifier {
	var nextCap bool
	safePart := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		} else if r >= 'A' && r <= 'Z' {
			if nextCap {
				nextCap = false
				return r
			}
			return r + 'a' - 'A'
		} else if r >= 'a' && r <= 'z' {
			if nextCap {
				nextCap = false
				return r - 'a' + 'A'
			}
			return r
		} else {
			nextCap = true
			return -1
		}
	}, raw)
	if len(safePart) == 0 || safePart[0] > 'z' || safePart[0] < 'a' {
		safePart = "c" + safePart
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(raw))
	return ComponentGoIdentifier(safePart + hex.EncodeToString(h.Sum(nil)))
}

func structToGeneric(s interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildSubstitutionTracker(metadata map[string]interface{}, hook func(alias string) error) framework.Substituter {
	inner := framework.BuildSubstitutionFunction(metadata, nil)
	return framework.Substituter{
		// don't replace
		Replacer: func(s string) (string, error) {
			parts := framework.SplitRefParts(s)
			if len(parts) >= 2 && parts[0] == "resources" {
				if err := hook(parts[1]); err != nil {
					return "", err
				}
				return "${" + s + "}", nil
			}
			return inner(s)
		},
		// don't unescape
		UnEscaper: func(s string) (string, error) {
			return s, nil
		},
	}
}

func (cfg *ScoreConfig) GenerateComponentGraph() (ComponentGraph, error) {
	g := ComponentGraph{
		Nodes:        make(map[ComponentGoIdentifier]ComponentInstance),
		Dependencies: make(map[ComponentGoIdentifier]map[LocalAlias]ComponentGoIdentifier),
	}

	for _, workload := range cfg.Workloads {
		workloadName := workload.Metadata["name"].(string)
		workloadGoIdentifier := GenerateGoVar("workload." + workloadName)
		workloadDeps := make(map[LocalAlias]ComponentGoIdentifier)

		for alias, res := range workload.Resources {
			resId := "workload." + workloadName + "." + alias
			if res.Id != nil {
				resId = "shared." + *res.Id
			}
			resGoIdentifier := GenerateGoVar(resId)
			c, ok := g.Nodes[resGoIdentifier]
			if !ok {
				c = ComponentInstance{
					Package:     "github.com/astromechza/score-pulumi/lib/echo",
					Constructor: "NewEcho",
					ArgsType:    "EchoArgs",
					Name:        resId,
				}
			}
			if res.Params != nil {
				if c.Params != nil && !reflect.DeepEqual(res.Params, c.Params) {
					return g, fmt.Errorf("duplicate resource %q with conflicting parameters", resId)
				} else if c.Params == nil {
					c.Params = res.Params
					c.ParamsDefinedBy = workloadGoIdentifier
				}
			}

			resDeps := make(map[LocalAlias]ComponentGoIdentifier)
			tracker := buildSubstitutionTracker(workload.Metadata, func(otherAlias string) error {
				if r, ok := workload.Resources[otherAlias]; !ok {
					return fmt.Errorf("unknown resource alias %q referenced by params in %q", otherAlias, resId)
				} else if r.Id != nil {
					resDeps[LocalAlias(otherAlias)] = GenerateGoVar("shared." + *r.Id)
				} else {
					resDeps[LocalAlias(otherAlias)] = GenerateGoVar("workload." + workloadName + "." + otherAlias)
				}
				return nil
			})
			if cp, err := tracker.Substitute(c.Params); err != nil {
				return g, err
			} else {
				c.Params = cp.(map[string]interface{})
			}

			g.Nodes[resGoIdentifier] = c
			if len(resDeps) > 0 {
				g.Dependencies[resGoIdentifier] = resDeps
			}
			workloadDeps[LocalAlias(alias)] = resGoIdentifier
		}

		workloadParams := make(map[string]interface{})
		if m, err := structToGeneric(workload.Metadata); err != nil {
			return g, err
		} else {
			workloadParams["metadata"] = m
		}
		if c, err := structToGeneric(workload.Containers); err != nil {
			return g, err
		} else {
			workloadParams["containers"] = c
		}
		if workload.Service != nil {
			if s, err := structToGeneric(workload.Service); err != nil {
				return g, err
			} else {
				workloadParams["service"] = s
			}
		}
		g.Nodes[workloadGoIdentifier] = ComponentInstance{
			Package:         "github.com/astromechza/score-pulumi/lib/echo",
			Constructor:     "NewEcho",
			ArgsType:        "EchoArgs",
			Name:            "workload." + workloadName,
			Params:          workloadParams,
			ParamsDefinedBy: workloadGoIdentifier,
		}
		if len(workloadDeps) > 0 {
			g.Dependencies[workloadGoIdentifier] = workloadDeps
		}
	}

	return g, nil
}

func BuildJenFile(g ComponentGraph) (*jen.File, error) {
	f := jen.NewFile("main")

	blockParts := make([]jen.Code, 0)
	if err := g.VisitInDependencyOrder(func(id ComponentGoIdentifier) error {
		n := g.Nodes[id]
		blockParts = append(
			blockParts,
			jen.List(jen.Id(string(id)), jen.Error()).Op(":=").Qual(n.Package, n.Constructor).Call(jen.Id("ctx"), jen.Op("&").Qual(n.Package, n.ArgsType).Values(jen.DictFunc(func(d jen.Dict) {}))),
		)
		return nil
	}); err != nil {
		return nil, err
	}

	f.Func().Id("main").Params().Block(
		jen.Qual("github.com/pulumi/pulumi/sdk/v3/go/pulumi", "Run").Call(jen.Func().Params(
			jen.Id("ctx").Op("*").Qual("github.com/pulumi/pulumi/sdk/v3/go/pulumi", "Context"),
		).Error().Block(
			blockParts...,
		)),
	)

	return f, nil
}
