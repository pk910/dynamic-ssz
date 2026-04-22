package tests

// The directives below drive the code generator from YAML config files
// rather than long inline CLI invocations. See the per-file .yaml configs
// for what each batch generates; see docs/code-generator-config.md for the
// config format reference.

//go:generate go run -cover ../../dynssz-gen -config gen_ssz.yaml
//go:generate go run -cover ../../dynssz-gen -config gen_extended.yaml
//go:generate go run -cover ../../dynssz-gen -config gen_annotated.yaml
//go:generate go run -cover ../../dynssz-gen -config gen_viewtypes4.yaml
//go:generate go run -cover ../../dynssz-gen -config gen_nodynexpr.yaml
