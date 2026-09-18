package assert

import (
	"reflect"
	"strings"
	"testing"
)

func Contains(t *testing.T, got string, wantSubstring string) {
	t.Helper()

	if !strings.Contains(got, wantSubstring) {
		t.Fatalf("got %q, want it to contain %q", got, wantSubstring)
	}
}

func Equal(t *testing.T, got any, want any) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func Error(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("got no error, want an error")
	}
}

func NoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("got error %v, want no error", err)
	}
}

func NotContains(t *testing.T, got string, unwantedSubstring string) {
	t.Helper()

	if strings.Contains(got, unwantedSubstring) {
		t.Fatalf("got %q, want it not to contain %q", got, unwantedSubstring)
	}
}

func NotEqual(t *testing.T, got any, unwanted any) {
	t.Helper()

	if reflect.DeepEqual(got, unwanted) {
		t.Fatalf("got %#v, want anything else", got)
	}
}
