package commandconfig

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/internal/config"
)

const configFlag = "config"

func Load(cmd *cobra.Command) (config.Config, error) {
	if !cmd.Root().PersistentFlags().Changed(configFlag) {
		return config.LoadDefault(".")
	}

	configFile, err := cmd.Root().PersistentFlags().GetString(configFlag)
	if err != nil {
		return config.Config{}, fmt.Errorf("get config flag: %w", err)
	}

	return config.Load(configFile)
}
