package internal

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/dave/jennifer/jen"
	"github.com/score-spec/score-go/framework"
)

const (
	DefaultPulumiPackage = "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

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
			resClass := "default"
			if res.Class != nil {
				resClass = *res.Class
			}
			resGoIdentifier := GenerateGoVar(resId)
			c, ok := g.Nodes[resGoIdentifier]
			if !ok {
				componentEntry, ok := FindResourceComponent(cfg.ResourceComponents, res.Type, resClass, resId)
				if !ok {
					return g, fmt.Errorf("failed to find an entry in the component library to provision resource '%s' with type '%s' and class '%s'", resId, res.Type, resClass)
				}
				// TODO: we need to validate the resource component entries
				c = ComponentInstance{
					Package:     componentEntry.Package,
					Constructor: componentEntry.ConstructorFunc,
					ArgsType:    componentEntry.ArgsStruct,
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

		// TODO: we need to validate the workload component entry
		g.Nodes[workloadGoIdentifier] = ComponentInstance{
			Package:         cfg.DefaultWorkloadComponent.Package,
			Constructor:     cfg.DefaultWorkloadComponent.ConstructorFunc,
			ArgsType:        cfg.DefaultWorkloadComponent.ArgsStruct,
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

func mapLookupOutput(ctx map[string]interface{}) func(keys ...string) (interface{}, error) {
	return func(keys ...string) (interface{}, error) {
		var resolvedValue interface{}
		resolvedValue = ctx
		for _, k := range keys {
			mapV, ok := resolvedValue.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("cannot lookup key '%s', context is not a map", k)
			}
			resolvedValue, ok = mapV[k]
			if !ok {
				return "", fmt.Errorf("key '%s' not found", k)
			}
		}
		return resolvedValue, nil
	}
}

func buildInnerSubstitutionFunc(metadata map[string]interface{}, dependencies map[LocalAlias]ComponentGoIdentifier) func(fmtArgs *[]jen.Code) func(s string) (string, error) {
	metadataLookup := mapLookupOutput(metadata)
	return func(fmtArgs *[]jen.Code) func(s string) (string, error) {
		return func(ref string) (string, error) {
			parts := framework.SplitRefParts(ref)
			switch parts[0] {
			case "metadata":
				if len(parts) < 2 {
					return "", fmt.Errorf("invalid ref '%s': requires at least a metadata key to lookup", ref)
				}
				rv, err := metadataLookup(parts[1:]...)
				if err != nil {
					return "", fmt.Errorf("invalid ref '%s': %w", ref, err)
				}
				*fmtArgs = append(*fmtArgs, jen.Lit(rv))
				return "%v", nil
			case "resources":
				if len(parts) < 2 {
					return "", fmt.Errorf("invalid ref '%s': requires at least a resource name to lookup", ref)
				}
				rv, ok := dependencies[LocalAlias(parts[1])]
				if !ok {
					return "", fmt.Errorf("invalid ref '%s': no known resource '%s'", ref, parts[1])
				}
				c := jen.Id(string(rv))
				for _, p := range parts[2:] {
					c = c.Dot(toParamName(p).GoString())
				}
				*fmtArgs = append(*fmtArgs, c)
				return "%v", nil
			default:
				return "", fmt.Errorf("invalid ref '%s': unknown reference root, use $$ to escape the substitution", ref)
			}
		}
	}
}

func pulumifyValue(path []string, raw interface{}, innerSubstFunc func(fmtArgs *[]jen.Code) func(s string) (string, error)) (jen.Code, error) {
	if raw == nil {
		return jen.Nil(), nil
	}
	switch typed := raw.(type) {
	case string:
		if strings.Contains(typed, "${") {
			typed = strings.ReplaceAll(typed, "%", "%%")
			fmtArgs := make([]jen.Code, 0)
			v, err := framework.SubstituteString(typed, innerSubstFunc(&fmtArgs))
			if err != nil {
				return nil, fmt.Errorf("failed to substitute %q at %s: %w", typed, strings.Join(path, "."), err)
			}
			fmtArgs = append([]jen.Code{jen.Lit(v)}, fmtArgs...)
			return jen.Qual(DefaultPulumiPackage, "Sprintf").Call(fmtArgs...), nil
		}
		return jen.Qual(DefaultPulumiPackage, "String").Call(jen.Lit(typed)), nil
	case bool:
		if typed {
			return jen.Qual(DefaultPulumiPackage, "Bool").Call(jen.True()), nil
		}
		return jen.Qual(DefaultPulumiPackage, "Bool").Call(jen.False()), nil
	case float64:
		return jen.Qual(DefaultPulumiPackage, "Float64").Call(jen.Lit(typed)), nil
	case int:
		return jen.Qual(DefaultPulumiPackage, "Int").Call(jen.Lit(typed)), nil
	case []interface{}:
		listValues := make([]jen.Code, 0, len(typed))
		for i, v := range typed {
			out, err := pulumifyValue(append(path, fmt.Sprintf("[%d]", i)), v, innerSubstFunc)
			if err != nil {
				return nil, err
			}
			listValues = append(listValues, out)
		}
		return jen.Qual(DefaultPulumiPackage, "Array").Values(jen.List(listValues...)), nil
	case map[string]interface{}:
		mapValues := make(jen.Dict, len(typed))
		for k, v := range typed {
			out, err := pulumifyValue(append(path, k), v, innerSubstFunc)
			if err != nil {
				return nil, err
			}
			mapValues[jen.Lit(k)] = out
		}
		return jen.Qual(DefaultPulumiPackage, "Map").Values(mapValues), nil
	default:
		panic(fmt.Sprintf("unsupported type %T", typed))
	}
}

// toParamName converts field names into Go-like field identifiers in CamelCase.
func toParamName(p string) *jen.Statement {
	var buff bytes.Buffer
	nextUpper := true
	for _, v := range p {
		if unicode.IsUpper(v) || unicode.IsDigit(v) {
			buff.WriteRune(v)
			nextUpper = false
		} else if unicode.IsLower(v) {
			if nextUpper {
				buff.WriteRune(unicode.ToUpper(v))
			} else {
				buff.WriteRune(v)
			}
			nextUpper = false
		} else {
			nextUpper = true
		}
	}
	return jen.Id(buff.String())
}

func BuildJenFile(g ComponentGraph) (*jen.File, error) {
	f := jen.NewFile("main")

	blockParts := make([]jen.Code, 0)
	if err := g.VisitInDependencyOrder(func(id ComponentGoIdentifier) error {
		n := g.Nodes[id]

		substFunc := buildInnerSubstitutionFunc(n.Params, g.Dependencies[id])
		argAssignments := make(jen.Dict, len(n.Params))
		for k, v := range n.Params {
			o, err := pulumifyValue([]string{k}, v, substFunc)
			if err != nil {
				return err
			}
			argAssignments[toParamName(k)] = o
		}

		blockParts = append(
			blockParts,
			jen.List(jen.Id(string(id)), jen.Err()).Op(":=").Qual(n.Package, n.Constructor).Call(jen.Id("ctx"), jen.Lit(n.Name), jen.Op("&").Qual(n.Package, n.ArgsType).Values(jen.DictFunc(func(d jen.Dict) {
				for k, v := range argAssignments {
					d[k] = v
				}
			}))),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Err())),
			jen.Id("_").Op("=").Id("ctx.Log.Debug").Call(jen.Lit("provisioned"), jen.Op("&").Qual(DefaultPulumiPackage, "LogArgs").Values(jen.Dict{jen.Id("Resource"): jen.Id(string(id))})),
			jen.Line(),
		)
		return nil
	}); err != nil {
		return nil, err
	}

	blockParts = append(blockParts, jen.Return(jen.Nil()))

	f.Func().Id("main").Params().Block(
		jen.Qual(DefaultPulumiPackage, "Run").Call(jen.Func().Params(
			jen.Id("ctx").Op("*").Qual(DefaultPulumiPackage, "Context"),
		).Error().Block(
			blockParts...,
		)),
	)

	return f, nil
}
