module github.com/pk910/dynamic-ssz/spectests

go 1.25.0

require (
	github.com/golang/snappy v1.0.0
	github.com/holiman/uint256 v1.3.2
	github.com/huandu/go-clone/generic v1.7.3
	github.com/pk910/dynamic-ssz v1.3.3-0.20260812091520-ef568569f9c1
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15
	github.com/stretchr/testify v1.11.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/OffchainLabs/go-bitfield v0.0.0-20251031151322-f427d04d8506 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/ethpandaops/go-eth2-client v0.1.7
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/huandu/go-clone v1.7.3 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/pk910/hashtree-bindings v0.2.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool github.com/pk910/dynamic-ssz/dynssz-gen

replace github.com/pk910/dynamic-ssz => ../../
