package codegen

import (
	"runtime/debug"
)

var Version = "unknown"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/pk910/dynamic-ssz" {
				Version = dep.Version
				break
			}
		}
	}
}
