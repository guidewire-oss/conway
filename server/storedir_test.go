package main

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The store directory is a pre-Postgres artifact. DATABASE_URL is required and
// Postgres is the only backend, so accounts, the signing secret and the game
// snapshot all live in the database; this directory is read at most twice ever,
// by the one-time legacy imports. Creating it must therefore never be fatal.
//
// It was fatal, and it took down every pod on the dev cluster: the image bakes
// CONWAY_STORE=/data/store.json and the container runs with a read-only root
// filesystem, so `mkdir /data` failed and the process exited over a directory it
// would never write to. Observed 2026-08-25 in namespace conway:
// "create store directory /data: mkdir /data: read-only file system",
// CrashLoopBackOff with 15 restarts while the previous pod kept serving.
var _ = Describe("ensureStoreDir", func() {
	It("creates the directory when it can", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "nested", "store.json")

		Expect(ensureStoreDir(path)).To(Succeed())

		info, err := os.Stat(filepath.Dir(path))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())
		// 0o750, not 0o755: it used to hold the credential store (gosec G301).
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o750)))
	})

	It("reports an unwritable location instead of ending the process", func() {
		root := GinkgoT().TempDir()
		locked := filepath.Join(root, "locked")
		Expect(os.Mkdir(locked, 0o500)).To(Succeed())
		DeferCleanup(func() { _ = os.Chmod(locked, 0o700) }) // so TempDir can clean up

		err := ensureStoreDir(filepath.Join(locked, "sub", "store.json"))

		Expect(err).To(HaveOccurred(), "an unwritable path must be reported, not fatal")
		Expect(err.Error()).To(ContainSubstring(filepath.Join(locked, "sub")),
			"the message must name the directory, so a read-only mount is obvious")
	})

	It("is a no-op for a bare filename, which has no directory to create", func() {
		Expect(ensureStoreDir("store.json")).To(Succeed())
		Expect(ensureStoreDir("")).To(Succeed())
	})

	It("accepts a directory that already exists", func() {
		root := GinkgoT().TempDir()
		Expect(ensureStoreDir(filepath.Join(root, "store.json"))).To(Succeed())
		Expect(ensureStoreDir(filepath.Join(root, "store.json"))).To(Succeed())
	})
})
