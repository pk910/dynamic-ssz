module github.com/pk910/dynamic-ssz/test

go 1.21.1

require (
	github.com/attestantio/go-eth2-client v0.24.0
	github.com/pk910/dynamic-ssz v0.0.4
)

require (
	github.com/emicklei/dot v1.6.4 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/ferranbt/fastssz v0.1.4 // indirect
	github.com/goccy/go-yaml v1.9.2 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15 // indirect
	golang.org/x/crypto v0.32.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

//replace github.com/attestantio/go-eth2-client => github.com/pk910/go-eth2-client v0.0.0-20240817214544-da46a1c4cc0a

//replace github.com/attestantio/go-eth2-client => ../../go-eth2-client
replace github.com/attestantio/go-eth2-client => github.com/pk910/go-eth2-client v0.0.0-20250218205628-270f1b6bcae3

replace github.com/pk910/dynamic-ssz => ../
