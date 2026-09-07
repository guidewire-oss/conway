package main

import (
	"context"
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strings"
)

// specs/027-evidence-linked-planning-assistant.md:226
var _ = Describe("assistant question interpretation", func() {
	It("requests only a bounded task selection and validates exact scope", func() {
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			var body map[string]any
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
			Expect(body["store"]).To(BeFalse())
			Expect(body).NotTo(HaveKey("tools"))
			Expect(body["model"]).To(Equal("configured-model"))
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer fixture-key"))
			_, e := w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"task\":\"schedule\",\"initiative\":\"Beacon\",\"team\":\"Atlas\"}"}]}]}`))
			Expect(e).NotTo(HaveOccurred())
		}))
		defer host.Close()
		model := &assistantInterpreter{endpoint: host.URL, key: "fixture-key", model: "configured-model", client: host.Client(), slots: make(chan struct{}, 4)}
		choice, err := model.interpret(context.Background(), "Why is Beacon held?", []string{"Beacon"}, []string{"Atlas"})
		Expect(err).NotTo(HaveOccurred())
		Expect(choice.Task).To(Equal("schedule"))
		Expect(choice.Initiative).To(Equal("Beacon"))
	})
	It("refuses malformed or unsupported model output without exposing provider bodies", func() {
		for _, body := range []string{`{"status":"incomplete","output":[]}`, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"task\":\"delete\",\"initiative\":\"\",\"team\":\"\"}"}]}]}`, `provider-secret-fixture`} {
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			model := &assistantInterpreter{endpoint: host.URL, key: "fixture-key", model: "configured-model", client: host.Client(), slots: make(chan struct{}, 4)}
			_, err := model.interpret(context.Background(), "question", []string{"Beacon"}, []string{"Atlas"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("provider-secret-fixture"))
			host.Close()
		}
	})
	It("bounds incoming questions and respects cancellation", func() {
		model := &assistantInterpreter{slots: make(chan struct{}, 1)}
		_, err := model.interpret(context.Background(), strings.Repeat("x", 2001), nil, nil)
		Expect(err).To(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = model.interpret(ctx, "question", nil, nil)
		Expect(err).To(HaveOccurred())
	})
})
