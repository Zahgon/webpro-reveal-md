package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// runSelfCheck is the always-on half of this test: without the reference
// implementation installed the outputs cannot be compared, but every fixture
// must still export a complete static site, which exercises the same code the
// differential run drives.
func runSelfCheck(t *testing.T) {
	t.Helper()
	binary, err := buildGoBinary()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, f := range corpus() {
		dir, err := prepareFixture(root, "selfcheck-"+f.name, "go", f)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(binary, ".", "--static", "_out")
		command.Dir = dir
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", f.name, err, output)
		}
		info, err := os.Stat(filepath.Join(dir, "_out", "index.html"))
		if err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s: wrote an empty index.html", f.name)
		}
	}
	t.Logf("self-checked %d fixtures", len(corpus()))
}

func TestDifferential(t *testing.T) {
	oracle := defaultOracle()
	_, nodeErr := exec.LookPath("node")
	if runtime.GOOS == "windows" || nodeErr != nil || oracleReady(oracle) != nil {
		runSelfCheck(t)
		return
	}

	total, failures, err := runSuite(suiteOptions{
		oracle: oracle,
		logf:   t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("no differential cases ran")
	}
	for _, f := range failures {
		t.Errorf("%s\n%s", f.caseName, indent(f.detail))
	}
	t.Logf("%d cases, %d passed, %d failed", total, total-len(failures), len(failures))
}

func TestLogAndIndentFormatHarnessOutput(t *testing.T) {
	var lines []string
	opts := suiteOptions{logf: func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }}
	opts.log("checked %d", 3)
	suiteOptions{}.log("dropped")
	if len(lines) != 1 || lines[0] != "checked 3" {
		t.Errorf("log lines = %v", lines)
	}
	if got := indent("a\nb"); got != "    a\n    b" {
		t.Errorf("indent() = %q", got)
	}
}

func TestKillGroupStopsARunningProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are a Unix concept")
	}
	command := exec.Command("sleep", "30")
	useProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	killGroup(command)
	_ = command.Wait()
}
