package internal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBuildWorkloadComponentForProfile_builtin(t *testing.T) {
	for k, p := range supportedWorkloadModules {
		t.Run(k, func(t *testing.T) {
			e, ok := parseWorkloadProfileOneLiner(p)
			assert.True(t, ok)
			assert.Regexp(t, ".*/"+k, e.Package)
			assert.Equal(t, "New", e.ConstructorFunc)
			assert.Equal(t, "Args", e.ArgsStruct)
		})
	}
}
