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
	Imports []string // package import paths needed
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
	imports := make(map[string]string) // path -> alias (empty = no alias)
	for _, td := range g.types {
		for _, f := range td.Fields {
			for _, imp := range f.Imports {
				if imp == "github.com/pk910/dynamic-ssz" {
					imports[imp] = "dynssz"
				} else {
					imports[imp] = ""
				}
			}
		}
	}

	fmt.Fprintf(&sb, "package %s\n\n", packageName)

	if len(imports) > 0 {
		sb.WriteString("import (\n")
		for path, alias := range imports {
			if alias != "" {
				fmt.Fprintf(&sb, "\t%s \"%s\"\n", alias, path)
			} else {
				fmt.Fprintf(&sb, "\t\"%s\"\n", path)
			}
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
		for _, imp := range f.Imports {
			if imp == "math/big" || imp == "github.com/pk910/dynamic-ssz" {
				usesExtended = true
			}
		}
		fields = append(fields, f)
	}

	return TypeDef{
		Name:     name,
		Fields:   fields,
		Extended: usesExtended || g.cfg.Extended,
	}
}

// generateProgressiveContainer generates a struct type with ssz-index annotations
// (progressive container). Fields have increasing, possibly sparse indices.
func (g *Generator) generateProgressiveContainer(depth int) TypeDef {
	name := fmt.Sprintf("FuzzType%d", g.nextID)
	g.nextID++

	numFields := 2 + g.rng.Intn(g.cfg.MaxFields-1)
	fields := make([]FieldDef, 0, numFields)
	usesExtended := false

	idx := 0
	for i := range numFields {
		f := g.generateLeafField(fmt.Sprintf("Field%d", i))
		// Progressive containers: increasing, possibly sparse indices
		f.Tags = fmt.Sprintf("`ssz-index:\"%d\"`", idx)
		// 40% chance to skip an index (sparse)
		if g.rng.Intn(5) < 2 {
			idx += 1 + g.rng.Intn(3)
		} else {
			idx++
		}
		for _, imp := range f.Imports {
			if imp == "math/big" {
				usesExtended = true
			}
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
	roll := g.rng.Intn(155)

	switch {
	case roll < 25:
		return g.generateBasicField(name)
	case roll < 33:
		return g.generateByteArrayField(name)
	case roll < 43:
		return g.generateFixedArrayField(name, depth)
	case roll < 53:
		return g.generateListField(name, depth)
	case roll < 58:
		return g.generateFixedSizeSliceField(name)
	case roll < 63:
		return g.generateByteListField(name)
	case roll < 68:
		return g.generateBitvectorField(name)
	case roll < 73:
		return g.generateBitlistField(name)
	case roll < 78:
		return g.generateFixedByteSliceField(name)
	case roll < 85:
		return g.generateNestedStructField(name, depth)
	case roll < 92:
		return g.generateVectorOfStructsField(name, depth)
	case roll < 98:
		return g.generateMultiDimArrayField(name, depth)
	case roll < 104:
		return g.generateMultiDimListField(name)
	case roll < 108:
		return g.generateProgressiveListField(name)
	case roll < 112:
		return g.generateProgressiveContainerField(name, depth)
	case roll < 116:
		return g.generateUnionField(name)
	case roll < 120:
		return g.generateTypeWrapperField(name)
	case roll < 124:
		return g.generateExplicitListField(name)
	case roll < 128:
		return g.generateByteBitlistField(name)
	case roll < 132:
		return g.generateProgressiveBitlistField(name)
	case roll < 136:
		return g.generateUint128Field(name)
	case roll < 140:
		return g.generateUint256Field(name)
	case roll < 144:
		return g.generateTimeField(name)
	default:
		if g.cfg.Extended {
			return g.generateExtendedField(name)
		}
		return g.generateBasicField(name)
	}
}

// generateLeafField generates a field type that doesn't reference other structs.
func (g *Generator) generateLeafField(name string) FieldDef {
	roll := g.rng.Intn(130)
	switch {
	case roll < 40:
		return g.generateBasicField(name)
	case roll < 55:
		return g.generateByteArrayField(name)
	case roll < 65:
		return g.generateByteListField(name)
	case roll < 72:
		return g.generateFixedByteSliceField(name)
	case roll < 79:
		return g.generateBitvectorField(name)
	case roll < 86:
		return g.generateBitlistField(name)
	case roll < 93:
		return g.generateFixedSizeSliceField(name)
	case roll < 100:
		return g.generateExplicitListField(name)
	case roll < 105:
		return g.generateByteBitlistField(name)
	case roll < 110:
		return g.generateProgressiveBitlistField(name)
	case roll < 115:
		return g.generateUint128Field(name)
	case roll < 120:
		return g.generateUint256Field(name)
	case roll < 125:
		return g.generateTimeField(name)
	default:
		return g.generateProgressiveListField(name)
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

	// Cap struct array size based on depth to prevent exponential blowup.
	structArrayLen := 1 + g.rng.Intn(g.structArrayMax(depth))

	if len(g.refTypes) > 0 && g.rng.Intn(2) == 0 {
		ref := g.refTypes[g.rng.Intn(len(g.refTypes))]
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d]*%s", structArrayLen, ref),
		}
	}

	helperType := g.generateType(depth + 1)
	g.types = append(g.types, helperType)
	g.refTypes = append(g.refTypes, helperType.Name)

	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[%d]*%s", structArrayLen, helperType.Name),
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

	// Cap struct list limit based on depth to prevent exponential blowup.
	structListLimit := 1 + g.rng.Intn(g.structListMax(depth))

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
		Tags:   fmt.Sprintf("`ssz-max:\"%d\"`", structListLimit),
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

// generateVectorOfStructsField generates a fixed-size array of struct pointers.
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

// generateMultiDimArrayField generates multi-dimensional arrays like [4][8]uint32,
// [2][]byte with ssz-size:"2,32", or mixed array/list like [N][]T with ssz-size:"N,?".
func (g *Generator) generateMultiDimArrayField(name string, depth int) FieldDef {
	outerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 8))

	basics := []string{"uint8", "uint16", "uint32", "uint64"}
	elemType := basics[g.rng.Intn(len(basics))]

	roll := g.rng.Intn(100)
	switch {
	case roll < 25:
		// [N][M]T - fully fixed 2D array (vector of vectors)
		innerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 16))
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d][%d]%s", outerLen, innerLen, elemType),
		}
	case roll < 45:
		// [N][]T with ssz-size:"N,M" - fixed outer, fixed inner via tag (vector of vectors)
		innerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 16))
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d][]%s", outerLen, elemType),
			Tags:   fmt.Sprintf("`ssz-size:\"%d,%d\"`", outerLen, innerLen),
		}
	case roll < 65:
		// [N][]T with ssz-size:"N,?" ssz-max:"?,M" - mixed: array of lists
		innerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d][]%s", outerLen, elemType),
			Tags:   fmt.Sprintf("`ssz-size:\"%d,?\" ssz-max:\"?,%d\"`", outerLen, innerMax),
		}
	case roll < 80:
		// [][]T with ssz-max:"N,M" - variable outer and inner (list of lists)
		outerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		innerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[][]%s", elemType),
			Tags:   fmt.Sprintf("`ssz-max:\"%d,%d\"`", outerMax, innerMax),
		}
	default:
		// [][]T with ssz-size:"?,?" ssz-max:"N,M" - explicit dynamic sizes (list of lists)
		outerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		innerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[][]%s", elemType),
			Tags:   fmt.Sprintf("`ssz-size:\"?,?\" ssz-max:\"%d,%d\"`", outerMax, innerMax),
		}
	}
}

