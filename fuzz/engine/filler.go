package engine

import (
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
)

// Filler populates Go struct instances with random valid values using reflection.
// It reads SSZ struct tags to determine valid sizes for slices, bitlists, etc.
type Filler struct {
	rng         *rand.Rand
	nilChance   float64 // probability of leaving an optional/struct pointer nil
	emptyChance float64 // probability of empty slices
	maxListFill int     // cap on list elements (may be less than ssz-max)
}

// NewFiller creates a new random value filler.
func NewFiller(rng *rand.Rand) *Filler {
	return &Filler{
		rng:         rng,
		nilChance:   0.15,
		emptyChance: 0.10,
		maxListFill: 16,
	}
}

// Fill populates the value v with random data appropriate to its type.
// v must be settable (typically obtained via reflect.ValueOf(&x).Elem()).
func (f *Filler) Fill(v reflect.Value) {
	f.fillValue(v, "")
}

// FillStruct fills a struct pointer with random values.
func (f *Filler) FillStruct(target any) {
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	f.fillValue(v, "")
}

func (f *Filler) fillValue(v reflect.Value, tags string) {
	if !v.CanSet() {
		return
	}

	switch v.Kind() {
	case reflect.Ptr:
		f.fillPointer(v, tags)
	case reflect.Struct:
		f.fillStructFields(v)
	case reflect.Array:
		f.fillArray(v)
	case reflect.Slice:
		f.fillSlice(v, tags)
	case reflect.Bool:
		v.SetBool(f.rng.Intn(2) == 1)
	case reflect.Uint8:
		v.SetUint(uint64(f.rng.Intn(256)))
	case reflect.Uint16:
		v.SetUint(uint64(f.rng.Intn(65536)))
	case reflect.Uint32:
		v.SetUint(uint64(f.rng.Uint32()))
	case reflect.Uint64:
		v.SetUint(f.rng.Uint64())
	case reflect.Int8:
		v.SetInt(int64(f.rng.Intn(256) - 128))
	case reflect.Int16:
		v.SetInt(int64(f.rng.Intn(65536) - 32768))
	case reflect.Int32:
		v.SetInt(int64(f.rng.Int31()))
	case reflect.Int64:
		v.SetInt(f.rng.Int63())
	case reflect.Float32:
		v.SetFloat(float64(f.randomFloat32()))
	case reflect.Float64:
		v.SetFloat(f.randomFloat64())
	}
}

func (f *Filler) fillPointer(v reflect.Value, tags string) {
	isOptional := strings.Contains(tags, `ssz-type:"optional"`)

	// For optional types, higher chance of nil
	if isOptional && f.rng.Float64() < f.nilChance*2 {
		return // leave nil
	}

	// For struct pointers (nested containers), small chance of nil
	elemType := v.Type().Elem()
	if elemType.Kind() == reflect.Struct && f.rng.Float64() < f.nilChance {
		// Actually, SSZ struct pointers should generally not be nil
		// unless optional. Don't leave nil for non-optional structs.
	}

	// Allocate and fill
	if v.IsNil() {
		v.Set(reflect.New(elemType))
	}

	f.fillValue(v.Elem(), tags)
}

func (f *Filler) fillStructFields(v reflect.Value) {
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldTags := string(field.Tag)
		f.fillValue(v.Field(i), fieldTags)
	}
}

func (f *Filler) fillArray(v reflect.Value) {
	elemKind := v.Type().Elem().Kind()

	if elemKind == reflect.Uint8 {
		// Byte array - fill with random bytes
		for i := range v.Len() {
			v.Index(i).SetUint(uint64(f.rng.Intn(256)))
		}
		return
	}

	if elemKind == reflect.Bool {
		// Bitvector - fill with random bits
		for i := range v.Len() {
			v.Index(i).SetBool(f.rng.Intn(2) == 1)
		}
		return
	}

	// Array of other types
	for i := range v.Len() {
		f.fillValue(v.Index(i), "")
	}
}

func (f *Filler) fillSlice(v reflect.Value, tags string) {
	elemKind := v.Type().Elem().Kind()

	// Determine max from ssz-max or ssz-size tag
	maxLen := f.parseMaxFromTags(tags)
	if maxLen == 0 {
		maxLen = 16 // fallback
	}

	// Check if fixed size via ssz-size
	fixedLen := f.parseSizeFromTags(tags)

	if elemKind == reflect.Bool {
		// Bitlist - max is in bits
		return // leave nil/empty for now, bitlists are complex
	}

	// Determine actual length
	var length int
	if fixedLen > 0 {
		length = fixedLen
	} else if f.rng.Float64() < f.emptyChance {
		length = 0
	} else {
		// Cap at maxListFill for performance
		capMax := maxLen
		if capMax > f.maxListFill {
			capMax = f.maxListFill
		}
		length = f.rng.Intn(capMax + 1)
	}

	if length == 0 {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		return
	}

	slice := reflect.MakeSlice(v.Type(), length, length)

	if elemKind == reflect.Uint8 {
		// Byte slice - fill with random bytes
		for i := range length {
			slice.Index(i).SetUint(uint64(f.rng.Intn(256)))
		}
	} else {
		for i := range length {
			f.fillValue(slice.Index(i), "")
		}
	}

	v.Set(slice)
}

func (f *Filler) parseMaxFromTags(tags string) int {
	return f.parseIntTag(tags, "ssz-max")
}

func (f *Filler) parseSizeFromTags(tags string) int {
	return f.parseIntTag(tags, "ssz-size")
}

func (f *Filler) parseIntTag(tags, key string) int {
	// Look for key:"value" in the tag string
	idx := strings.Index(tags, key+":\"")
	if idx < 0 {
		return 0
	}
	start := idx + len(key) + 2 // skip key:"
	end := strings.Index(tags[start:], "\"")
	if end < 0 {
		return 0
	}
	valStr := tags[start : start+end]
	// Handle comma-separated values (e.g., "32,32") - take first
	if commaIdx := strings.Index(valStr, ","); commaIdx >= 0 {
		valStr = valStr[:commaIdx]
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0
	}
	return val
}

func (f *Filler) randomFloat32() float32 {
	// Mix of interesting values
	roll := f.rng.Intn(10)
	switch roll {
	case 0:
		return 0
	case 1:
		return 1.0
	case 2:
		return -1.0
	case 3:
		return float32(math.SmallestNonzeroFloat32)
	case 4:
		return float32(math.MaxFloat32)
	default:
		return f.rng.Float32()*200 - 100
	}
}

func (f *Filler) randomFloat64() float64 {
	roll := f.rng.Intn(10)
	switch roll {
	case 0:
		return 0
	case 1:
		return 1.0
	case 2:
		return -1.0
	case 3:
		return math.SmallestNonzeroFloat64
	case 4:
		return math.MaxFloat64
	default:
		return f.rng.Float64()*200 - 100
	}
}
