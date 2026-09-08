package jsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// SystemError reproduces a Node.js libuv-backed system error (the objects
// produced by fs operations). test/program.spec.js pins the exact stderr text
// that `console.error(err)` produces for a failing fs.stat, so the message,
// the property order and the punctuation are all part of the contract:
//
//	[Error: ENOENT: no such file or directory, stat '/abs/no_such_file.md'] {
//	  errno: -2,
//	  code: 'ENOENT',
//	  syscall: 'stat',
//	  path: '/abs/no_such_file.md'
//	}
type SystemError struct {
	Errno   int    // libuv errno: the negated platform errno
	Code    string // e.g. "ENOENT"
	Syscall string // e.g. "stat"
	Path    string // absolute path, as Node reports it
	Desc    string // libuv description, e.g. "no such file or directory"
}

// uvMessages maps error codes to the libuv description strings Node embeds in
// the error message. Go's own error strings differ ("no such file or
// directory" happens to match, but "not a directory" vs "NotDir" does not),
// so the table is explicit.
var uvMessages = map[string]string{
	"ENOENT":       "no such file or directory",
	"ENOTDIR":      "not a directory",
	"EISDIR":       "illegal operation on a directory",
	"EACCES":       "permission denied",
	"EPERM":        "operation not permitted",
	"ELOOP":        "too many symbolic links encountered",
	"ENAMETOOLONG": "name too long",
	"EMFILE":       "too many open files",
	"ENFILE":       "file table overflow",
	"ENOTEMPTY":    "directory not empty",
	"EEXIST":       "file already exists",
	"EBUSY":        "resource busy or locked",
	"EINVAL":       "invalid argument",
	"EIO":          "i/o error",
	"ENOSPC":       "no space left on device",
	"EROFS":        "read-only file system",
	"EBADF":        "bad file descriptor",
	"EAGAIN":       "resource temporarily unavailable",
	"EADDRINUSE":   "address already in use",
	"ECONNREFUSED": "connection refused",
}

// errnoCodes maps the platform errno values reveal-md can encounter to their
// symbolic names. The numeric values come from the syscall package, so this
// stays correct across GOOS (ELOOP is 62 on darwin and 40 on linux).
var errnoCodes = map[syscall.Errno]string{
	syscall.ENOENT:       "ENOENT",
	syscall.ENOTDIR:      "ENOTDIR",
	syscall.EISDIR:       "EISDIR",
	syscall.EACCES:       "EACCES",
	syscall.EPERM:        "EPERM",
	syscall.ELOOP:        "ELOOP",
	syscall.ENAMETOOLONG: "ENAMETOOLONG",
	syscall.EMFILE:       "EMFILE",
	syscall.ENFILE:       "ENFILE",
	syscall.ENOTEMPTY:    "ENOTEMPTY",
	syscall.EEXIST:       "EEXIST",
	syscall.EBUSY:        "EBUSY",
	syscall.EINVAL:       "EINVAL",
	syscall.EIO:          "EIO",
	syscall.ENOSPC:       "ENOSPC",
	syscall.EROFS:        "EROFS",
	syscall.EBADF:        "EBADF",
	syscall.EAGAIN:       "EAGAIN",
	syscall.EADDRINUSE:   "EADDRINUSE",
	syscall.ECONNREFUSED: "ECONNREFUSED",
}

// Error returns the `message` property of the JavaScript error, which is what
// appears inside the square brackets.
func (e *SystemError) Error() string {
	return fmt.Sprintf("%s: %s, %s '%s'", e.Code, e.Desc, e.Syscall, e.Path)
}

// Inspect renders the error the way `console.error(err)` does: the bracketed
// message followed by the own enumerable properties, one per line, indented by
// two spaces, with a trailing comma on every line but the last.
func (e *SystemError) Inspect() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Error: %s] {\n", e.Error())
	fmt.Fprintf(&b, "  errno: %d,\n", e.Errno)
	fmt.Fprintf(&b, "  code: %s,\n", InspectString(e.Code))
	fmt.Fprintf(&b, "  syscall: %s,\n", InspectString(e.Syscall))
	fmt.Fprintf(&b, "  path: %s\n", InspectString(e.Path))
	b.WriteString("}")
	return b.String()
}

// Inspect renders an error the way console.error(err) would.
func Inspect(err error) string {
	if inspectable, ok := err.(interface{ Inspect() string }); ok {
		return inspectable.Inspect()
	}
	return err.Error()
}

// NewSystemError converts a Go filesystem error into its Node equivalent.
// syscallName is the libuv operation name Node would report ("stat", "open",
// "scandir", ...) and path is the absolute path Node would have used.
func NewSystemError(err error, syscallName, path string) *SystemError {
	var already *SystemError
	if errors.As(err, &already) {
		return already
	}

	code := "UNKNOWN"
	errno := -1

	var errnoVal syscall.Errno
	if errors.As(err, &errnoVal) {
		if name, ok := errnoCodes[errnoVal]; ok {
			code = name
		}
		errno = -int(errnoVal)
	} else {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			code = "ENOENT"
			errno = -int(syscall.ENOENT)
		case errors.Is(err, fs.ErrPermission):
			code = "EACCES"
			errno = -int(syscall.EACCES)
		case errors.Is(err, fs.ErrExist):
			code = "EEXIST"
			errno = -int(syscall.EEXIST)
		}
	}

	desc, ok := uvMessages[code]
	if !ok {
		desc = err.Error()
	}
	return &SystemError{
		Errno:   errno,
		Code:    code,
		Syscall: syscallName,
		Path:    path,
		Desc:    desc,
	}
}

// InspectString renders a string the way util.inspect does: single quotes by
// default, double quotes when the value contains a single quote but no double
// quote, and backticks when it contains both kinds of quote.
func InspectString(s string) string {
	quote := byte('\'')
	switch {
	case !strings.Contains(s, "'"):
		quote = '\''
	case !strings.Contains(s, `"`):
		quote = '"'
	default:
		quote = '`'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == quote || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\v':
			b.WriteString(`\v`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02X`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// PhysicalCwd returns the working directory the way Node's process.cwd() does.
//
// Node calls getcwd(3), which always returns the fully resolved physical path;
// Go's os.Getwd prefers $PWD when it refers to the same directory, so on macOS
// a shell sitting in /tmp yields "/tmp" from Go but "/private/tmp" from Node.
// The ENOENT message pinned by test/program.spec.js contains this path, so the
// symlinks have to be resolved to match byte for byte.
func PhysicalCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// filepath.EvalSymlinks is the closest equivalent of realpath(3).
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return cwd, nil
	}
	return resolved, nil
}
