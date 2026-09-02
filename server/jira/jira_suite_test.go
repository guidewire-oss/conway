package jira

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestJiraSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "jira suite")
}
