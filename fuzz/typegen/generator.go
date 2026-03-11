// Package typegen generates random SSZ-compatible Go struct definitions for fuzz testing.
package typegen

import (
	"fmt"
	"math/rand"
	"strings"
)

// Config controls the type generation parameters.
type Config struct {
	NumTypes     int  // Number of top-level types to generate
	MaxFields    int  // Maximum fields per struct
	MaxDepth     int  // Maximum nesting depth
	MaxArrayLen  int  // Maximum fixed array length
	MaxListLimit int  // Maximum list ssz-max value
	Extended     bool // Generate extended types (signed ints, floats, big.Int, optional)
	Seed         int64
}

// DefaultConfig returns a reasonable default configuration.
func DefaultConfig() Config {
	return Config{
		NumTypes:     50,
		MaxFields:    8,
		MaxDepth:     3,
		MaxArrayLen:  8,
		MaxListLimit: 32,
		Extended:     false,
		Seed:         0,
	}
}

// TypeDef represents a generated struct definition.
type TypeDef struct {
	Name     string
	Fields   []FieldDef
	Extended bool // whether this type uses extended types
}

// FieldDef represents a single field in a generated struct.
type FieldDef struct {
	Name    string
	GoType  string
	Tags    string
	Imports []string
}

// Generator generates random SSZ-compatible Go struct types.
type Generator struct {
	rng      *rand.Rand
	cfg      Config
	types    []TypeDef
	nextID   int
	refTypes []string // types available for nesting references
}

// NewGenerator creates a new type generator.
func NewGenerator(cfg Config) *Generator {
	return &Generator{
		rng:    rand.New(rand.NewSource(cfg.Seed)),
		cfg:    cfg,
		types:  make([]TypeDef, 0, cfg.NumTypes),
		nextID: 0,
	}
}

// Generate creates the configured number of random types.
// Returns the generated type definitions.
func (g *Generator) Generate() []TypeDef {
	for range g.cfg.NumTypes {
		td := g.generateType(0)
		g.types = append(g.types, td)
		g.refTypes = append(g.refTypes, td.Name)
	}

	return g.types
}

