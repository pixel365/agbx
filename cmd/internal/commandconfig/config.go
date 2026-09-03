package commandconfig

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/internal/config"
)

const configFlag = "config"

type LoadedConfig struct {
	FilePath      string
	Configuration config.Config
}

func Load(cmd *cobra.Command) (config.Config, error) {
	loadedConfig, err := LoadWithPath(cmd)
	if err != nil {
		return config.Config{}, err
	}

	return loadedConfig.Configuration, nil
}

func LoadWithPath(cmd *cobra.Command) (LoadedConfig, error) {
	if !cmd.Root().PersistentFlags().Changed(configFlag) {
		configuration, filePath, err := config.LoadDefaultWithPath(".")
		if err != nil {
			return LoadedConfig{}, err
		}

		return LoadedConfig{Configuration: configuration, FilePath: filePath}, nil
	}

	configFile, err := cmd.Root().PersistentFlags().GetString(configFlag)
	if err != nil {
		return LoadedConfig{}, fmt.Errorf("get config flag: %w", err)
	}
	configuration, err := config.Load(configFile)
	if err != nil {
		return LoadedConfig{}, err
	}
	filePath, err := filepath.Abs(configFile)
	if err != nil {
		return LoadedConfig{}, fmt.Errorf("get absolute config path %q: %w", configFile, err)
	}

	return LoadedConfig{Configuration: configuration, FilePath: filepath.Clean(filePath)}, nil
}
