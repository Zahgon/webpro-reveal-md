package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type implementation struct {
	name string
	bin  string
	pre  []string
}

func (i implementation) command(args []string) *exec.Cmd {
	full := append(append([]string{}, i.pre...), args...)
	return exec.Command(i.bin, full...)
}

type cliResult struct {
	stdout   string
	stderr   string
	exitCode int
	tree     map[string]string
}

type failure struct {
	caseName string
	detail   string
}

type suiteOptions struct {
	oracle  string
	node    string
	binary  string
	filter  string
	verbose bool
	logf    func(format string, args ...any)
}

func (o suiteOptions) log(format string, args ...any) {
	if o.logf != nil {
		o.logf(format, args...)
	}
}

func defaultOracle() string {
	return filepath.Join(repoRoot(), "source_javascript")
}

func oracleReady(oracle string) error {
	entry := filepath.Join(oracle, "bin", "reveal-md.js")
	if _, err := os.Stat(entry); err != nil {
		return fmt.Errorf("reference implementation not found at %s: %w", entry, err)
	}
	if _, err := os.Stat(filepath.Join(oracle, "node_modules")); err != nil {
		return fmt.Errorf("%s has no node_modules: %w", oracle, err)
	}
	return nil
}

func runSuite(opts suiteOptions) (int, []failure, error) {
	if opts.oracle == "" {
		opts.oracle = defaultOracle()
	}
	if opts.node == "" {
		opts.node = "node"
	}
	if err := oracleReady(opts.oracle); err != nil {
		return 0, nil, err
	}

	goBinary := opts.binary
	if goBinary == "" {
		built, err := buildGoBinary()
		if err != nil {
			return 0, nil, fmt.Errorf("failed to build the Go binary: %w", err)
		}
		goBinary = built
	}

	js := implementation{name: "node", bin: opts.node, pre: []string{filepath.Join(opts.oracle, "bin", "reveal-md.js")}}
	golang := implementation{name: "go", bin: goBinary}

	fixtures := map[string]fixture{}
	for _, f := range corpus() {
		fixtures[f.name] = f
	}

	root, err := os.MkdirTemp("", "difftest-")
	if err != nil {
		return 0, nil, err
	}
	defer os.RemoveAll(root)

	var failures []failure
	total := 0

	for _, c := range append(globalCases(), staticCases()...) {
		if opts.filter != "" && !strings.Contains(c.name, opts.filter) {
			continue
		}
		total++
		if opts.verbose {
			opts.log("· %s\n", c.name)
		}
		detail, err := runCLIComparison(root, js, golang, fixtures, c)
		if err != nil {
			failures = append(failures, failure{c.name, err.Error()})
			continue
		}
		if detail != "" {
			failures = append(failures, failure{c.name, detail})
		}
	}

	for _, sc := range serverCases() {
		if opts.filter != "" && !strings.Contains(sc.name, opts.filter) {
			continue
		}
		total++
		if opts.verbose {
			opts.log("· %s\n", sc.name)
		}
		detail, err := runServerComparison(root, js, golang, fixtures, sc)
		if err != nil {
			failures = append(failures, failure{sc.name, err.Error()})
			continue
		}
		if detail != "" {
			failures = append(failures, failure{sc.name, detail})
		}
	}

	return total, failures, nil
}

func main() {
	oracle := flag.String("oracle", "", "path to the JavaScript reference implementation (default: source_javascript/)")
	node := flag.String("node", "node", "node executable")
	binary := flag.String("binary", "", "path to a prebuilt reveal-md Go binary (built automatically when empty)")
	filter := flag.String("run", "", "only run cases whose name contains this substring")
	verbose := flag.Bool("v", false, "print every case as it runs")
	flag.Parse()

	total, failures, err := runSuite(suiteOptions{
		oracle:  *oracle,
		node:    *node,
		binary:  *binary,
		filter:  *filter,
		verbose: *verbose,
		logf:    func(format string, args ...any) { fmt.Printf(format, args...) },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "install its dependencies with: (cd source_javascript && PUPPETEER_SKIP_DOWNLOAD=true npm install)")
		os.Exit(2)
	}

	fmt.Printf("\n%d cases, %d passed, %d failed\n", total, total-len(failures), len(failures))
	if len(failures) > 0 {
		fmt.Println()
		for _, f := range failures {
			fmt.Printf("FAIL %s\n%s\n", f.caseName, indent(f.detail))
		}
		os.Exit(1)
	}
	fmt.Println("all differential checks passed")
}

