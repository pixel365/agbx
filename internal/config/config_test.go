package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	exampleImageName         = "example/image"
	additionalMountSource    = "docs"
	additionalMountTarget    = AdditionalMountDirectory + "/docs"
	mountSourceEnvironment   = "AGBX_TEST_MOUNT_SOURCE"
	providerMountSource      = "instructions"
	providerMountTarget      = AdditionalMountDirectory + "/instructions"
	otherProviderName        = "codex"
	otherProviderMountSource = "codex-instructions"
	otherProviderMountTarget = AdditionalMountDirectory + "/codex-instructions"
	testProviderName         = "claude"
	validConfigYAML          = "version: 1\nimage:\n  name: " + exampleImageName + "\n  tag: latest\n"
	validConfigWithMountYAML = "version: 1\nimage:\n  name: " + exampleImageName +
		"\n  tag: latest\nmounts:\n  - source: " + additionalMountSource +
		"\n    target: " + additionalMountTarget + "\n"
)

var validConfig = Config{
	Version: currentVersion,
	Image: Image{
		Name: exampleImageName,
		Tag:  "latest",
	},
}

func TestLoadReadsFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), defaultYAMLFileName)
	require.NoError(t, os.WriteFile(filePath, []byte(validConfigYAML), 0o600))

	configuration, err := Load(filePath)

	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
}

func TestLoadReadsMounts(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, defaultYAMLFileName)
	require.NoError(t, os.Mkdir(filepath.Join(directory, additionalMountSource), 0o700))
	require.NoError(t, os.WriteFile(filePath, []byte(validConfigWithMountYAML), 0o600))

	configuration, err := Load(filePath)

	require.NoError(t, err)
	assert.Equal(t, []Mount{{
		Source: filepath.Join(directory, additionalMountSource),
		Target: additionalMountTarget,
	}}, configuration.Mounts)
	assert.True(t, configuration.Mounts[0].IsReadOnly())
}

func TestLoadReadsProviderMounts(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, defaultYAMLFileName)
	require.NoError(t, os.Mkdir(filepath.Join(directory, providerMountSource), 0o700))
	contents := validConfigYAML + "providers:\n  " + testProviderName + ":\n    mounts:\n" +
		"      - source: " + providerMountSource + "\n        target: " + providerMountTarget + "\n"
	require.NoError(t, os.WriteFile(filePath, []byte(contents), 0o600))

	configuration, err := Load(filePath)

	require.NoError(t, err)
	assert.Equal(t, []Mount{{
		Source: filepath.Join(directory, providerMountSource),
		Target: providerMountTarget,
	}}, configuration.Providers[testProviderName].Mounts)
}

func TestLoadRejectsMissingProviderMountSource(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, defaultYAMLFileName)
	contents := validConfigYAML + "providers:\n  " + testProviderName + ":\n    mounts:\n" +
		"      - source: " + providerMountSource + "\n        target: " + providerMountTarget + "\n"
	require.NoError(t, os.WriteFile(filePath, []byte(contents), 0o600))

	configuration, err := Load(filePath)

	assert.Equal(t, Config{}, configuration)
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, "config provider \"claude\" mounts")
}

func TestLoadRejectsMissingMountSource(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, defaultYAMLFileName)
	require.NoError(t, os.WriteFile(filePath, []byte(validConfigWithMountYAML), 0o600))

	configuration, err := Load(filePath)

	assert.Equal(t, Config{}, configuration)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestLoadExpandsMountSourceEnvironmentVariable(t *testing.T) {
	directory := t.TempDir()
	sourceDirectory := t.TempDir()
	t.Setenv(mountSourceEnvironment, sourceDirectory)
	filePath := filepath.Join(directory, defaultYAMLFileName)
	contents := validConfigYAML + "mounts:\n  - source: ${" + mountSourceEnvironment +
		"}\n    target: " + additionalMountTarget + "\n"
	require.NoError(t, os.WriteFile(filePath, []byte(contents), 0o600))

	configuration, err := Load(filePath)

	require.NoError(t, err)
	assert.Equal(t, sourceDirectory, configuration.Mounts[0].Source)
}

func TestLoadRejectsUnsetMountSourceEnvironmentVariable(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(mountSourceEnvironment, "")
	filePath := filepath.Join(directory, defaultYAMLFileName)
	contents := validConfigYAML + "mounts:\n  - source: ${" + mountSourceEnvironment +
		"}\n    target: " + additionalMountTarget + "\n"
	require.NoError(t, os.WriteFile(filePath, []byte(contents), 0o600))

	configuration, err := Load(filePath)

	assert.Equal(t, Config{}, configuration)
	assert.ErrorContains(t, err, "environment variable \""+mountSourceEnvironment+"\" is not set")
}

func TestLoadReturnsErrorForMissingFile(t *testing.T) {
	configuration, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))

	assert.Equal(t, Config{}, configuration)
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestLoadReturnsErrorForDirectory(t *testing.T) {
	directory := t.TempDir()

	configuration, err := Load(directory)

	assert.Equal(t, Config{}, configuration)
	assert.Error(t, err)
}

func TestLoadDefaultReadsYAMLFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, defaultYAMLFileName), []byte(validConfigYAML), 0o600),
	)

	configuration, err := LoadDefault(directory)

	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
}

func TestLoadDefaultReadsYMLFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, defaultYMLFileName), []byte(validConfigYAML), 0o600),
	)

	configuration, err := LoadDefault(directory)

	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
}

