// Package sizeguard holds the matrix of every construct the code generator
// forms or bounds a size from, against every generated path that forms one.
//
// A size that is bounded on one path and not another is how a value gets past
// a limit on that path alone, and the shape that keeps recurring is a helper
// reached from five of its six sites. The matrix states, per construct and per
// path, what a value past the limit produces: the condition the path reports,
// or why it reports nothing. A guard that stops being emitted turns a cell from
// a report into an acceptance.
//
// The fixtures live here rather than in codegen/tests so the recorded outcomes
// are not archived with a release and can be changed by a later one.
package sizeguard

//go:generate go run ../../dynssz-gen -config gen_ssz.yaml
