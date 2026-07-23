package statusmerge

import "testing"

type mergeStatus struct {
	Pointer *mergeNested
	Name    string
	Items   []mergeItem
}

type mergeNested struct {
	Value string
}

type mergeItem struct {
	Field string
}

type mergeObject struct {
	Status mergeStatus
}

func TestMergeNamedStructField_OracleRestartStyle(t *testing.T) {
	dst := &mergeObject{
		Status: mergeStatus{
			Name:  "",
			Items: []mergeItem{{Field: "dst"}},
		},
	}
	src := &mergeObject{
		Status: mergeStatus{
			Pointer: &mergeNested{Value: "src"},
			Name:    "src-name",
			Items:   []mergeItem{{Field: "src"}},
		},
	}

	err := MergeNamedStructField(dst, src, "Status", Options{
		PointerMode: PointerCopyIfNil,
		SliceMode:   SliceReplace,
	})
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if dst.Status.Pointer == nil || dst.Status.Pointer.Value != "src" {
		t.Fatalf("expected pointer copied from src, got %#v", dst.Status.Pointer)
	}
	if dst.Status.Name != "src-name" {
		t.Fatalf("expected name to be set from src, got %q", dst.Status.Name)
	}
	if len(dst.Status.Items) != 1 || dst.Status.Items[0].Field != "src" {
		t.Fatalf("expected slice to be replaced from src, got %#v", dst.Status.Items)
	}
}

func TestMergeNamedStructField_RacStyle(t *testing.T) {
	dst := &mergeObject{
		Status: mergeStatus{
			Pointer: &mergeNested{Value: ""},
			Name:    "",
			Items:   []mergeItem{{Field: "dst0"}},
		},
	}
	src := &mergeObject{
		Status: mergeStatus{
			Pointer: &mergeNested{Value: "src"},
			Name:    "src-name",
			Items:   []mergeItem{{Field: "src0"}, {Field: "src1"}},
		},
	}

	err := MergeNamedStructField(dst, src, "Status", Options{
		PointerMode: PointerDeepMerge,
		SliceMode:   SliceMergeByIndex,
	})
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if dst.Status.Pointer == nil || dst.Status.Pointer.Value != "src" {
		t.Fatalf("expected pointer deep-merge value from src, got %#v", dst.Status.Pointer)
	}
	if dst.Status.Name != "src-name" {
		t.Fatalf("expected name to be set from src, got %q", dst.Status.Name)
	}
	if len(dst.Status.Items) != 2 {
		t.Fatalf("expected slice merge to append missing elements, got %#v", dst.Status.Items)
	}
	if dst.Status.Items[0].Field != "dst0" {
		t.Fatalf("expected existing first item to be preserved for struct merge-by-index, got %#v", dst.Status.Items)
	}
	if dst.Status.Items[1].Field != "src1" {
		t.Fatalf("expected second item appended from src, got %#v", dst.Status.Items)
	}
}

func TestMergeNamedStructField_SkipSliceField(t *testing.T) {
	dst := &mergeObject{
		Status: mergeStatus{
			Items: []mergeItem{{Field: "dst"}},
		},
	}
	src := &mergeObject{
		Status: mergeStatus{
			Items: []mergeItem{{Field: "src"}},
		},
	}

	err := MergeNamedStructField(dst, src, "Status", Options{
		PointerMode: PointerCopyIfNil,
		SliceMode:   SliceReplace,
		SkipSliceFields: map[string]struct{}{
			"Items": {},
		},
	})
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if len(dst.Status.Items) != 1 || dst.Status.Items[0].Field != "dst" {
		t.Fatalf("expected skipped slice to remain unchanged, got %#v", dst.Status.Items)
	}
}