func TestLoadDefaultReturnsNotFound(t *testing.T) {
	configuration, err := LoadDefault(t.TempDir())

	assert.Equal(t, Config{}, configuration)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestConfigValidateRequiresVersion(t *testing.T) {
	err := Config{}.Validate()

	assert.EqualError(t, err, "config version is required")
}

func TestConfigValidateRejectsUnsupportedVersion(t *testing.T) {
	configuration := validConfig
	configuration.Version++
	err := configuration.Validate()

	assert.EqualError(t, err, "unsupported config version 2")
}

func TestConfigValidateRequiresImageName(t *testing.T) {
	configuration := validConfig
	configuration.Image.Name = ""

	err := configuration.Validate()

	assert.EqualError(t, err, "config image name is required")
}

func TestConfigValidateRequiresImageTag(t *testing.T) {
	configuration := validConfig
	configuration.Image.Tag = ""

	err := configuration.Validate()

	assert.EqualError(t, err, "config image tag is required")
}

func TestConfigValidateRejectsInvalidMount(t *testing.T) {
	testCases := []struct {
		name  string
		mount Mount
		want  string
	}{
		{
			name:  "missing source",
			mount: Mount{Target: additionalMountTarget},
			want:  "config mount 1 source is required",
		},
		{
			name:  "missing target",
			mount: Mount{Source: additionalMountSource},
			want:  "config mount 1 target is required",
		},
		{
			name:  "relative target",
			mount: Mount{Source: additionalMountSource, Target: "docs"},
			want:  "config mount 1 target \"docs\" must be absolute",
		},
		{
			name:  "outside additional mounts",
			mount: Mount{Source: additionalMountSource, Target: "/workspace/AGENTS.md"},
			want: "config mount 1 target \"/workspace/AGENTS.md\" must be inside " +
				"\"" + AdditionalMountDirectory + "\"",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			configuration := validConfig
			configuration.Mounts = []Mount{testCase.mount}

			err := configuration.Validate()

			assert.EqualError(t, err, testCase.want)
		})
	}
}

func TestConfigValidateRejectsOverlappingMountTarget(t *testing.T) {
	configuration := validConfig
	configuration.Mounts = []Mount{
		{Source: additionalMountSource, Target: additionalMountTarget},
		{Source: "more-docs", Target: AdditionalMountDirectory + "/docs/../docs"},
	}

	err := configuration.Validate()

	assert.EqualError(
		t,
		err,
		"config mount 2 target \""+additionalMountTarget+"\" overlaps config mount 1 target \""+
			additionalMountTarget+"\"",
	)
}

func TestConfigValidateRejectsProviderMountTargetOverlap(t *testing.T) {
	configuration := validConfig
	configuration.Mounts = []Mount{{
		Source: additionalMountSource,
		Target: additionalMountTarget,
	}}
	configuration.Providers = map[string]ProviderConfig{
		testProviderName: {Mounts: []Mount{{
			Source: providerMountSource,
			Target: additionalMountTarget + "/instructions",
		}}},
	}

	err := configuration.Validate()

	assert.EqualError(
		t,
		err,
		"config provider \"claude\" mounts: config mount 2 target \""+additionalMountTarget+
			"/instructions\" overlaps config mount 1 target \""+additionalMountTarget+"\"",
	)
}

func TestConfigMountsForProviderCombinesSharedAndProviderMounts(t *testing.T) {
	configuration := validConfig
	configuration.Mounts = []Mount{{
		Source: additionalMountSource,
		Target: additionalMountTarget,
	}}
	configuration.Providers = map[string]ProviderConfig{
		testProviderName: {Mounts: []Mount{{
			Source: providerMountSource,
			Target: providerMountTarget,
		}}},
		otherProviderName: {Mounts: []Mount{{
			Source: otherProviderMountSource,
			Target: otherProviderMountTarget,
		}}},
	}

	mounts, err := configuration.MountsForProvider(testProviderName)

	require.NoError(t, err)
	assert.Equal(t, []Mount{
		{Source: additionalMountSource, Target: additionalMountTarget},
		{Source: providerMountSource, Target: providerMountTarget},
	}, mounts)
}

func TestConfigValidateAllowsAdditionalMountTarget(t *testing.T) {
	configuration := validConfig
	configuration.Mounts = []Mount{{
		Source: additionalMountSource,
		Target: AdditionalMountDirectory + "/AGENTS.md",
	}}

	assert.NoError(t, configuration.Validate())
}

func TestMountIsReadOnlyDefaultsToTrue(t *testing.T) {
	readWrite := false

	assert.True(t, Mount{}.IsReadOnly())
	assert.False(t, Mount{ReadOnly: &readWrite}.IsReadOnly())
}

func TestImageReference(t *testing.T) {
	assert.Equal(t, exampleImageName+":1.0", Image{
		Name: exampleImageName,
		Tag:  "1.0",
	}.Reference())
	assert.Equal(t, exampleImageName+":1.0@sha256:abc", Image{
		Name:   exampleImageName,
		Tag:    "1.0",
		Digest: "sha256:abc",
	}.Reference())
}

func TestCreateWritesValidConfig(t *testing.T) {
	directory := t.TempDir()

	require.NoError(t, Create(directory, validConfig))

	configuration, err := Load(filepath.Join(directory, defaultYAMLFileName))
	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
}
