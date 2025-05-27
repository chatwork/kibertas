package ktesting

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type HelmChartMeta struct {
	Name         string                `yaml:"name"`
	Version      string                `yaml:"version"`
	Dependencies []HelmChartDependency `yaml:"dependencies"`
}

type HelmChartDependency struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

func LoadHelmChartMeta(chartPath string) (*HelmChartMeta, error) {
	chartMeta := &HelmChartMeta{}

	yamlFile, err := os.ReadFile(chartPath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(yamlFile, chartMeta)
	if err != nil {
		return nil, err
	}

	return chartMeta, nil
}

func (h *HelmChartMeta) GetVersion(name string) (string, error) {
	for _, dependency := range h.Dependencies {
		if dependency.Name == name {
			return dependency.Version, nil
		}
	}

	return "", fmt.Errorf("dependency %s not found", name)
}
