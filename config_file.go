package main

import (
	"os"

	"github.com/go-yaml/yaml"
)

type ClientConfig struct {
	URL     string `yaml:"url"`
	Token   string `yaml:"token"`
	Cookies []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"cookies"`
	UserName string `yaml:"user_name"`
}

type StructureConfig struct {
	ID               int    `yaml:"id"`
	JQL              string `yaml:"jql"`
	ParallelProjects int    `yaml:"parallel_projects"`
	StartDateID      int    `yaml:"start_date_id"`
}

type FileConfig struct {
	Client     ClientConfig               `yaml:"client"`
	Structures map[string]StructureConfig `yaml:"structures"`
}

func loadConfig(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
