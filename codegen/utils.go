package codegen

import (
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

func indentStr(s string, spaces int) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = strings.Repeat("\t", spaces) + lines[i]
		}
	}

	return strings.Join(lines, "\n")
}

func getActiveFields(sourceType *dynssz.TypeDescriptor) []byte {
	// Find the highest ssz-index to determine bitlist size
	maxIndex := uint16(0)
	for _, field := range sourceType.ContainerDesc.Fields {
		if field.SszIndex > maxIndex {
			maxIndex = field.SszIndex
		}
	}

	// Create bitlist with enough bytes to hold maxIndex+1 bits
	bytesNeeded := (int(maxIndex) + 8) / 8 // +7 for rounding up, +1 already included in maxIndex
	activeFields := make([]byte, bytesNeeded)

	// Set most significant bit for length bit.
	i := uint8(1 << (maxIndex % 8))
	activeFields[maxIndex/8] |= i

	// Set bit for each field that has an ssz-index
	for _, field := range sourceType.ContainerDesc.Fields {
		byteIndex := field.SszIndex / 8
		bitIndex := field.SszIndex % 8
		if int(byteIndex) < len(activeFields) {
			activeFields[byteIndex] |= (1 << bitIndex)
		}
	}

	return activeFields
}
