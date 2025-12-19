package debug

import (
	"log/slog"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Args struct {
	Metadata   pulumi.MapInput `pulumi:"metadata"`
	Containers pulumi.MapInput `pulumi:"containers"`
	Service    pulumi.MapInput `pulumi:"service"`
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
