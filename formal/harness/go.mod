module github.com/pk910/dynamic-ssz/formal/harness

go 1.25.0

require (
	github.com/golang/snappy v1.0.0
	github.com/pk910/dynamic-ssz v1.4.0
	gopkg.in/yaml.v2 v2.4.0
	pgregory.net/rapid v1.3.0
)

require (
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15 // indirect
)

require (
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/pk910/dynamic-ssz/spectests v0.0.0
	github.com/pk910/hashtree-bindings v0.2.6 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/pk910/dynamic-ssz => ../../

replace github.com/pk910/dynamic-ssz/spectests => ../../tests/spectests
