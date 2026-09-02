package logging

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The one RunSpecs bootstrap this package is permitted to have (ADR-0002).
func TestLoggingSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "logging suite")
}
