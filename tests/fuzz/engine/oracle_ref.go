package engine

// This is intentionally independent of dynamic-ssz internals. It is a small,
// conservative SSZ implementation used as a fuzzing oracle for classic
// schemas. Unsupported annotations are skipped instead of being guessed at;
// official ethereum/ssz-specs vectors cover progressive types and unions.

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type oracleTag struct {
	size []int
	max  []int
	typ  string
}

func parseOracleTag(tag reflect.StructTag) (oracleTag, bool) {
	result := oracleTag{typ: tag.Get("ssz-type")}
	for _, item := range []struct {
		name string
		dst  *[]int
	}{{"ssz-size", &result.size}, {"ssz-max", &result.max}} {
		value := tag.Get(item.name)
		if value == "" {
			continue
		}
		for _, part := range strings.Split(value, ",") {
			if part == "?" {
				*item.dst = append(*item.dst, -1)
				continue
			}
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 {
				return oracleTag{}, false
			}
			*item.dst = append(*item.dst, n)
		}
	}
	return result, true
}

func (tag oracleTag) shift() oracleTag {
	if len(tag.size) > 0 {
		tag.size = tag.size[1:]
	}
	if len(tag.max) > 0 {
		tag.max = tag.max[1:]
	}
	return tag
}

func oracleSupports(t reflect.Type, rawTag reflect.StructTag) bool {
	if rawTag.Get("dynssz-size") != "" || rawTag.Get("dynssz-max") != "" {
		return false
	}
	tag, ok := parseOracleTag(rawTag)
	if !ok || len(tag.size) > 1 || len(tag.max) > 1 {
		return false
	}
	if tag.typ != "" && tag.typ != "list" && tag.typ != "vector" {
		return false
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Array, reflect.Slice:
		elem := t.Elem()
		if elem.Kind() == reflect.Array || elem.Kind() == reflect.Slice {
			return false
		}
		return oracleSupports(elem, "")
	case reflect.Struct:
		if t.PkgPath() == "time" || t.PkgPath() == "math/big" || t.PkgPath() == "github.com/pk910/dynamic-ssz" {
			return false
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() || field.Tag.Get("ssz-index") != "" || field.Tag.Get("ssz-excluded") != "" || !oracleSupports(field.Type, field.Tag) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func oracleDeref(value reflect.Value) reflect.Value {
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Zero(value.Type().Elem())
		}
		value = value.Elem()
	}
	return value
}

func oracleFixedSize(t reflect.Type, tag oracleTag) (int, bool) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool, reflect.Uint8:
		return 1, true
	case reflect.Uint16:
		return 2, true
	case reflect.Uint32:
		return 4, true
	case reflect.Uint64:
		return 8, true
	case reflect.Array:
		size, ok := oracleFixedSize(t.Elem(), tag.shift())
		return size * t.Len(), ok
	case reflect.Slice:
		if len(tag.size) == 0 || tag.size[0] < 0 {
			return 0, false
		}
		size, ok := oracleFixedSize(t.Elem(), tag.shift())
		return size * tag.size[0], ok
	case reflect.Struct:
		total := 0
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldTag, _ := parseOracleTag(field.Tag)
			size, ok := oracleFixedSize(field.Type, fieldTag)
			if !ok {
				return 0, false
			}
			total += size
		}
		return total, true
	default:
		return 0, false
	}
}

func oracleMarshal(value reflect.Value, tag oracleTag) []byte {
	value = oracleDeref(value)
	switch value.Kind() {
	case reflect.Bool:
		if value.Bool() {
			return []byte{1}
		}
		return []byte{0}
	case reflect.Uint8:
		return []byte{byte(value.Uint())}
	case reflect.Uint16:
		out := make([]byte, 2)
		binary.LittleEndian.PutUint16(out, uint16(value.Uint()))
		return out
	case reflect.Uint32:
		out := make([]byte, 4)
		binary.LittleEndian.PutUint32(out, uint32(value.Uint()))
		return out
	case reflect.Uint64:
		out := make([]byte, 8)
		binary.LittleEndian.PutUint64(out, value.Uint())
		return out
	case reflect.Array, reflect.Slice:
		return oracleMarshalCollection(value, tag)
	case reflect.Struct:
		return oracleMarshalStruct(value)
	default:
		panic("unsupported reference kind: " + value.Kind().String())
	}
}

func oracleMarshalCollection(value reflect.Value, tag oracleTag) []byte {
	elemTag := tag.shift()
	if _, fixed := oracleFixedSize(value.Type().Elem(), elemTag); fixed {
		var out []byte
		for i := 0; i < value.Len(); i++ {
			out = append(out, oracleMarshal(value.Index(i), elemTag)...)
		}
		return out
	}
	return oracleOffsets(value.Len(), func(i int) []byte { return oracleMarshal(value.Index(i), elemTag) })
}

