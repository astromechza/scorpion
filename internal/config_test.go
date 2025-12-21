package internal

import (
	"os"
	"testing"

	"github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_missing(t *testing.T) {
	t.Chdir(t.TempDir())
	c, ok, err := LoadConfig()
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ScoreConfig{}, c)
}

func TestLoadConfig_bad_file(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir(ConfigFile, 0o700))
	c, ok, err := LoadConfig()
	require.EqualError(t, err, "failed to decode config file: yaml: input error: read score.config.yaml: is a directory")
	require.False(t, ok)
	require.Equal(t, ScoreConfig{}, c)
}

func TestLoadConfig_bad_content(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(ConfigFile, []byte(`{"workloads":[],"blobs":4}`), 0o644))
	c, ok, err := LoadConfig()
	require.EqualError(t, err, "failed to decode config file: yaml: unmarshal errors:\n  line 1: field blobs not found in type internal.ScoreConfig")
	require.False(t, ok)
	require.Equal(t, ScoreConfig{}, c)
}

func TestLoadConfig_good(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(ConfigFile, []byte(`{"workloads":[{"apiVersion":"score.dev/v1b1","metadata":{"name":"app"},"containers":{"main":{"image":"thing"}}}]}`), 0o644))
	c, ok, err := LoadConfig()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, ScoreConfig{Workloads: []types.Workload{{
		ApiVersion: "score.dev/v1b1",
		Metadata: map[string]interface{}{
			"name": "app",
		},
		Containers: map[string]types.Container{
			"main": {Image: "thing"},
		},
	}}}, c)
}

func TestSaveConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := ScoreConfig{Workloads: []types.Workload{{
		ApiVersion: "score.dev/v1b1",
		Metadata: map[string]interface{}{
			"name": "app",
		},
		Containers: map[string]types.Container{
			"main": {Image: "thing"},
		},
	}}}
	require.NoError(t, SaveConfig(cfg))
	cfg2, ok, err := LoadConfig()
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, cfg, cfg2)
	_, err = os.Stat(ConfigFile + ".tmp")
	require.EqualError(t, err, "stat score.config.yaml.tmp: no such file or directory")
}

func Test_isResourceComponentAMatch(t *testing.T) {
	entry := ResourceComponentEntry{
		ResourceType:       "eg",
		ResourceClassRegex: `.*`,
		ResourceIdRegex:    `.*`,
	}
	f := buildResourceComponentMatcher("eg", "default", "some.resource")
	assert.True(t, f(entry))
	t.Run("with bad type", func(t *testing.T) {
		entry := entry
		entry.ResourceType = "unknown"
		assert.False(t, f(entry))
	})
	t.Run("with wrong class", func(t *testing.T) {
		entry := entry
		entry.ResourceClassRegex = `noth.*`
		assert.False(t, f(entry))
	})
	t.Run("with wrong id", func(t *testing.T) {
		entry := entry
		entry.ResourceIdRegex = `noth.*`
		assert.False(t, f(entry))
	})
	t.Run("with bad class regex", func(t *testing.T) {
		entry := entry
		entry.ResourceClassRegex = `*`
		assert.False(t, f(entry))
	})
	t.Run("with bad id regex", func(t *testing.T) {
		entry := entry
		entry.ResourceIdRegex = `*`
		assert.False(t, f(entry))
	})
}

func Test_FindResourceComponent(t *testing.T) {
	_, ok := FindResourceComponent(nil, "eg", "default", "some.resource")
	assert.False(t, ok)
	e, ok := FindResourceComponent([]ResourceComponentEntry{{
		ResourceType: "eg", ResourceClassRegex: `.*`, ResourceIdRegex: `.*`,
	}}, "eg", "default", "some.resource")
	assert.True(t, ok)
	assert.Equal(t, `.*`, e.ResourceClassRegex)
}
