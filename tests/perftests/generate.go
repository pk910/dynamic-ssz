package perftests

//go:generate go tool github.com/pk910/dynamic-ssz/dynssz-gen -package . -types SignedBeaconBlock,BeaconBlock,BeaconState -output gen_ssz.go -legacy -with-streaming
