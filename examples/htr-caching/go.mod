module github.com/pk910/dynamic-ssz/examples/htr-caching

go 1.25.8

require github.com/pk910/dynamic-ssz v1.3.2

require (
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/pk910/hashtree-bindings v0.2.5 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool github.com/pk910/dynamic-ssz/dynssz-gen

replace github.com/pk910/dynamic-ssz => ../../
