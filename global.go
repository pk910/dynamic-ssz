package dynssz

var globalDynSsz *DynSsz

func GetGlobalDynSsz() *DynSsz {
	if globalDynSsz == nil {
		globalDynSsz = NewDynSsz(nil)
	}
	return globalDynSsz
}

func SetGlobalSpecs(specs map[string]any) {
	globalDynSsz = NewDynSsz(specs)
}
