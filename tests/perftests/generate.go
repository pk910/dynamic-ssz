package perftests

//go:generate go tool github.com/pk910/dynamic-ssz/dynssz-gen -package . -types SignedBeaconBlock,BeaconBlock,BeaconState,SliceVectorBundle -output gen_ssz.go -legacy -with-streaming
