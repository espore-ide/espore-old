package config

import (
	"espore/utils"
	"fmt"
	"os"
	"path/filepath"
)

type BuildConfig struct {
	Libs    []string `json:"libs"`
	Devices []string `json:"devices"`
	Output  string   `json:"output"`
}

var DefaultConfig = &EsporeConfig{

	Build: BuildConfig{
		Output: "dist",
	},
}

type EsporeConfig struct {
	Build   BuildConfig `json:"build"`
	DataDir string      `json:"dataDir"`
}

func (ec *EsporeConfig) GetDataDir() string {
	if ec.DataDir != "" {
		return ec.DataDir
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(homeDir, ".espore")
}

func Read() (*EsporeConfig, error) {
	var config EsporeConfig
	err := utils.ReadJSON("espore.json", &config)
	if err != nil {
		return DefaultConfig, fmt.Errorf("Cannot find espore.json in the current directory. Using default configuration")
	}
	return &config, nil
}