// WriteGoSource writes all generated types as valid Go source code.
func (g *Generator) WriteGoSource(packageName string) string {
	var sb strings.Builder

	// Collect imports
	imports := make(map[string]bool)
	for _, td := range g.types {
		for _, f := range td.Fields {
			for _, imp := range f.Imports {
				imports[imp] = true
			}
		}
	}

	fmt.Fprintf(&sb, "package %s\n\n", packageName)

	if len(imports) > 0 {
		sb.WriteString("import (\n")
		for imp := range imports {
			fmt.Fprintf(&sb, "\t\"%s\"\n", imp)
		}
		sb.WriteString(")\n\n")
	}

	for _, td := range g.types {
		fmt.Fprintf(&sb, "type %s struct {\n", td.Name)
		for _, f := range td.Fields {
			if f.Tags != "" {
				fmt.Fprintf(&sb, "\t%s %s %s\n", f.Name, f.GoType, f.Tags)
			} else {
				fmt.Fprintf(&sb, "\t%s %s\n", f.Name, f.GoType)
			}
		}
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

// WriteRegistry generates a registry_gen.go that populates corpus.Registry.
func (g *Generator) WriteRegistry(packageName string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "package %s\n\n", packageName)
	sb.WriteString("func init() {\n")
	sb.WriteString("\tRegistry = []TypeEntry{\n")

	for _, td := range g.types {
		ext := "false"
		if td.Extended {
			ext = "true"
		}
		fmt.Fprintf(&sb,
			"\t\t{Name: %q, New: func() any { return &%s{} }, Extended: %s},\n",
			td.Name, td.Name, ext,
		)
	}

	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	return sb.String()
}

func (g *Generator) generateType(depth int) TypeDef {
	name := fmt.Sprintf("FuzzType%d", g.nextID)
	g.nextID++

	numFields := 1 + g.rng.Intn(g.cfg.MaxFields)
	fields := make([]FieldDef, 0, numFields)
	usesExtended := false

	for i := range numFields {
		f := g.generateField(depth, i)
		if len(f.Imports) > 0 {
			usesExtended = true
		}
		fields = append(fields, f)
	}

	return TypeDef{
		Name:     name,
		Fields:   fields,
		Extended: usesExtended || g.cfg.Extended,
	}
}

func (g *Generator) generateField(depth, index int) FieldDef {
	name := fmt.Sprintf("Field%d", index)

	// At max depth, only generate basic/leaf fields
	if depth >= g.cfg.MaxDepth {
		return g.generateLeafField(name)
	}

	// Choose field category with weighted distribution
	// 0-29: basic types
	// 30-39: byte arrays
	// 40-49: fixed arrays of basic
	// 50-59: lists (slices with ssz-max)
	// 60-64: fixed-size slices (ssz-size)
	// 65-69: byte lists
	// 70-74: bitvector
	// 75-79: bitlist
	// 80-84: fixed-size byte slice (ssz-size)
	// 85-89: nested struct
	// 90-94: vector of structs
	// 95-99: extended types (if enabled, otherwise basic)
	roll := g.rng.Intn(100)

	switch {
	case roll < 30:
		return g.generateBasicField(name)
	case roll < 40:
		return g.generateByteArrayField(name)
	case roll < 50:
		return g.generateFixedArrayField(name, depth)
	case roll < 60:
		return g.generateListField(name, depth)
	case roll < 65:
		return g.generateFixedSizeSliceField(name)
	case roll < 70:
		return g.generateByteListField(name)
	case roll < 75:
		return g.generateBitvectorField(name)
	case roll < 80:
		return g.generateBitlistField(name)
	case roll < 85:
		return g.generateFixedByteSliceField(name)
	case roll < 90:
		return g.generateNestedStructField(name, depth)
	case roll < 95:
		return g.generateVectorOfStructsField(name, depth)
	default:
		if g.cfg.Extended {
			return g.generateExtendedField(name)
		}
		return g.generateBasicField(name)
	}
}

// generateLeafField generates a field type that doesn't reference other structs.
func (g *Generator) generateLeafField(name string) FieldDef {
	roll := g.rng.Intn(100)
	switch {
	case roll < 50:
		return g.generateBasicField(name)
	case roll < 70:
		return g.generateByteArrayField(name)
	case roll < 80:
		return g.generateByteListField(name)
	case roll < 85:
		return g.generateFixedByteSliceField(name)
	case roll < 90:
		return g.generateBitvectorField(name)
	case roll < 95:
		return g.generateBitlistField(name)
	default:
		return g.generateFixedSizeSliceField(name)
	}
}

func (g *Generator) generateBasicField(name string) FieldDef {
	basics := []string{"bool", "uint8", "uint16", "uint32", "uint64"}
	return FieldDef{
		Name:   name,
		GoType: basics[g.rng.Intn(len(basics))],
	}
}

func (g *Generator) generateByteArrayField(name string) FieldDef {
	sizes := []int{1, 2, 4, 8, 16, 32, 48, 64, 96}
	size := sizes[g.rng.Intn(len(sizes))]
	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[%d]byte", size),
	}
}

func (g *Generator) generateFixedArrayField(name string, depth int) FieldDef {
	arrayLen := 1 + g.rng.Intn(g.cfg.MaxArrayLen)

	if depth+1 >= g.cfg.MaxDepth || g.rng.Intn(3) < 2 {
		basics := []string{"uint8", "uint16", "uint32", "uint64"}
		elemType := basics[g.rng.Intn(len(basics))]
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d]%s", arrayLen, elemType),
		}
	}

	if len(g.refTypes) > 0 && g.rng.Intn(2) == 0 {
		ref := g.refTypes[g.rng.Intn(len(g.refTypes))]
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d]*%s", arrayLen, ref),
		}
	}

	helperType := g.generateType(depth + 1)
	g.types = append(g.types, helperType)
	g.refTypes = append(g.refTypes, helperType.Name)

	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[%d]*%s", arrayLen, helperType.Name),
	}
}

func (g *Generator) generateListField(name string, depth int) FieldDef {
	maxLimit := 1 + g.rng.Intn(g.cfg.MaxListLimit)

	if depth+1 >= g.cfg.MaxDepth || g.rng.Intn(3) < 2 {
		basics := []string{"uint8", "uint16", "uint32", "uint64"}
		elemType := basics[g.rng.Intn(len(basics))]
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[]%s", elemType),
			Tags:   fmt.Sprintf("`ssz-max:\"%d\"`", maxLimit),
		}
	}

	var refName string
	if len(g.refTypes) > 0 && g.rng.Intn(2) == 0 {
		refName = g.refTypes[g.rng.Intn(len(g.refTypes))]
	} else {
		helperType := g.generateType(depth + 1)
		g.types = append(g.types, helperType)
		g.refTypes = append(g.refTypes, helperType.Name)
		refName = helperType.Name
	}

	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[]*%s", refName),
		Tags:   fmt.Sprintf("`ssz-max:\"%d\"`", maxLimit),
	}
}

