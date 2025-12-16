package internal

import (
	"github.com/dave/jennifer/jen"
	"testing"

	"github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentGraph_empty(t *testing.T) {
	var g ComponentGraph
	assert.NoError(t, g.VisitInDependencyOrder(func(id ComponentGoIdentifier) error {
		assert.Fail(t, "unexpected visit")
		return nil
	}))
}

func TestComponentGraph_cycle(t *testing.T) {
	g := ComponentGraph{
		Nodes: map[ComponentGoIdentifier]ComponentInstance{
			"a": {},
			"b": {},
		},
		Dependencies: map[ComponentGoIdentifier]map[LocalAlias]ComponentGoIdentifier{
			"a": {"b": "b"},
			"b": {"a": "a"},
		},
	}
	assert.EqualError(t, g.VisitInDependencyOrder(func(id ComponentGoIdentifier) error {
		assert.Fail(t, "unexpected visit")
		return nil
	}), "cycle detected at node a")
}

func TestComponentGraph_diamond(t *testing.T) {
	g := ComponentGraph{
		Nodes: map[ComponentGoIdentifier]ComponentInstance{
			"a": {},
			"b": {},
			"c": {},
			"d": {},
		},
		Dependencies: map[ComponentGoIdentifier]map[LocalAlias]ComponentGoIdentifier{
			"a": {"b": "b", "c": "c"},
			"b": {"d": "d"},
			"c": {"d": "d"},
		},
	}
	visited := make([]ComponentGoIdentifier, 0)
	assert.NoError(t, g.VisitInDependencyOrder(func(id ComponentGoIdentifier) error {
		visited = append(visited, id)
		return nil
	}))
	assert.Equal(t, []ComponentGoIdentifier{"d", "b", "c", "a"}, visited)
}

func TestGenerateGoVar(t *testing.T) {
	assert.Equal(t, GenerateGoVar(""), ComponentGoIdentifier("c811c9dc5"))
	assert.Equal(t, GenerateGoVar(" "), ComponentGoIdentifier("c250c8f7f"))
	assert.Equal(t, GenerateGoVar("A"), ComponentGoIdentifier("ac40bf6cc"))
	assert.Equal(t, GenerateGoVar("workload.foo"), ComponentGoIdentifier("workloadFoo6d39e786"))
	assert.Equal(t, GenerateGoVar("workload.bar"), ComponentGoIdentifier("workloadBar2eeefa0f"))
	assert.Equal(t, GenerateGoVar("workload.thing.x"), ComponentGoIdentifier("workloadThingXfebb8a50"))
	assert.Equal(t, GenerateGoVar("shared.db"), ComponentGoIdentifier("sharedDbf876cb74"))
}

func ref(id string) *string { return &id }

