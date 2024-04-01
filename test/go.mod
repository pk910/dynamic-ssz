module github.com/pk910/dynamic-ssz/test

go 1.21.1

require (
	github.com/attestantio/go-eth2-client v0.21.1
	github.com/pk910/dynamic-ssz v0.0.0-20240330205931-ea3b4a12b2f2
)

require (
	github.com/Knetic/govaluate v3.0.0+incompatible // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/ferranbt/fastssz v0.1.3 // indirect
	github.com/goccy/go-yaml v1.9.2 // indirect
	github.com/holiman/uint256 v1.2.4 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prysmaticlabs/go-bitfield v0.0.0-20210809151128-385d8c5e3fb7 // indirect
	golang.org/x/crypto v0.20.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/attestantio/go-eth2-client => github.com/pk910/go-eth2-client v0.0.0-20240330075337-93f905e392bd

replace github.com/pk910/dynamic-ssz => ../
