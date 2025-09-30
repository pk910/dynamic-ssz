// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"runtime/debug"
)

// Version contains the version string of the dynamic-ssz library used for code generation.
//
// This variable is automatically populated during initialization by examining the build
// information to determine the version of the dynamic-ssz dependency. It's included
// in generated code headers to provide traceability and debugging information.
//
// The version helps identify:
//   - Which version of dynamic-ssz generated the code
//   - Compatibility requirements for the generated code
//   - When to regenerate code after library updates
//
// If the version cannot be determined (e.g., during development), it defaults to "unknown".
var Version = "unknown"

// init automatically determines the dynamic-ssz library version at package initialization.
//
// This function examines the build information available at runtime to identify
// the version of the dynamic-ssz dependency being used. The detected version
// is stored in the Version variable for use in generated code headers.
//
// The function searches through all dependencies in the build information to find
// the github.com/pk910/dynamic-ssz module and extracts its version string.
// If the build information is not available or the dependency is not found,
// the Version remains "unknown".
//
// This automatic version detection ensures that generated code always includes
// accurate version information without manual intervention.
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
