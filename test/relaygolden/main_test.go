package relaygolden

import (
	"flag"
	"os"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden fixtures")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}
