package render

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

const nodeShim = `
import { pathToFileURL } from 'node:url';
const { default: fn } = await import(pathToFileURL(process.env.REVEAL_MD_PREPROCESSOR));
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const markdown = Buffer.concat(chunks).toString();
const options = JSON.parse(process.env.REVEAL_MD_OPTIONS ?? '{}');
process.stdout.write(String(await fn(markdown, options)));
`

// runPreprocessor ports getPreprocessor plus its invocation.
//
// The original imports an arbitrary ES module and calls its default export.
// Go cannot import JavaScript, so a preprocessor is run as a subprocess: a
// JavaScript file through Node when it is available, any other executable
// directly, receiving the markdown on stdin and the options as JSON in
// REVEAL_MD_OPTIONS, and returning the result on stdout.
func runPreprocessor(cfg *config.Config, options *jsutil.Object, markdown string) (string, error) {
	value := options.Get("preprocessor")
	if !jsutil.Truthy(value) {
		return markdown, nil
	}
	name := jsutil.JSString(value)
	scriptPath := jsutil.PathResolve(cfg.Cwd, name)
	if _, err := os.Stat(scriptPath); err != nil {
		return "", fmt.Errorf("cannot find preprocessor %s: %w", name, err)
	}

	optionsJSON := jsutil.StringifyOrEmpty(options)
	if optionsJSON == "" {
		optionsJSON = "{}"
	}

	cmd, err := preprocessorCommand(scriptPath)
	if err != nil {
		return "", err
	}
	cmd.Dir = cfg.Cwd
	cmd.Env = append(os.Environ(),
		"REVEAL_MD_OPTIONS="+optionsJSON,
		"REVEAL_MD_PREPROCESSOR="+scriptPath)
	cmd.Stdin = strings.NewReader(markdown)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("preprocessor %s failed: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func preprocessorCommand(scriptPath string) (*exec.Cmd, error) {
	if isJavaScript(scriptPath) {
		node, err := exec.LookPath("node")
		if err != nil {
			return nil, fmt.Errorf("preprocessor %s is a JavaScript module and node is not installed: %w", scriptPath, err)
		}
		return exec.Command(node, "--input-type=module", "-e", nodeShim), nil
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		return nil, err
	}
	if info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("preprocessor %s is neither a JavaScript module nor executable", scriptPath)
	}
	return exec.Command(scriptPath), nil
}

func isJavaScript(p string) bool {
	switch jsutil.PathExtname(p) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}
