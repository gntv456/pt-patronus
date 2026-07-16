package models

import (
	"encoding/json"
	"testing"
)

func TestSliceTypesMarshalEmptyAsArrays(t *testing.T) {
	for name, value := range map[string]any{
		"nil int slice":      IntSlice(nil),
		"empty int slice":    IntSlice{},
		"nil string slice":   StringSlice(nil),
		"empty string slice": StringSlice{},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "[]" {
				t.Fatalf("json = %s, want []", got)
			}
		})
	}
}

func TestSliceTypesMarshalValues(t *testing.T) {
	cases := map[string]struct {
		value any
		want  string
	}{
		"int slice":    {IntSlice{1, 2}, `[1,2]`},
		"string slice": {StringSlice{"free", "2x"}, `["free","2x"]`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("json = %s, want %s", got, tc.want)
			}
		})
	}
}