func oracleMarshalStruct(value reflect.Value) []byte {
	t := value.Type()
	fixedLen := 0
	parts := make([][]byte, t.NumField())
	variable := make([]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag, _ := parseOracleTag(field.Tag)
		parts[i] = oracleMarshal(value.Field(i), tag)
		if _, ok := oracleFixedSize(field.Type, tag); ok {
			fixedLen += len(parts[i])
		} else {
			fixedLen += 4
			variable[i] = true
		}
	}
	var head, body []byte
	offset := fixedLen
	for i, part := range parts {
		if !variable[i] {
			head = append(head, part...)
			continue
		}
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, uint32(offset))
		head = append(head, word...)
		body = append(body, part...)
		offset += len(part)
	}
	return append(head, body...)
}

func oracleOffsets(count int, item func(int) []byte) []byte {
	if count == 0 {
		return nil
	}
	parts := make([][]byte, count)
	offset := count * 4
	out := make([]byte, 0, offset)
	for i := 0; i < count; i++ {
		parts[i] = item(i)
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, uint32(offset))
		out = append(out, word...)
		offset += len(parts[i])
	}
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func oracleHTR(value reflect.Value, tag oracleTag) [32]byte {
	value = oracleDeref(value)
	switch value.Kind() {
	case reflect.Bool, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var root [32]byte
		copy(root[:], oracleMarshal(value, tag))
		return root
	case reflect.Array, reflect.Slice:
		return oracleCollectionHTR(value, tag)
	case reflect.Struct:
		roots := make([][32]byte, value.NumField())
		for i := range roots {
			fieldTag, _ := parseOracleTag(value.Type().Field(i).Tag)
			roots[i] = oracleHTR(value.Field(i), fieldTag)
		}
		return oracleMerkleize(roots, len(roots))
	default:
		panic("unsupported reference kind: " + value.Kind().String())
	}
}

func oracleCollectionHTR(value reflect.Value, tag oracleTag) [32]byte {
	elem := value.Type().Elem()
	isVector := value.Kind() == reflect.Array || (len(tag.size) > 0 && tag.size[0] >= 0)
	limit := value.Len()
	if isVector && value.Kind() == reflect.Slice {
		limit = tag.size[0]
	}
	if !isVector && len(tag.max) > 0 {
		limit = tag.max[0]
	}

	var root [32]byte
	if oracleBasic(elem.Kind()) {
		chunks := oracleChunkify(oracleMarshal(value, tag))
		limitChunks := (limit*oracleBasicSize(elem.Kind()) + 31) / 32
		root = oracleMerkleize(chunks, limitChunks)
	} else {
		roots := make([][32]byte, value.Len())
		for i := range roots {
			roots[i] = oracleHTR(value.Index(i), tag.shift())
		}
		root = oracleMerkleize(roots, limit)
	}
	if !isVector {
		root = oracleMixIn(root, uint64(value.Len()))
	}
	return root
}

func oracleBasic(kind reflect.Kind) bool {
	return kind == reflect.Bool || kind == reflect.Uint8 || kind == reflect.Uint16 || kind == reflect.Uint32 || kind == reflect.Uint64
}

func oracleBasicSize(kind reflect.Kind) int {
	switch kind {
	case reflect.Bool, reflect.Uint8:
		return 1
	case reflect.Uint16:
		return 2
	case reflect.Uint32:
		return 4
	case reflect.Uint64:
		return 8
	default:
		return 0
	}
}

func oracleChunkify(data []byte) [][32]byte {
	chunks := make([][32]byte, (len(data)+31)/32)
	for i := range chunks {
		copy(chunks[i][:], data[i*32:min(len(data), (i+1)*32)])
	}
	return chunks
}

func oracleMerkleize(chunks [][32]byte, limit int) [32]byte {
	count := max(len(chunks), limit)
	width := 1
	for width < count {
		width <<= 1
	}
	nodes := make([][32]byte, width)
	copy(nodes, chunks)
	for width > 1 {
		for i := 0; i < width/2; i++ {
			var pair [64]byte
			copy(pair[:32], nodes[2*i][:])
			copy(pair[32:], nodes[2*i+1][:])
			nodes[i] = sha256.Sum256(pair[:])
		}
		width /= 2
	}
	return nodes[0]
}

func oracleMixIn(root [32]byte, length uint64) [32]byte {
	var pair [64]byte
	copy(pair[:32], root[:])
	binary.LittleEndian.PutUint64(pair[32:40], length)
	return sha256.Sum256(pair[:])
}

// referenceCheck returns supported=false when the independent oracle does not
// model the schema. A supported mismatch is always reported.
func referenceCheck(value any, encoded []byte, root [32]byte) (detail string, supported, ok bool) {
	t := reflect.TypeOf(value)
	if !oracleSupports(t, "") {
		return "", false, true
	}
	rv := reflect.ValueOf(value)
	defer func() {
		if recovered := recover(); recovered != nil {
			detail = fmt.Sprintf("reference oracle panic: %v", recovered)
			supported = true
			ok = false
		}
	}()
	wantBytes := oracleMarshal(rv, oracleTag{})
	if !reflect.DeepEqual(wantBytes, encoded) {
		return fmt.Sprintf("marshal: dynssz=%x reference=%x", encoded, wantBytes), true, false
	}
	wantRoot := oracleHTR(rv, oracleTag{})
	if wantRoot != root {
		return fmt.Sprintf("root: dynssz=%x reference=%x", root, wantRoot), true, false
	}
	return "", true, true
}
