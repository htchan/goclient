package circuitbreaker

import (
	"flag"
	"os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	leak := flag.Bool("leak", false, "check for goroutine leaks")
	flag.Parse()

	if *leak {
		goleak.VerifyTestMain(m)
	} else {
		os.Exit(m.Run())
	}
}
