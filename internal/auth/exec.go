package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/inhere/apic/internal/model"
)

const (
	dryExecToken       = "<exec-token>"
	execOutputLimit    = 4 << 20
	defaultExecTimeout = 30 * time.Second
	execWaitDelay      = 2 * time.Second
	execErrMaxRunes    = 200
)

// applyExec mirrors applyLogin: in-memory token, then cache, then (unless dry)
// run the command.
func (p *Provider) applyExec(ctx context.Context, req *http.Request, dry bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && (p.expires.IsZero() || p.deps.Now().Add(expirySkew).Before(p.expires)) {
		p.addSecret(p.token)
		p.injectToken(req, p.token, p.spec.Apply)
		return nil
	}
	if e, ok := p.readCache(); ok {
		p.token, p.expires = e.Token, e.ExpiresAt
		p.addSecret(p.token)
		p.injectToken(req, p.token, p.spec.Apply)
		return nil
	}
	if dry {
		p.injectToken(req, dryExecToken, p.spec.Apply)
		return nil
	}
	if err := p.doExec(ctx); err != nil {
		return err
	}
	p.injectToken(req, p.token, p.spec.Apply)
	return nil
}

// doExec runs the command and stores the token. The cache is written only when
// the expiry is known. Caller holds p.mu.
func (p *Provider) doExec(ctx context.Context) error {
	token, expires, err := p.runExec(ctx)
	if err != nil {
		return err
	}
	p.addSecret(token)
	if expires.IsZero() {
		if ttl, ok := parsePositiveDuration(p.spec.Exec.TTL); ok {
			expires = p.deps.Now().Add(ttl)
		}
	}
	p.token, p.expires = token, expires
	if expires.IsZero() {
		return nil
	}
	if err := p.writeCache(cacheEntry{Token: token, ExpiresAt: expires, Fingerprint: p.fingerprint()}); err != nil {
		return model.Errorf(model.ExitLocal, "auth %s: write token cache: %s", p.authName, p.redact(err.Error()))
	}
	return nil
}

func parsePositiveDuration(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	d, err := time.ParseDuration(s)
	return d, err == nil && d > 0
}

// execCacheable reports whether a successful run can ever be cached.
func (p *Provider) execCacheable() bool {
	x := p.spec.Exec
	if x.Format == "json" && x.Expires.Path != "" {
		return true
	}
	_, ok := parsePositiveDuration(x.TTL)
	return ok
}

// execFailure builds the only error shape runExec returns. The message never
// contains argv (including command[0]).
func (p *Provider) execFailure(code int, msg string) error {
	return model.Errorf(model.ExitLocal, "auth %s: exec failed (exit %d): %s", p.authName, code, msg)
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

// lastLine returns the last non-empty stderr line, cleaned, redacted and capped.
func (p *Provider) lastLine(stderr []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(stderr), "\r", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := ansiRe.ReplaceAllString(lines[i], "")
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Redact before truncating so a cut can never leave a secret prefix.
		r := []rune(p.redact(line))
		if len(r) > execErrMaxRunes {
			r = r[:execErrMaxRunes]
		}
		return string(r)
	}
	return ""
}

// resolveCommand maps command[0] to an executable path. Relative paths resolve
// against dir; bare names go through PATH and never match the current directory.
func resolveCommand(name, dir string) (string, error) {
	switch {
	case name == "":
		return "", errors.New("empty command")
	case filepath.IsAbs(name):
		return name, nil
	case strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, "."):
		return filepath.Join(dir, name), nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		if errors.Is(err, exec.ErrDot) {
			return "", errors.New("command resolves to the current directory")
		}
		return "", err
	}
	return path, nil
}

// limitBuffer keeps up to limit bytes and calls onExceed once when more arrive.
type limitBuffer struct {
	buf      bytes.Buffer
	limit    int
	exceeded bool
	onExceed func()
	mu       sync.Mutex
}

func (b *limitBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.exceeded {
		return len(data), nil // keep draining so the child cannot block
	}
	if b.buf.Len()+len(data) > b.limit {
		b.exceeded = true
		b.onExceed()
		return len(data), nil
	}
	return b.buf.Write(data)
}

func (b *limitBuffer) over() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exceeded
}

