package main

// The one RunSpecs bootstrap for package main, and so the only file here
// permitted to import "testing" under the Go pack's dialect gate. The package's
// older stdlib tests stay as they are — see specs/002-factory-adoption.md Q2.

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "server suite")
}
