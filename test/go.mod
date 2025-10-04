module github.com/pk910/dynamic-ssz/test

go 1.22.2

require (
	github.com/attestantio/go-eth2-client v0.25.2
	github.com/holiman/uint256 v1.3.2
	github.com/pk910/dynamic-ssz v1.1.0
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/OffchainLabs/hashtree v0.2.1-0.20250530191054-577f0b75c7f7 // indirect
	github.com/casbin/govaluate v1.8.0 // indirect
	github.com/emicklei/dot v1.6.4 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/ferranbt/fastssz v0.1.4 // indirect
	github.com/goccy/go-yaml v1.9.2 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
)

replace github.com/attestantio/go-eth2-client => ../../go-eth2-client

//replace github.com/attestantio/go-eth2-client => github.com/pk910/go-eth2-client v0.0.0-20250624161731-3d549c5576da // waiting for https://github.com/attestantio/go-eth2-client/pull/242

replace github.com/pk910/dynamic-ssz => ../
