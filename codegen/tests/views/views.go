package views

type ViewTypes1_View3 struct {
	F1 uint64
	C1 *ViewTypes1_View3_C1
}

type ViewTypes1_View3_C1 struct {
	F2 []uint64 `ssz-size:"4"`
}