// generateFixedSizeSliceField generates a slice with ssz-size (treated as vector).
func (g *Generator) generateFixedSizeSliceField(name string) FieldDef {
	size := 1 + g.rng.Intn(g.cfg.MaxArrayLen)
	basics := []string{"uint8", "uint16", "uint32", "uint64"}
	elemType := basics[g.rng.Intn(len(basics))]
	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[]%s", elemType),
		Tags:   fmt.Sprintf("`ssz-size:\"%d\"`", size),
	}
}

func (g *Generator) generateByteListField(name string) FieldDef {
	maxLimit := 1 + g.rng.Intn(g.cfg.MaxListLimit)
	return FieldDef{
		Name:   name,
		GoType: "[]byte",
		Tags:   fmt.Sprintf("`ssz-max:\"%d\"`", maxLimit),
	}
}

// generateFixedByteSliceField generates a []byte with ssz-size (fixed-size byte vector).
func (g *Generator) generateFixedByteSliceField(name string) FieldDef {
	sizes := []int{1, 2, 4, 8, 16, 32, 48, 64, 96}
	size := sizes[g.rng.Intn(len(sizes))]
	return FieldDef{
		Name:   name,
		GoType: "[]byte",
		Tags:   fmt.Sprintf("`ssz-size:\"%d\"`", size),
	}
}

func (g *Generator) generateBitvectorField(name string) FieldDef {
	size := 1 + g.rng.Intn(64)
	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[%d]bool", size),
	}
}

func (g *Generator) generateBitlistField(name string) FieldDef {
	maxBits := 1 + g.rng.Intn(256)
	return FieldDef{
		Name:   name,
		GoType: "[]bool",
		Tags:   fmt.Sprintf("`ssz-max:\"%d\"`", maxBits),
	}
}

func (g *Generator) generateNestedStructField(name string, depth int) FieldDef {
	if len(g.refTypes) > 0 && g.rng.Intn(2) == 0 {
		ref := g.refTypes[g.rng.Intn(len(g.refTypes))]
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("*%s", ref),
		}
	}

	helperType := g.generateType(depth + 1)
	g.types = append(g.types, helperType)
	g.refTypes = append(g.refTypes, helperType.Name)

	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("*%s", helperType.Name),
	}
}

// generateVectorOfStructsField generates a fixed-size array of struct pointers
// with various nesting patterns.
func (g *Generator) generateVectorOfStructsField(name string, depth int) FieldDef {
	arrayLen := 1 + g.rng.Intn(4) // keep small for vectors of structs

	var refName string
	if len(g.refTypes) > 0 && g.rng.Intn(2) == 0 {
		refName = g.refTypes[g.rng.Intn(len(g.refTypes))]
	} else {
		helperType := g.generateType(depth + 1)
		g.types = append(g.types, helperType)
		g.refTypes = append(g.refTypes, helperType.Name)
		refName = helperType.Name
	}

	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[%d]*%s", arrayLen, refName),
	}
}

func (g *Generator) generateExtendedField(name string) FieldDef {
	// 0-3: signed ints, 4-5: floats, 6: big.Int, 7: optional
	roll := g.rng.Intn(8)
	switch roll {
	case 0:
		return FieldDef{Name: name, GoType: "int8"}
	case 1:
		return FieldDef{Name: name, GoType: "int16"}
	case 2:
		return FieldDef{Name: name, GoType: "int32"}
	case 3:
		return FieldDef{Name: name, GoType: "int64"}
	case 4:
		return FieldDef{Name: name, GoType: "float32"}
	case 5:
		return FieldDef{Name: name, GoType: "float64"}
	case 6:
		return FieldDef{
			Name:    name,
			GoType:  "*big.Int",
			Imports: []string{"math/big"},
		}
	default:
		// Optional field - wrap a basic type
		basics := []string{"uint8", "uint16", "uint32", "uint64"}
		elemType := basics[g.rng.Intn(len(basics))]
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("*%s", elemType),
			Tags:   "`ssz-type:\"optional\"`",
		}
	}
}