// runExec runs the configured command and returns the token and an absolute
// expiry (zero when the output gives none).
func (p *Provider) runExec(ctx context.Context) (string, time.Time, error) {
	x := p.spec.Exec
	if len(x.Command) == 0 {
		return "", time.Time{}, p.execFailure(-1, "no command configured")
	}
	timeout := defaultExecTimeout
	if d, ok := parsePositiveDuration(x.Timeout); ok {
		timeout = d
	}
	path, err := resolveCommand(x.Command[0], p.deps.ConfigDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, exec.ErrNotFound) {
			return "", time.Time{}, p.execFailure(-1, "command not found")
		}
		return "", time.Time{}, p.execFailure(-1, "cannot resolve command")
	}
	args := x.Command[1:]
	if err := checkResolved(path, args); err != nil {
		return "", time.Time{}, p.execFailure(-1, err.Error())
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, args...)
	cmd.Dir = p.deps.ConfigDir
	cmd.Stdin = nil // the null device
	cmd.Env = p.execEnv()
	cmd.WaitDelay = execWaitDelay
	configureCmd(cmd)
	stdout := &limitBuffer{limit: execOutputLimit, onExceed: cancel}
	stderr := &limitBuffer{limit: execOutputLimit, onExceed: cancel}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	runErr := cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	if stdout.over() || stderr.over() {
		return "", time.Time{}, p.execFailure(code, "output exceeded 4 MiB")
	}
	if runErr != nil && !(errors.Is(runErr, exec.ErrWaitDelay) && code == 0) {
		switch {
		case ctx.Err() != nil:
			return "", time.Time{}, p.execFailure(code, "exec timed out (request deadline)")
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			return "", time.Time{}, p.execFailure(code, "exec timed out after "+timeout.String())
		case cmd.ProcessState == nil && (errors.Is(runErr, os.ErrNotExist) || errors.Is(runErr, exec.ErrNotFound)):
			return "", time.Time{}, p.execFailure(-1, "command not found")
		case cmd.ProcessState == nil && errors.Is(runErr, os.ErrPermission):
			return "", time.Time{}, p.execFailure(-1, "permission denied")
		case cmd.ProcessState == nil:
			return "", time.Time{}, p.execFailure(-1, "cannot start command")
		}
		return "", time.Time{}, p.execFailure(code, p.lastLine(stderr.buf.Bytes()))
	}
	return p.parseExecOutput(stdout.buf.Bytes())
}

// execEnv is the parent environment plus exec.env plus the APIC_* identity.
func (p *Provider) execEnv() []string {
	env := os.Environ()
	keys := make([]string, 0, len(p.spec.Exec.Env))
	for k := range p.spec.Exec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+p.spec.Exec.Env[k])
	}
	return append(env, "APIC_SITE="+p.site, "APIC_ENV="+p.envName, "APIC_AUTH="+p.authName)
}

func validToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (p *Provider) parseExecOutput(out []byte) (string, time.Time, error) {
	x := p.spec.Exec
	if x.Format != "json" {
		token := strings.TrimSpace(string(out))
		if !validToken(token) {
			return "", time.Time{}, p.execFailure(0, "output is not a single-line token")
		}
		return token, time.Time{}, nil
	}
	doc, err := model.Decode(out)
	if err != nil {
		return "", time.Time{}, p.execFailure(0, "output is not valid JSON")
	}
	pick, err := model.ParsePick(x.Token.Path)
	if err != nil {
		return "", time.Time{}, p.execFailure(0, "token.path is invalid")
	}
	val, err := pick.Apply(doc)
	token, ok := val.(string)
	if err != nil || !ok || !validToken(token) {
		return "", time.Time{}, p.execFailure(0, "token not found or not a single-line string in output")
	}
	var expires time.Time
	if x.Expires.Path != "" {
		if ep, err := model.ParsePick(x.Expires.Path); err == nil {
			if v, err := ep.Apply(doc); err == nil {
				if secs, ok := execSeconds(v); ok && secs > 0 {
					expires = p.deps.Now().Add(time.Duration(secs * float64(time.Second)))
				}
			}
		}
	}
	return token, expires, nil
}

func execSeconds(v any) (float64, bool) {
	var s string
	switch t := v.(type) {
	case json.Number:
		s = t.String()
	case string:
		s = strings.TrimSpace(t)
	default:
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// execFingerprint binds an exec token to type, base_url, the expanded exec
// section and apply. json.Marshal sorts map keys.
func (p *Provider) execFingerprint() string {
	raw, _ := json.Marshal(struct {
		Type, BaseURL string
		Exec          model.ExecSpec
		Apply         model.ApplySpec
	}{p.spec.Type, p.env.BaseURL, p.spec.Exec, p.spec.Apply})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// checkBatchArgs rejects arguments cmd.exe would re-interpret when the
// resolved executable is a .bat or .cmd file. Errors carry only the argument
// number, never its value.
func checkBatchArgs(resolvedPath string, args []string) error {
	ext := strings.ToLower(filepath.Ext(resolvedPath))
	if ext != ".bat" && ext != ".cmd" {
		return nil
	}
	for i, a := range args {
		if strings.ContainsAny(a, "&|<>^%!\"\r\n") {
			return errors.New("argument " + strconv.Itoa(i+1) + " contains characters not allowed for a batch file")
		}
	}
	return nil
}