// generateMultiDimListField generates multi-dimensional lists/byte-matrices,
// including mixed array/list variants with ssz-size:"N,?" tags.
func (g *Generator) generateMultiDimListField(name string) FieldDef {
	roll := g.rng.Intn(100)
	switch {
	case roll < 25:
		// [][]byte with ssz-size:"N,M" - fixed dimensions via tag (vector of vectors)
		outerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 8))
		innerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 32))
		return FieldDef{
			Name:   name,
			GoType: "[][]byte",
			Tags:   fmt.Sprintf("`ssz-size:\"%d,%d\"`", outerLen, innerLen),
		}
	case roll < 45:
		// [][]byte with ssz-max:"N,M" - variable dimensions (list of lists)
		outerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		innerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 32))
		return FieldDef{
			Name:   name,
			GoType: "[][]byte",
			Tags:   fmt.Sprintf("`ssz-max:\"%d,%d\"`", outerMax, innerMax),
		}
	case roll < 60:
		// [N][]byte with ssz-size:"N,M" - fixed outer, fixed inner via tag
		outerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 8))
		innerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 32))
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d][]byte", outerLen),
			Tags:   fmt.Sprintf("`ssz-size:\"%d,%d\"`", outerLen, innerLen),
		}
	case roll < 75:
		// [N][]byte with ssz-size:"N,?" ssz-max:"?,M" - mixed: array of byte lists
		outerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 8))
		innerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 32))
		return FieldDef{
			Name:   name,
			GoType: fmt.Sprintf("[%d][]byte", outerLen),
			Tags:   fmt.Sprintf("`ssz-size:\"%d,?\" ssz-max:\"?,%d\"`", outerLen, innerMax),
		}
	case roll < 88:
		// [][]byte with ssz-size:"?,?" ssz-max:"N,M" - explicit dynamic (list of byte lists)
		outerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		innerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 32))
		return FieldDef{
			Name:   name,
			GoType: "[][]byte",
			Tags:   fmt.Sprintf("`ssz-size:\"?,?\" ssz-max:\"%d,%d\"`", outerMax, innerMax),
		}
	default:
		// [][]byte with ssz-size:"N,?" ssz-max:"?,M" - fixed outer count, dynamic inner
		outerLen := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 8))
		innerMax := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 32))
		return FieldDef{
			Name:   name,
			GoType: "[][]byte",
			Tags:   fmt.Sprintf("`ssz-size:\"%d,?\" ssz-max:\"?,%d\"`", outerLen, innerMax),
		}
	}
}

