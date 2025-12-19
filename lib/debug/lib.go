package main

import (
	"log/slog"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumix"
)

type Args struct {
	Metadata   pulumi.MapInput            `pulumi:"metadata"`
	Containers pulumix.Map[ContainerArgs] `pulumi:"containers"`
	Service    ServiceArgs                `pulumi:"service"`
}

type ContainerArgs struct {
	Image pulumi.StringInput `pulumi:"image"`
}

type ServiceArgs struct {
	Ports pulumix.Map[ServicePort] `pulumi:"ports"`
}

type ServicePort struct {
	Port int `pulumi:"port"`
}

type Debug struct {
	pulumi.ResourceState
	Args Args
}

func New(ctx *pulumi.Context, name string, args *Args, opts ...pulumi.ResourceOption) (*Debug, error) {
	debug := &Debug{}
	if err := ctx.RegisterComponentResource("scorpion:builtin:Debug", name, debug, opts...); err != nil {
		return nil, err
	}
	debug.Args = *args
	slog.Info("debug workload profile executing", slog.Any("args", args))
	return debug, nil
}
