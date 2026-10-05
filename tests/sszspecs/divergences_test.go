// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszspecs

// knownDivergences lists the cases the library is known not to pass, by case
// id, with the reason. A listed case has to keep failing: one that starts to
// pass fails the test until it is removed from this list, so the list cannot
// go stale.
// unionCompatibility is the one rule behind every entry so far. EIP-8016
// requires the options of a compatible union to merkleize alike; the library
// leaves that to the schema author (see docs/supported-types.md).
const unionCompatibility = "the library does not verify that the options of a compatible union merkleize alike"

var knownDivergences = map[string]string{
	"compatibility/incompatible/bitlist_capacities":         unionCompatibility,
	"compatibility/incompatible/bitvector_capacities":       unionCompatibility,
	"compatibility/incompatible/container_field_renamed":    unionCompatibility,
	"compatibility/incompatible/container_fields_reordered": unionCompatibility,
	"compatibility/incompatible/layout_name_two_positions":  unionCompatibility,
	"compatibility/incompatible/layout_position_two_names":  unionCompatibility,
	"compatibility/incompatible/list_capacities":            unionCompatibility,
	"compatibility/incompatible/list_element_types":         unionCompatibility,
	"compatibility/incompatible/vector_lengths":             unionCompatibility,
	"illegal/compatible_union/incompatible_options":         unionCompatibility,
}
