module github.com/pk910/dynamic-ssz/spectests

go 1.21.0

toolchain go1.22.4

require (
	github.com/attestantio/go-eth2-client v0.26.0
	github.com/golang/snappy v0.0.4
	github.com/huandu/go-clone/generic v1.6.0
	github.com/pk910/dynamic-ssz v0.0.6
	github.com/stretchr/testify v1.8.4
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/casbin/govaluate v1.8.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/dot v1.6.4 // indirect
	github.com/fatih/color v1.10.0 // indirect
	github.com/ferranbt/fastssz v0.1.4 // indirect
	github.com/goccy/go-yaml v1.9.2 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/huandu/go-clone v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15 // indirect
	github.com/prysmaticlabs/gohashtree v0.0.4-beta // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/pk910/dynamic-ssz => ../

replace github.com/attestantio/go-eth2-client => github.com/pk910/go-eth2-client v0.0.0-20250624213751-460cfc4a25b0 // waiting for https://github.com/attestantio/go-eth2-client/pull/242
