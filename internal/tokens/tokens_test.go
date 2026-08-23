package tokens

import (
	"reflect"
	"testing"
)

func TestSplitIdentifier(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"foo", []string{"foo"}},
		{"foo_bar", []string{"foo_bar", "foo", "bar"}},
		{"FooBar", []string{"foobar", "foo", "bar"}},
		{"XMLParser", []string{"xmlparser", "xml", "parser"}},
		{"getHTTPResponse", []string{"gethttpresponse", "get", "http", "response"}},
	}
	for _, tc := range cases {
		got := SplitIdentifier(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SplitIdentifier(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestTokenize(t *testing.T) {
	got := Tokenize("hello FooBar world")
	want := []string{"hello", "foobar", "foo", "bar", "world"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize=%v want %v", got, want)
	}
}
