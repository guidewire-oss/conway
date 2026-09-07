//go:build darwin || linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func browserCommand(ctx context.Context, node string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, node, args...) // #nosec G702 -- Test runner path comes from trusted local/CI configuration; arguments are test-owned and no shell is invoked.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 10 * time.Second
	// per specs/011-bootstrap-adoption-debt.md:229
	// Playwright launches Chromium in a separate process group. Capture that
	// ownership before interrupting Node, while parent links still exist.
	cmd.Cancel = func() error {
		processes, inspectErr := browserDescendants(cmd.Process.Pid)
		interruptErr := cmd.Process.Signal(os.Interrupt)
		if errors.Is(interruptErr, os.ErrProcessDone) {
			interruptErr = nil
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && browserProcessesAlive(processes) {
			time.Sleep(25 * time.Millisecond)
		}
		// Refresh while the runner is still alive to include subprocesses
		// created during its graceful shutdown. Retain the original snapshot
		// for descendants whose parent has already exited.
		if browserProcessAlive(cmd.Process.Pid) {
			latest, err := browserDescendants(cmd.Process.Pid)
			inspectErr = errors.Join(inspectErr, err)
			processes = append(processes, latest...)
		}
		groups := map[int]bool{cmd.Process.Pid: true}
		for _, process := range processes {
			groups[process.group] = true
		}
		var cleanupErr error
		for group := range groups {
			if group <= 1 || group == syscall.Getpgrp() {
				continue
			}
			if err := syscall.Kill(-group, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("terminate browser process group %d: %w", group, err))
			}
		}
		return errors.Join(inspectErr, interruptErr, cleanupErr)
	}
	return cmd
}

func browserProcessAlive(pid int) bool {
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func browserProcessesAlive(processes []browserProcess) bool {
	for _, process := range processes {
		if browserProcessAlive(process.pid) {
			return true
		}
	}
	return false
}

type browserProcess struct {
	pid, parent, group int
}

func browserDescendants(root int) ([]browserProcess, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pgid=").Output()
	if err != nil {
		return nil, fmt.Errorf("inspect browser process tree: %w", err)
	}
	var rows []browserProcess
	for line := range strings.SplitSeq(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])
		group, groupErr := strconv.Atoi(fields[2])
		if pidErr != nil || parentErr != nil || groupErr != nil {
			return nil, fmt.Errorf("invalid browser process row: %q", line)
		}
		rows = append(rows, browserProcess{pid, parent, group})
	}
	owned := map[int]bool{root: true}
	for changed := true; changed; {
		changed = false
		for _, row := range rows {
			if owned[row.parent] && !owned[row.pid] {
				owned[row.pid] = true
				changed = true
			}
		}
	}
	var descendants []browserProcess
	for _, row := range rows {
		if owned[row.pid] {
			descendants = append(descendants, row)
		}
	}
	return descendants, nil
}

// per specs/011-bootstrap-adoption-debt.md:229
var _ = Describe("browser process cleanup", Label("browser"), func() {
	// per specs/011-bootstrap-adoption-debt.md:229
	DescribeTable("cancels the runner and its detached browser descendants", func(blockEventLoop bool) {
		if os.Getenv("CONWAY_TEST_BROWSER") != "1" {
			Skip("Set CONWAY_TEST_BROWSER=1 with Playwright; no database is required.")
		}
		node := os.Getenv("CONWAY_TEST_NODE")
		if node == "" {
			node = "node"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := browserCommand(ctx, node, "--input-type=module", "-e", `
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const channel=process.env.PLAYWRIGHT_BROWSER_CHANNEL;
const server=await chromium.launchServer({headless:true,...(channel?{channel}:{})});
const browser=await chromium.connect(server.wsEndpoint());
await browser.newPage();
console.log(JSON.stringify({runner:process.pid,browser:server.process().pid}));
if(process.env.CONWAY_BLOCK_EVENT_LOOP==='1') Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0);
else setInterval(()=>{},1000);
`)
		blocked := "0"
		if blockEventLoop {
			blocked = "1"
		}
		cmd.Env = append(os.Environ(), "CONWAY_BLOCK_EVENT_LOOP="+blocked)
		stdout, err := cmd.StdoutPipe()
		Expect(err).NotTo(HaveOccurred())
		cmd.Stderr = GinkgoWriter
		Expect(cmd.Start()).To(Succeed())
		finished := make(chan error, 1)
		reaped := make(chan struct{})
		go func() {
			finished <- cmd.Wait()
			close(reaped)
		}()
		var browserPID int
		DeferCleanup(func() {
			cancel()
			// Preserve parent links until Cmd.Cancel snapshots the detached
			// browser, including failures before browserPID was recorded.
			select {
			case <-reaped:
				return
			case <-time.After(20 * time.Second):
			}
			if browserPID > 0 {
				_ = syscall.Kill(-browserPID, syscall.SIGKILL)
			}
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			Eventually(reaped, 20*time.Second).Should(BeClosed(), "The launched runner is reaped even if setup or an assertion fails")
		})
		ready := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(stdout)
			if scanner.Scan() {
				ready <- scanner.Text()
			} else {
				ready <- ""
			}
		}()
		var line string
		Eventually(ready, 30*time.Second).Should(Receive(&line), "Chrome must launch before testing cancellation")
		var ids struct{ Runner, Browser int }
		Expect(json.Unmarshal([]byte(line), &ids)).To(Succeed())
		browserPID = ids.Browser
		Expect(ids.Runner).To(Equal(cmd.Process.Pid))
		group, err := syscall.Getpgid(browserPID)
		Expect(err).NotTo(HaveOccurred())
		Expect(group).To(Equal(browserPID), "Playwright actually launches Chrome in a detached group")
		Expect(group).NotTo(Equal(cmd.Process.Pid))
		processes, err := browserDescendants(cmd.Process.Pid)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(processes)).To(BeNumerically(">", 2), "The cancellation must cover browser subprocesses")
		if blockEventLoop {
			cancel()
		}
		var waitErr error
		Eventually(finished, 35*time.Second).Should(Receive(&waitErr))
		Expect(waitErr).To(HaveOccurred())
		if !blockEventLoop {
			Expect(ctx.Err()).To(MatchError(context.DeadlineExceeded), "The responsive scenario exercises an actual workload timeout")
		}
		Eventually(func() []int {
			var alive []int
			for _, process := range processes {
				if syscall.Kill(process.pid, 0) == nil {
					alive = append(alive, process.pid)
				}
			}
			return alive
		}, 5*time.Second).Should(BeEmpty(), "No captured Node, Chromium or renderer process may remain after cancellation")
		GinkgoWriter.Printf("Cancellation removed runner %d, detached browser %d and all %d captured processes (blocked event loop: %t).\n", ids.Runner, browserPID, len(processes), blockEventLoop)
	}, Entry("responsive Playwright runner", false), Entry("unresponsive runner event loop", true))
})
