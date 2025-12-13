package sszutils

func ResolveSpecValueWithDefault(ds DynamicSpecs, name string, defaultValue uint64) (uint64, error) {
	hasLimit, limit, err := ds.ResolveSpecValue(name)
	if err != nil {
		return 0, err
	}
	if !hasLimit {
		return defaultValue, nil
	}
	return limit, nil
}
