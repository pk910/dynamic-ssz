package sszutils

import "fmt"

var (
	ErrListTooBig    = fmt.Errorf("list length is higher than max value")
	ErrUnexpectedEOF = fmt.Errorf("unexpected end of SSZ")
	ErrOffset        = fmt.Errorf("incorrect offset")
)
