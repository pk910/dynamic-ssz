package spectests

import (
	_ "embed"

	"gopkg.in/yaml.v2"

	ssz "github.com/pk910/dynamic-ssz"
)

var (
	dynSszOnlyMainnet   *ssz.DynSsz
	dynsszHybridMainnet *ssz.DynSsz
	dynSszOnlyMinimal   *ssz.DynSsz
	dynsszHybridMinimal *ssz.DynSsz
)

//go:embed presets/mainnet-preset.yaml
var mainnetPresetData []byte

//go:embed presets/minimal-preset.yaml
var minimalPresetData []byte

// loadPresetSpecsFromData loads the specifications from embedded YAML data
func loadPresetSpecsFromData(data []byte) (map[string]any, error) {
	var specs map[string]any
	err := yaml.Unmarshal(data, &specs)
	if err != nil {
		return nil, err
	}

	return specs, nil
}

// initializeDynSszInstances initializes the global dynSsz instances with appropriate presets
func initializeDynSszInstances() {
	// Load mainnet preset from embedded data
	mainnetSpecs, err := loadPresetSpecsFromData(mainnetPresetData)
	if err != nil {
		panic("Failed to load mainnet preset: " + err.Error())
	}

	// Load minimal preset from embedded data
	minimalSpecs, err := loadPresetSpecsFromData(minimalPresetData)
	if err != nil {
		panic("Failed to load minimal preset: " + err.Error())
	}

	dynSszOnlyMainnet = ssz.NewDynSsz(mainnetSpecs)
	dynSszOnlyMainnet.NoFastSsz = true

	dynsszHybridMainnet = ssz.NewDynSsz(mainnetSpecs)
	dynsszHybridMainnet.NoFastSsz = false

	dynSszOnlyMinimal = ssz.NewDynSsz(minimalSpecs)
	dynSszOnlyMinimal.NoFastSsz = true

	dynsszHybridMinimal = ssz.NewDynSsz(minimalSpecs)
	dynsszHybridMinimal.NoFastSsz = false
}

func init() {
	initializeDynSszInstances()
}
