package certlib

import (
	"errors"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// ErrRecursive marks a descriptor that refers to itself: a certificate is a
// finite term, so a recursive type has none (its depth-bounded unrollings do;
// the harness declares those per value).
var ErrRecursive = errors.New("recursive type has no finite declaration")

// isRecursive reports whether the descriptor graph has a cycle.
func isRecursive(desc *ssztypes.TypeDescriptor) bool {
	onPath := map[*ssztypes.TypeDescriptor]bool{}
	var walk func(d *ssztypes.TypeDescriptor) bool
	walk = func(d *ssztypes.TypeDescriptor) bool {
		if d == nil {
			return false
		}
		if onPath[d] {
			return true
		}
		onPath[d] = true
		defer delete(onPath, d)
		if walk(d.ElemDesc) {
			return true
		}
		if d.ContainerDesc != nil {
			for i := range d.ContainerDesc.Fields {
				if walk(d.ContainerDesc.Fields[i].Type) {
					return true
				}
			}
		}
		for _, v := range d.UnionVariants {
			if walk(v) {
				return true
			}
		}
		return false
	}
	return walk(desc)
}
