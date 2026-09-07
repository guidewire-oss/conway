package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per specs/011-bootstrap-adoption-debt.md:58
// per specs/011-bootstrap-adoption-debt.md:67
// per specs/011-bootstrap-adoption-debt.md:73
// per specs/011-bootstrap-adoption-debt.md:77
// per specs/011-bootstrap-adoption-debt.md:82
var _ = Describe("Bootstrap adoption browser", Label("browser"), func() {
	// per specs/011-bootstrap-adoption-debt.md:73
	// per specs/011-bootstrap-adoption-debt.md:77
	// per specs/011-bootstrap-adoption-debt.md:82
	// per specs/012-in-app-usage-guide.md:228
	It("retains operational drafts and accessible controls through dynamic rendering and theme changes", func() {
		if os.Getenv("CONWAY_TEST_BROWSER") != "1" {
			Skip("Set CONWAY_TEST_BROWSER=1 with Playwright; no database is required.")
		}
		app, err := filepath.Abs("../app")
		Expect(err).NotTo(HaveOccurred())
		mux := http.NewServeMux()
		mux.HandleFunc("/bootstrap-acceptance", func(w http.ResponseWriter, _ *http.Request) {
			defer GinkgoRecover()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, writeErr := w.Write([]byte(`<!doctype html><html lang="en" data-bs-theme="dark"><head>
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="/vendor/bootstrap/bootstrap.min.css">
<link rel="stylesheet" href="/css/conway.css"><link rel="stylesheet" href="/css/style.css">
<link rel="stylesheet" href="/css/planning-ux.css"><link rel="stylesheet" href="/css/execution.css">
<link rel="stylesheet" href="/css/readyqueue.css"></head><body>
<main class="container-fluid p-3"><div id="ready"></div><div id="evidence" class="execution-review"></div><div id="timeline"></div><div id="dynamic"></div><div id="badge-examples" class="d-flex flex-column gap-2"></div></main>
<script src="/vendor/d3.min.js"></script><script src="/vendor/bootstrap/bootstrap.bundle.min.js"></script></body></html>`))
			Expect(writeErr).NotTo(HaveOccurred())
		})
		mux.Handle("/", http.FileServer(http.Dir(app)))
		host := httptest.NewServer(mux)
		DeferCleanup(host.Close)
		node := os.Getenv("CONWAY_TEST_NODE")
		if node == "" {
			node = "node"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		script, err := filepath.Abs("../tests/browser/bootstrap-adoption.mjs")
		Expect(err).NotTo(HaveOccurred())
		cmd := browserCommand(ctx, node, script)
		cmd.Env = append(os.Environ(), "CONWAY_TEST_BASE_URL="+host.URL)
		output, err := cmd.CombinedOutput()
		GinkgoWriter.Printf("%s", output)
		Expect(ctx.Err()).NotTo(HaveOccurred(), "Bootstrap browser workload exceeded its three-minute execution limit; inspect browser output before attributing this to a product assertion.")
		Expect(err).NotTo(HaveOccurred(), "%s", output)
	})
})
