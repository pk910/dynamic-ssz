module github.com/pk910/dynamic-ssz/perftests

go 1.25.0

tool github.com/pk910/dynamic-ssz/dynssz-gen

require (
	github.com/OffchainLabs/go-bitfield v0.0.0-20260316135939-ffb3947a62a5
	github.com/pk910/dynamic-ssz v1.3.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/casbin/govaluate v1.10.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/pk910/hashtree-bindings v0.1.0 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/pk910/dynamic-ssz => ../../
