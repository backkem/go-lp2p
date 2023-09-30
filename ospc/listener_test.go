package ospc

import "testing"

func TestTXTRecordSet(t *testing.T) {
	orig := TXTRecordSet{}
	orig.Set("fp", "foo")
	orig.Set("mv", "bar")
	orig.Set("at", "baz")

	actual := TXTRecordSet{}
	err := actual.FromSlice(orig.ToSlice())
	if err != nil {
		t.Fatal(err)
	}

	actualFp, err := actual.GetOne("fp")
	if err != nil {
		t.Fatal(err)
	}
	if actualFp != "foo" {
		t.Fatalf("wrong fp: %s != foo", actualFp)
	}

	actualMv, err := actual.GetOne("mv")
	if err != nil {
		t.Fatal(err)
	}
	if actualMv != "bar" {
		t.Fatalf("wrong mv: %s != bar", actualFp)
	}

	actualAt, err := actual.GetOne("at")
	if err != nil {
		t.Fatal(err)
	}
	if actualAt != "baz" {
		t.Fatalf("wrong at: %s != baz", actualFp)
	}
}
