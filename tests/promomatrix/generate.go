// Package promomatrix holds a generated matrix of embedding shapes.
//
// Go promotes an embedded type's methods to the outer type, and an interface
// assertion cannot tell a promoted method from a declared one. A promoted SSZ
// method answers for the embedded value, so reaching it encodes, sizes or
// hashes the wrong value while reporting success. The matrix places each of the
// six fastssz-style methods in each of three states -- declared on the outer
// type, promoted from the embedded one, or absent -- and asserts every one of
// the 3^6 combinations produces the outer value's answer.
//
// The types are generated rather than committed: 729 of them, with their inner
// counterparts, run to some sixteen thousand lines. Without `go generate` this
// directory holds only this file, so the tree still builds.
package promomatrix

//go:generate go run gen.go