// generateProgressiveListField generates a list with ssz-type:"progressive-list".
func (g *Generator) generateProgressiveListField(name string) FieldDef {
	maxLimit := 1 + g.rng.Intn(g.cfg.MaxListLimit)

	basics := []string{"uint8", "uint16", "uint32", "uint64"}
	elemType := basics[g.rng.Intn(len(basics))]

	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[]%s", elemType),
		Tags:   fmt.Sprintf("`ssz-max:\"%d\" ssz-type:\"progressive-list\"`", maxLimit),
	}
}

// generateExplicitListField generates a slice with explicit ssz-type:"list" annotation.
// Lists use ssz-max (not ssz-size) to specify the maximum length.
func (g *Generator) generateExplicitListField(name string) FieldDef {
	basics := []string{"uint8", "uint16", "uint32", "uint64"}
	elemType := basics[g.rng.Intn(len(basics))]

	// []T with ssz-type:"list" ssz-size:"?" ssz-max:"N"
	maxLimit := 1 + g.rng.Intn(g.cfg.MaxListLimit)
	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("[]%s", elemType),
		Tags:   fmt.Sprintf("`ssz-type:\"list\" ssz-size:\"?\" ssz-max:\"%d\"`", maxLimit),
	}
}

// generateByteBitlistField generates a []byte bitlist with ssz-type:"bitlist".
func (g *Generator) generateByteBitlistField(name string) FieldDef {
	maxBits := 1 + g.rng.Intn(256)
	return FieldDef{
		Name:   name,
		GoType: "[]byte",
		Tags:   fmt.Sprintf("`ssz-type:\"bitlist\" ssz-max:\"%d\"`", maxBits),
	}
}

// generateProgressiveBitlistField generates a []byte progressive-bitlist.
func (g *Generator) generateProgressiveBitlistField(name string) FieldDef {
	maxBits := 1 + g.rng.Intn(256)
	return FieldDef{
		Name:   name,
		GoType: "[]byte",
		Tags:   fmt.Sprintf("`ssz-type:\"progressive-bitlist\" ssz-max:\"%d\"`", maxBits),
	}
}

// generateUint128Field generates a uint128 field ([16]byte or [2]uint64 with ssz-type:"uint128").
func (g *Generator) generateUint128Field(name string) FieldDef {
	if g.rng.Intn(2) == 0 {
		return FieldDef{
			Name:   name,
			GoType: "[16]byte",
			Tags:   "`ssz-type:\"uint128\"`",
		}
	}
	return FieldDef{
		Name:   name,
		GoType: "[2]uint64",
		Tags:   "`ssz-type:\"uint128\"`",
	}
}

// generateUint256Field generates a uint256 field ([32]byte or [4]uint64 with ssz-type:"uint256").
func (g *Generator) generateUint256Field(name string) FieldDef {
	if g.rng.Intn(2) == 0 {
		return FieldDef{
			Name:   name,
			GoType: "[32]byte",
			Tags:   "`ssz-type:\"uint256\"`",
		}
	}
	return FieldDef{
		Name:   name,
		GoType: "[4]uint64",
		Tags:   "`ssz-type:\"uint256\"`",
	}
}

