package debug

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Outputs struct {
	pulumi.ResourceState
	Host pulumi.StringOutput `pulumi:"host"`
}

type Inputs struct {
	SubdomainLength pulumi.IntInput    `pulumi:"subdomain_length"`
	ParentDomain    pulumi.StringInput `pulumi:"parent_domain"`
}

const (
	resourceType = "scorpion:builtin:RandomSubdomain"
)

var (
	charset = []byte{
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'j', 'k', 'm', 'n', 'p', 'q', 'r', 's', 't', 'v', 'w', 'x', 'y', 'z',
		'2', '3', '4', '5', '6', '7', '8', '9',
	}
)

func New(ctx *pulumi.Context, name string, args *Inputs, opts ...pulumi.ResourceOption) (*Outputs, error) {
	instance := &Outputs{}
	if err := ctx.RegisterComponentResource(resourceType, name, instance, opts...); err != nil {
		return nil, err
	}

	validatedSubdomainLength := args.SubdomainLength.ToIntOutput().ApplyT(func(i int) (int, error) {
		if i < 1 || i > 63 {
			return 0, fmt.Errorf("invalid subdomain length %d", i)
		}
		return i, nil
	}).(pulumi.IntOutput)

	r, err := random.NewRandomBytes(ctx, "seed", &random.RandomBytesArgs{
		Length: validatedSubdomainLength,
	})
	if err != nil {
		return nil, err
	}

	validatedParentDomain := args.ParentDomain.ToStringOutput().ApplyT(func(s string) (string, error) {
		if !isValidParentDomain(s) {
			return "", fmt.Errorf("invalid parent domain '%s'", s)
		}
		return s, nil
	}).(pulumi.StringOutput)

	instance.Host = pulumi.Sprintf(
		"%v.%v",
		r.Hex.ApplyT(func(hexSeed string) string {
			rawSeed, _ := hex.DecodeString(hexSeed)
			out := make([]byte, len(rawSeed))
			n := len(charset)
			for i := range out {
				out[i] = charset[rawSeed[i]%byte(n)]
			}
			return string(out)
		}).(pulumi.StringOutput),
		validatedParentDomain,
	)

	if err := ctx.RegisterResourceOutputs(instance, pulumi.Map{
		"host": instance.Host,
	}); err != nil {
		return nil, err
	}
	return instance, nil
}

func isValidParentDomain(name string) bool {
	const maxLabelLength = 63
	const maxDomainNameLength = 253
	const maxParentDomainNameLength = maxDomainNameLength - maxLabelLength - 1
	if len(name) == 0 || len(name) > maxParentDomainNameLength {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > maxLabelLength {
			return false
		}
		if !isAlphanumeric(label[0]) || !isAlphanumeric(label[len(label)-1]) {
			return false
		}
		for i := 1; i < len(label)-1; i++ {
			if !isAlphanumeric(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
