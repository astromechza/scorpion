package debug

import (
	"log/slog"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Debug struct {
	pulumi.ResourceState
	Values pulumi.MapOutput
}

type Args struct {
	Values pulumi.MapInput `pulumi:"values"`
}

func New(ctx *pulumi.Context, name string, args *Args, opts ...pulumi.ResourceOption) (*Debug, error) {
	debug := &Debug{}
	if err := ctx.RegisterComponentResource("scorpion:builtin:Debug", name, debug, opts...); err != nil {
		return nil, err
	}
	debug.Values = args.Values.ToMapOutput()
	slog.Info("debug workload profile executing", slog.Any("values", args.Values))
	if err := ctx.RegisterResourceOutputs(debug, pulumi.Map{
		"values": args.Values.ToMapOutput(),
	}); err != nil {
		return nil, err
	}
	return debug, nil
}