func buildGoBinary() (string, error) {
	dir, err := os.MkdirTemp("", "difftest-bin-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "reveal-md")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = repoRoot()
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

func prepareFixture(root, caseName, impl string, f fixture) (string, error) {
	dir := filepath.Join(root, sanitizeName(caseName), impl)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := f.materialize(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func sanitizeName(name string) string {
	return strings.NewReplacer("/", "_", " ", "_").Replace(name)
}

func runCLIComparison(root string, js, golang implementation, fixtures map[string]fixture, c cliCase) (string, error) {
	f, ok := fixtures[c.fixture]
	if !ok {
		return "", fmt.Errorf("unknown fixture %q", c.fixture)
	}

	jsDir, err := prepareFixture(root, c.name, "node", f)
	if err != nil {
		return "", err
	}
	goDir, err := prepareFixture(root, c.name, "go", f)
	if err != nil {
		return "", err
	}

	jsResult, err := runCLI(js, jsDir, c)
	if err != nil {
		return "", err
	}
	goResult, err := runCLI(golang, goDir, c)
	if err != nil {
		return "", err
	}

	var diffs []string
	if jsResult.exitCode != goResult.exitCode {
		diffs = append(diffs, fmt.Sprintf("exit code: node=%d go=%d", jsResult.exitCode, goResult.exitCode))
	}
	if d := diffText("stdout", normalizeStream(jsResult.stdout, c.sortLines), normalizeStream(goResult.stdout, c.sortLines)); d != "" {
		diffs = append(diffs, d)
	}
	if d := diffText("stderr", normalizeStream(jsResult.stderr, c.sortLines), normalizeStream(goResult.stderr, c.sortLines)); d != "" {
		diffs = append(diffs, d)
	}
	if c.tree {
		if d := diffTrees(jsResult.tree, goResult.tree); d != "" {
			diffs = append(diffs, d)
		}
	}
	return strings.Join(diffs, "\n"), nil
}

func runCLI(impl implementation, dir string, c cliCase) (*cliResult, error) {
	cmd := impl.command(c.args)
	cmd.Dir = dir
	cmd.Env = childEnv()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	timer := time.AfterFunc(3*time.Minute, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	err := cmd.Run()
	timer.Stop()

	result := &cliResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCodeOf(err),
	}
	if err != nil && result.exitCode < 0 {
		return nil, fmt.Errorf("%s: %w", impl.name, err)
	}
	if c.tree {
		tree, terr := hashTree(dir)
		if terr != nil {
			return nil, terr
		}
		result.tree = tree
	}
	return result, nil
}

func runServerComparison(root string, js, golang implementation, fixtures map[string]fixture, sc serverCase) (string, error) {
	f, ok := fixtures[sc.fixture]
	if !ok {
		return "", fmt.Errorf("unknown fixture %q", sc.fixture)
	}

	jsDir, err := prepareFixture(root, sc.name, "node", f)
	if err != nil {
		return "", err
	}
	goDir, err := prepareFixture(root, sc.name, "go", f)
	if err != nil {
		return "", err
	}

	jsRun, err := runServerCase(js, jsDir, sc)
	if err != nil {
		return "", err
	}
	goRun, err := runServerCase(golang, goDir, sc)
	if err != nil {
		return "", err
	}

	var diffs []string
	if jsRun.exitCode != goRun.exitCode {
		diffs = append(diffs, fmt.Sprintf("exit code after SIGINT: node=%d go=%d", jsRun.exitCode, goRun.exitCode))
	}
	if d := diffText("stdout", normalizeStream(jsRun.stdout, false), normalizeStream(goRun.stdout, false)); d != "" {
		diffs = append(diffs, d)
	}
	for i, p := range sc.paths {
		a, b := jsRun.responses[i], goRun.responses[i]
		if a.status != b.status {
			diffs = append(diffs, fmt.Sprintf("%s: status node=%d go=%d", p, a.status, b.status))
		}
		if a.contentType != b.contentType {
			diffs = append(diffs, fmt.Sprintf("%s: content-type node=%q go=%q", p, a.contentType, b.contentType))
		}
		if d := diffText(p+" body", normalizeStream(a.body, false), normalizeStream(b.body, false)); d != "" {
			diffs = append(diffs, d)
		}
	}
	return strings.Join(diffs, "\n"), nil
}

func hashTree(dir string) (map[string]string, error) {
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		tree[filepath.ToSlash(rel)] = hashBytes(normalizeArtifact(rel, data))
		return nil
	})
	return tree, err
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

func diffTrees(a, b map[string]string) string {
	var lines []string
	for name, hash := range a {
		other, ok := b[name]
		if !ok {
			lines = append(lines, "missing in go: "+name)
			continue
		}
		if other != hash {
			lines = append(lines, "content differs: "+name)
		}
	}
	for name := range b {
		if _, ok := a[name]; !ok {
			lines = append(lines, "unexpected in go: "+name)
		}
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return ""
	}
	return "tree:\n" + strings.Join(lines, "\n")
}

func diffText(label, a, b string) string {
	if a == b {
		return ""
	}
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	var out []string
	out = append(out, label+":")
	max := len(aLines)
	if len(bLines) > max {
		max = len(bLines)
	}
	shown := 0
	for i := 0; i < max && shown < 12; i++ {
		var al, bl string
		if i < len(aLines) {
			al = aLines[i]
		}
		if i < len(bLines) {
			bl = bLines[i]
		}
		if al != bl {
			out = append(out, fmt.Sprintf("  line %d:\n    node: %q\n    go:   %q", i+1, al, bl))
			shown++
		}
	}
	return strings.Join(out, "\n")
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}

func asExitError(err error, target **exec.ExitError) bool {
	return errors.As(err, target)
}