// generateTimeField generates a time.Time field (SSZ-encoded as uint64).
func (g *Generator) generateTimeField(name string) FieldDef {
	return FieldDef{
		Name:    name,
		GoType:  "time.Time",
		Imports: []string{"time"},
	}
}

// generateProgressiveContainerField generates an inline progressive container
// (struct with ssz-index tags) or a reference to a generated progressive container type.
func (g *Generator) generateProgressiveContainerField(name string, depth int) FieldDef {
	// Generate a progressive container type and reference it
	helperType := g.generateProgressiveContainer(depth + 1)
	g.types = append(g.types, helperType)
	g.refTypes = append(g.refTypes, helperType.Name)

	return FieldDef{
		Name:   name,
		GoType: fmt.Sprintf("*%s", helperType.Name),
	}
}

// generateUnionField generates a CompatibleUnion field.
func (g *Generator) generateUnionField(name string) FieldDef {
	numVariants := 2 + g.rng.Intn(3) // 2-4 variants

	var variants []string
	for i := range numVariants {
		variantType := g.randomBasicOrByteType()
		variants = append(variants, fmt.Sprintf("\t\tVariant%d %s", i, variantType))
	}

	descriptorStruct := fmt.Sprintf("struct {\n%s\n\t}", strings.Join(variants, "\n"))

	return FieldDef{
		Name:    name,
		GoType:  fmt.Sprintf("dynssz.CompatibleUnion[%s]", descriptorStruct),
		Imports: []string{"github.com/pk910/dynamic-ssz"},
	}
}

// generateTypeWrapperField generates a TypeWrapper field.
func (g *Generator) generateTypeWrapperField(name string) FieldDef {
	roll := g.rng.Intn(100)

	var dataType, descriptorTag string

	switch {
	case roll < 40:
		// []byte with ssz-size
		size := []int{4, 8, 16, 32, 48, 64}[g.rng.Intn(6)]
		dataType = "[]byte"
		descriptorTag = fmt.Sprintf("`ssz-size:\"%d\"`", size)
	case roll < 70:
		// []byte with ssz-max
		maxSize := 1 + g.rng.Intn(g.cfg.MaxListLimit)
		dataType = "[]byte"
		descriptorTag = fmt.Sprintf("`ssz-max:\"%d\"`", maxSize)
	case roll < 85:
		// []uint16 with ssz-size
		size := 1 + g.rng.Intn(min(g.cfg.MaxArrayLen, 8))
		dataType = "[]uint16"
		descriptorTag = fmt.Sprintf("`ssz-size:\"%d\"`", size)
	default:
		// []uint32 with ssz-max
		maxSize := 1 + g.rng.Intn(min(g.cfg.MaxListLimit, 16))
		dataType = "[]uint32"
		descriptorTag = fmt.Sprintf("`ssz-max:\"%d\"`", maxSize)
	}

	descriptorStruct := fmt.Sprintf("struct {\n\t\tData %s %s\n\t}", dataType, descriptorTag)

	return FieldDef{
		Name:    name,
		GoType:  fmt.Sprintf("dynssz.TypeWrapper[%s, %s]", descriptorStruct, dataType),
		Tags:    "`ssz-type:\"wrapper\"`",
		Imports: []string{"github.com/pk910/dynamic-ssz"},
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

// randomBasicOrByteType returns a random basic SSZ type or small byte array.
func (g *Generator) randomBasicOrByteType() string {
	types := []string{
		"uint8", "uint16", "uint32", "uint64",
		"[4]byte", "[8]byte", "[16]byte", "[32]byte",
		"bool",
	}
	return types[g.rng.Intn(len(types))]
}

// structArrayMax returns the max array length for struct element arrays at a given depth.
func (g *Generator) structArrayMax(depth int) int {
	m := g.cfg.MaxArrayLen
	if depth >= 2 {
		m = 4
	} else if depth >= 1 {
		m = 8
	}
	if m > g.cfg.MaxArrayLen {
		m = g.cfg.MaxArrayLen
	}
	return m
}

// structListMax returns the max list limit for struct element lists at a given depth.
func (g *Generator) structListMax(depth int) int {
	m := g.cfg.MaxListLimit
	if depth >= 2 {
		m = 8
	} else if depth >= 1 {
		m = 16
	}
	if m > g.cfg.MaxListLimit {
		m = g.cfg.MaxListLimit
	}
	return m
}