func TestGenerateComponentGraph_nominal(t *testing.T) {
	cfg := ScoreConfig{
		Workloads: []types.Workload{
			{
				Metadata: map[string]interface{}{"name": "foo"},
				Resources: map[string]types.Resource{
					"a": {
						Params: map[string]interface{}{
							"plain":   "${resources.b.p}",
							"wrapped": "before ${resources.b.p} after",
						},
					},
					"b": {
						Id: ref("thing"),
						Params: map[string]interface{}{
							"x": "hello",
							"y": "${metadata.name}",
						},
					},
				},
			},
			{
				Metadata: map[string]interface{}{"name": "bar"},
				Resources: map[string]types.Resource{
					"a": {
						Params: map[string]interface{}{
							"raw": "banana",
						},
					},
					"b": {
						Id: ref("thing"),
					},
				},
			},
		},
	}
	g, err := cfg.GenerateComponentGraph()
	assert.NoError(t, err)
	assert.Equal(t, ComponentGraph{
		Nodes: map[ComponentGoIdentifier]ComponentInstance{
			"sharedThingdae392ce":  {Package: "github.com/astromechza/pulumi-echo", Constructor: "NewComponent", ArgsType: "Args", Name: "shared.thing", Params: map[string]interface{}{"x": "hello", "y": "foo"}, ParamsDefinedBy: "workloadFoo6d39e786"},
			"workloadBar2eeefa0f":  {Package: "github.com/astromechza/pulumi-echo", Constructor: "NewComponent", ArgsType: "Args", Name: "workload.bar", Params: map[string]interface{}{"containers": map[string]interface{}(nil), "metadata": map[string]interface{}{"name": "bar"}}, ParamsDefinedBy: "workloadBar2eeefa0f"},
			"workloadBarA9c79b8d6": {Package: "github.com/astromechza/pulumi-echo", Constructor: "NewComponent", ArgsType: "Args", Name: "workload.bar.a", Params: map[string]interface{}{"raw": "banana"}, ParamsDefinedBy: "workloadBar2eeefa0f"},
			"workloadFoo6d39e786":  {Package: "github.com/astromechza/pulumi-echo", Constructor: "NewComponent", ArgsType: "Args", Name: "workload.foo", Params: map[string]interface{}{"containers": map[string]interface{}(nil), "metadata": map[string]interface{}{"name": "foo"}}, ParamsDefinedBy: "workloadFoo6d39e786"},
			"workloadFooAc5757e5b": {Package: "github.com/astromechza/pulumi-echo", Constructor: "NewComponent", ArgsType: "Args", Name: "workload.foo.a", Params: map[string]interface{}{"plain": "${resources.b.p}", "wrapped": "before ${resources.b.p} after"}, ParamsDefinedBy: "workloadFoo6d39e786"},
		},
		Dependencies: map[ComponentGoIdentifier]map[LocalAlias]ComponentGoIdentifier{
			"workloadBar2eeefa0f":  {"a": "workloadBarA9c79b8d6", "b": "sharedThingdae392ce"},
			"workloadFoo6d39e786":  {"a": "workloadFooAc5757e5b", "b": "sharedThingdae392ce"},
			"workloadFooAc5757e5b": {"b": "sharedThingdae392ce"},
		},
	}, g)
	visited := make([]ComponentGoIdentifier, 0)
	assert.NoError(t, g.VisitInDependencyOrder(func(id ComponentGoIdentifier) error {
		visited = append(visited, id)
		return nil
	}))
	assert.Equal(t, []ComponentGoIdentifier{"sharedThingdae392ce", "workloadBarA9c79b8d6", "workloadBar2eeefa0f", "workloadFooAc5757e5b", "workloadFoo6d39e786"}, visited)
}

func TestGeneratePulumiArgsStructAst(t *testing.T) {
	f, err := BuildJenFile(ComponentGraph{
		Nodes: map[ComponentGoIdentifier]ComponentInstance{
			"sharedThingdae392ce": {Package: "github.com/astromechza/score-pulumi/lib/echo", Constructor: "NewEcho", ArgsType: "EchoArgs", Name: "shared.thing", Params: map[string]interface{}{
				"x": "hello",
				"y": 42,
				"z": true,
				"w": []interface{}{"a", "b"},
				"v": map[string]interface{}{"a": 1, "b": 2},
			}, ParamsDefinedBy: "workloadFoo6d39e786"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, `package main

import (
	echo "github.com/astromechza/score-pulumi/lib/echo"
	pulumi "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		sharedThingdae392ce, err := echo.NewEcho(ctx, "shared.thing", &echo.EchoArgs{
			V: pulumi.Map{
				"a": pulumi.Int(1),
				"b": pulumi.Int(2),
			},
			W: pulumi.Array{pulumi.String("a"), pulumi.String("b")},
			X: pulumi.String("hello"),
			Y: pulumi.Int(42),
			Z: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		_ = ctx.Log.Debug("provisioned", &pulumi.LogArgs{Resource: sharedThingdae392ce})

		return nil
	})
}
`, f.GoString())
}

func Test_toParamName(t *testing.T) {
	for k, v := range map[string]string{
		"foo":        "Foo",
		"snake_case": "SnakeCase",
		"field42":    "Field42",
		"SomeField":  "SomeField",
	} {
		t.Run(k, func(t *testing.T) {
			assert.Equal(t, jen.Id(v), toParamName(k))
		})
	}
}
