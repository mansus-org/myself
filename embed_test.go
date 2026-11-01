package myself

import (
	"bytes"
	"os"
	"testing"
)

// The landing page plays its own copy of the demo cast; it must match demo/.
func TestSiteCastMatchesDemo(t *testing.T) {
	want, err := os.ReadFile("demo/demo.cast")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Site.ReadFile("site/assets/demo.cast")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("site/assets/demo.cast differs from demo/demo.cast: cp demo/demo.cast site/assets/demo.cast and update the transcript in site/index.html")
	}
}
