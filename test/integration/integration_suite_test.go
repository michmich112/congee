package integration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = Describe("Scaffolding", func() {
	It("passes until relay integration specs land in later phases", func() {
		Expect(true).To(BeTrue())
	})
})
