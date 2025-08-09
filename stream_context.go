package dynssz

import (
	"io"
)

const defaultBufferSize = 1024

// dynamicSizeNode represents a node in the dynamic size tree
type dynamicSizeNode struct {
	size     uint32
	children []*dynamicSizeNode
	index    int // current child index for traversal
}

// marshalWriterContext holds context for streaming marshal operations
type marshalWriterContext struct {
	writer      *limitedWriter
	buffer      []byte
	sizeTree    *dynamicSizeNode
	currentNode *dynamicSizeNode
}

// newMarshalWriterContext creates a new marshal writer context
func newMarshalWriterContext(w *limitedWriter, bufSize int) *marshalWriterContext {
	if bufSize <= 0 {
		bufSize = defaultBufferSize
	}
	return &marshalWriterContext{
		writer: w,
		buffer: make([]byte, 0, bufSize),
	}
}

// setSizeTree sets the dynamic size tree for offset calculation
func (ctx *marshalWriterContext) setSizeTree(tree *dynamicSizeNode) {
	ctx.sizeTree = tree
	ctx.currentNode = tree
}

// enterDynamicField enters a dynamic field in the size tree
func (ctx *marshalWriterContext) enterDynamicField() *dynamicSizeNode {
	if ctx.currentNode != nil && ctx.currentNode.index < len(ctx.currentNode.children) {
		child := ctx.currentNode.children[ctx.currentNode.index]
		ctx.currentNode.index++
		ctx.currentNode = child
		return child
	}
	return nil
}

// exitDynamicField exits the current dynamic field in the size tree
func (ctx *marshalWriterContext) exitDynamicField(parent *dynamicSizeNode) {
	ctx.currentNode = parent
}

// getChildSize returns the size of the child at the given index
func (ctx *marshalWriterContext) getChildSize(index int) (uint32, bool) {
	if ctx.currentNode != nil && index < len(ctx.currentNode.children) {
		return ctx.currentNode.children[index].size, true
	}
	return 0, false
}

// unmarshalReaderContext holds context for streaming unmarshal operations
type unmarshalReaderContext struct {
	buffer        []byte
	limitedReader *limitedReader
}

// newUnmarshalReaderContext creates a new unmarshal reader context
func newUnmarshalReaderContext(reader io.Reader, bufSize int) *unmarshalReaderContext {
	if bufSize <= 0 {
		bufSize = defaultBufferSize
	}
	return &unmarshalReaderContext{
		buffer:        make([]byte, bufSize),
		limitedReader: newLimitedReader(reader),
	}
}
