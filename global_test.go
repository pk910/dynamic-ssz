package dynssz

import (
	"reflect"
	"testing"
)

// Test SetGlobalSpecs function
func TestSetGlobalSpecs(t *testing.T) {
	// Reset global state after test
	defer func() {
		SetGlobalSpecs(nil)
	}()

	t.Run("SetGlobalSpecs creates new DynSsz with specs", func(t *testing.T) {
		specs := map[string]any{
			"MAX_VALIDATORS": float64(65536),
			"MAX_COMMITTEES": float64(64),
		}

		SetGlobalSpecs(specs)

		ds := GetGlobalDynSsz()
		if ds == nil {
			t.Fatal("expected GetGlobalDynSsz to return non-nil")
		}

		// Verify specs are available by testing with a type that uses them
		type TestStruct struct {
			Data []byte `ssz-max:"100" dynssz-max:"MAX_VALIDATORS"`
		}

		desc, err := ds.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
			t.Fatal("expected container with fields")
		}

		field := desc.ContainerDesc.Fields[0]
		if field.Type.Limit != 65536 {
			t.Errorf("expected Limit 65536 from specs, got %d", field.Type.Limit)
		}
	})

	t.Run("SetGlobalSpecs replaces existing DynSsz", func(t *testing.T) {
		specs1 := map[string]any{
			"TEST_VALUE": float64(100),
		}
		SetGlobalSpecs(specs1)
		ds1 := GetGlobalDynSsz()

		specs2 := map[string]any{
			"TEST_VALUE": float64(200),
		}
		SetGlobalSpecs(specs2)
		ds2 := GetGlobalDynSsz()

		if ds1 == ds2 {
			t.Error("expected SetGlobalSpecs to create a new DynSsz instance")
		}
	})

	t.Run("SetGlobalSpecs with nil specs", func(t *testing.T) {
		SetGlobalSpecs(nil)

		ds := GetGlobalDynSsz()
		if ds == nil {
			t.Fatal("expected GetGlobalDynSsz to return non-nil even with nil specs")
		}
	})
}
