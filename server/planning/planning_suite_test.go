package planning

// The one RunSpecs bootstrap for this package, and so the only file here
// permitted to import "testing" under the Go pack's dialect gate. New
// behavioural tests are Ginkgo specs; the package's older stdlib tests stay as
// they are — see specs/002-factory-adoption.md Q2.

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPlanningSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "planning suite")
}
