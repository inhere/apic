package config_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
)

// execStub stands in for the real runner and counts its calls.
type execStub struct {
	calls int
	argv  [][]string
	fn    func(argv []string) (string, error)
}

func (s *execStub) run(argv []string) (string, error) {
	s.calls++
	s.argv = append(s.argv, argv)
	if s.fn == nil {
		// Long enough to be registered as a secret.
		return "stub-token-value", nil
	}
	return s.fn(argv)
}

// newExecEnv returns an Env with EXEC_VAR set and the stub installed.
func newExecEnv(t *testing.T) (*config.Env, *execStub) {
	t.Helper()
	dir := testDir(t)
	putFile(t, filepath.Join(dir, ".env"), "EXEC_VAR=env-value\nNESTED=${exec:nested}\n")
	env, err := config.LoadEnv(dir)
	assert.Require(t, assert.NoErr(t, err))
	stub := &execStub{}
	env.Exec = stub.run
	return env, stub
}

// resolveOne resolves the single auth "pat" of a fresh site.
func resolveOne(site *model.Site, env *config.Env, sec *model.Secrets) (model.AuthSpec, error) {
	return config.ResolveAuth(site, "pat", env, sec)
}

func TestExecRefCredentials(t *testing.T) {
	t.Run("five credential values expand", func(t *testing.T) {
		env, stub := newExecEnv(t)
		stub.fn = func(argv []string) (string, error) { return "value-of-" + argv[0], nil }
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {
			Type: "login", Token: "${exec:get-token}", Password: "${exec:get-pass}", Value: "${exec:get-value}",
			Exec:  model.ExecSpec{Command: []string{"tool"}, Env: map[string]string{"KEY": "${exec:get-key}"}},
			Login: model.LoginSpec{Body: map[string]any{"user": "me", "nested": map[string]any{"password": "${exec:get-body}"}}},
		}}}
		resolved, err := resolveOne(site, env, &model.Secrets{})
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, "value-of-get-token", resolved.Token)
		assert.Eq(t, "value-of-get-pass", resolved.Password)
		assert.Eq(t, "value-of-get-value", resolved.Value)
		assert.Eq(t, "value-of-get-key", resolved.Exec.Env["KEY"])
		assert.Eq(t, "value-of-get-body", resolved.Login.Body["nested"].(map[string]any)["password"])
		assert.Eq(t, 5, stub.calls)
		assert.Eq(t, []string{"get-token"}, stub.argv[0])
	})

	t.Run("mixed with text and env references", func(t *testing.T) {
		env, _ := newExecEnv(t)
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {
			Type: "bearer", Token: "Bearer ${env:EXEC_VAR}-${exec:get-token}-end",
		}}}
		resolved, err := resolveOne(site, env, &model.Secrets{})
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, "Bearer env-value-stub-token-value-end", resolved.Token)
	})

	t.Run("expanded value is not scanned again", func(t *testing.T) {
		env, stub := newExecEnv(t)
		// An environment variable holding a reference stays literal.
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${env:NESTED}"}}}
		resolved, err := resolveOne(site, env, &model.Secrets{})
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, "${exec:nested}", resolved.Token)
		assert.Eq(t, 0, stub.calls)

		// A command output holding an env reference stays literal too.
		stub.fn = func(argv []string) (string, error) { return "${env:EXEC_VAR}", nil }
		site = &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token}"}}}
		resolved, err = resolveOne(site, env, &model.Secrets{})
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, "${env:EXEC_VAR}", resolved.Token)
	})

	t.Run("empty env name stays literal", func(t *testing.T) {
		env, stub := newExecEnv(t)
		var sec model.Secrets
		got, err := config.Expand("x${env:}y", env, &sec)
		assert.NoErr(t, err)
		assert.Eq(t, "x${env:}y", got)
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${env:}"}}}
		resolved, err := resolveOne(site, env, &sec)
		assert.NoErr(t, err)
		assert.Eq(t, "${env:}", resolved.Token)
		assert.Eq(t, 0, stub.calls)
	})

	t.Run("public Expand keeps exec references", func(t *testing.T) {
		env, stub := newExecEnv(t)
		got, err := config.Expand("a-${exec:get-token}-b-${env:EXEC_VAR}", env, &model.Secrets{})
		assert.NoErr(t, err)
		assert.Eq(t, "a-${exec:get-token}-b-env-value", got)
		assert.Eq(t, 0, stub.calls)
	})

	t.Run("deduplicated by reference text", func(t *testing.T) {
		env, stub := newExecEnv(t)
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {
			Type: "basic", Token: "${exec:get-token}", Password: "${exec:get-token}",
		}}}
		resolved, err := resolveOne(site, env, &model.Secrets{})
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, "stub-token-value", resolved.Token)
		assert.Eq(t, "stub-token-value", resolved.Password)
		assert.Eq(t, 1, stub.calls)

		// A failure is not cached: the next resolution runs it again.
		env, stub = newExecEnv(t)
		stub.fn = func(argv []string) (string, error) { return "", &model.ExecFailure{Code: 3, Reason: "boom"} }
		site = &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token}"}}}
		_, err = resolveOne(site, env, &model.Secrets{})
		assert.Err(t, err)
		_, err = resolveOne(site, env, &model.Secrets{})
		assert.Err(t, err)
		assert.Eq(t, 2, stub.calls)
	})

	t.Run("result is registered as a secret", func(t *testing.T) {
		env, _ := newExecEnv(t)
		sec := &model.Secrets{}
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token}"}}}
		_, err := resolveOne(site, env, sec)
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, "***", (&model.Redactor{Secrets: sec}).Redact("stub-token-value"))
	})
}

func TestExecRefPosition(t *testing.T) {
	cases := map[string]model.AuthSpec{
		"exec.command":      {Type: "exec", Exec: model.ExecSpec{Command: []string{"tool", "${exec:get-token}"}}},
		"apply.value":       {Type: "login", Apply: model.ApplySpec{Value: "${exec:get-token}"}},
		"login.apply.value": {Type: "login", Login: model.LoginSpec{Apply: model.ApplySpec{Value: "${exec:get-token}"}}},
		"token.path":        {Type: "exec", Exec: model.ExecSpec{Command: []string{"tool"}, Token: model.ValueFrom{Path: "${exec:get-token}"}}},
		"allow_methods":     {Type: "bearer", AllowMethods: []string{"GET", "${exec:get-token}"}},
		"login.path":        {Type: "login", Login: model.LoginSpec{Path: "${exec:get-token}"}},
		"username":          {Type: "basic", Username: "${exec:get-token}"},
	}
	for name, auth := range cases {
		t.Run("misplaced "+name, func(t *testing.T) {
			env, stub := newExecEnv(t)
			// The credential value would run before the check if it were late.
			auth.Token = "${exec:get-token}"
			site := &model.Site{Auths: map[string]model.AuthSpec{"pat": auth}}
			_, err := resolveOne(site, env, &model.Secrets{})
			assert.Eq(t, `auth "pat": ${exec:} is only allowed in credential values`, err.Error())
			assert.Eq(t, 0, stub.calls)
			localError(t, err, "${exec:} is only allowed in credential values")
		})
		t.Run("misplaced in dry-run "+name, func(t *testing.T) {
			env, stub := newExecEnv(t)
			auth.Token = "${exec:get-token}"
			site := &model.Site{Auths: map[string]model.AuthSpec{"pat": auth}}
			_, _, err := config.ResolveAuthDryRun(site, "pat", env, &model.Secrets{})
			assert.Eq(t, `auth "pat": ${exec:} is only allowed in credential values`, err.Error())
			assert.Eq(t, 0, stub.calls)
		})
	}
}

func TestExecRefMalformed(t *testing.T) {
	t.Run("unclosed in a credential value", func(t *testing.T) {
		env, stub := newExecEnv(t)
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token"}}}
		_, err := resolveOne(site, env, &model.Secrets{})
		assert.Eq(t, `auth "pat" token: malformed ${exec:} reference`, err.Error())
		assert.Eq(t, 0, stub.calls)
	})

	t.Run("unclosed outside credential values is a position error", func(t *testing.T) {
		env, stub := newExecEnv(t)
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "basic", Username: "${exec:get-token"}}}
		_, err := resolveOne(site, env, &model.Secrets{})
		assert.Eq(t, `auth "pat": ${exec:} is only allowed in credential values`, err.Error())
		assert.Eq(t, 0, stub.calls)
	})

	t.Run("unclosed reference injected by an expanded variable", func(t *testing.T) {
		dir := testDir(t)
		putFile(t, filepath.Join(dir, ".env"), "BROKEN=${exec:get-token\n")
		env, err := config.LoadEnv(dir)
		assert.Require(t, assert.NoErr(t, err))
		stub := &execStub{}
		env.Exec = stub.run
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${env:BROKEN}"}}}
		_, err = resolveOne(site, env, &model.Secrets{})
		assert.Eq(t, `auth "pat" token: malformed ${exec:} reference`, err.Error())
		assert.Eq(t, 0, stub.calls)
	})

	t.Run("a closed reference in front of an unclosed one does not run", func(t *testing.T) {
		env, stub := newExecEnv(t)
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token} ${exec:get-token"}}}
		_, err := resolveOne(site, env, &model.Secrets{})
		assert.Eq(t, `auth "pat" token: malformed ${exec:} reference`, err.Error())
		assert.Eq(t, 0, stub.calls)
	})

	t.Run("empty or nested content", func(t *testing.T) {
		for _, content := range []string{"${exec:}", "${exec:echo ${env:EXEC_VAR}}"} {
			t.Run(content, func(t *testing.T) {
				env, stub := newExecEnv(t)
				site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: content}}}
				_, err := resolveOne(site, env, &model.Secrets{})
				assert.Eq(t, `auth "pat" token: malformed ${exec:} reference`, err.Error())
				assert.Eq(t, 0, stub.calls)
			})
		}
	})
}

func TestExecRefOutput(t *testing.T) {
	t.Run("trailing line breaks removed, spaces kept", func(t *testing.T) {
		env, stub := newExecEnv(t)
		stub.fn = func(argv []string) (string, error) { return "  spaced-token  \r\n", nil }
		sec := &model.Secrets{}
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "[${exec:get-token}]"}}}
		resolved, err := resolveOne(site, env, sec)
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, "[  spaced-token  ]", resolved.Token)
		assert.Eq(t, "***", (&model.Redactor{Secrets: sec}).Redact("  spaced-token  "))
	})

	t.Run("empty or multi-line or control output fails", func(t *testing.T) {
		for name, tc := range map[string]struct{ out, marker string }{
			"empty":   {"", ""},
			"newline": {"first-line\nsecond-line", "first-line"},
			"cr":      {"first-line\rsecond-line", "first-line"},
			"control": {"leak\x00value", "leak"},
		} {
			t.Run(name, func(t *testing.T) {
				env, stub := newExecEnv(t)
				out := tc.out
				stub.fn = func(argv []string) (string, error) { return out, nil }
				site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token}"}}}
				_, err := resolveOne(site, env, &model.Secrets{})
				localError(t, err, "${exec:}")
				if tc.marker != "" {
					assert.False(t, strings.Contains(err.Error(), tc.marker))
				}
			})
		}
	})

	t.Run("runner failures", func(t *testing.T) {
		cases := []struct {
			name string
			err  error
			want string
		}{
			{"exit code", &model.ExecFailure{Code: 3, Reason: "bad secret path"}, `auth "pat" value: ${exec:} failed (exit 3): bad secret path`},
			{"not started", &model.ExecFailure{Code: -1, Reason: "timed out"}, `auth "pat" value: ${exec:} failed: timed out`},
			{"wrapped", &model.CodedError{C: 1, Err: &model.ExecFailure{Code: 2, Reason: "wrapped reason"}}, `auth "pat" value: ${exec:} failed (exit 2): wrapped reason`},
			{"other error", errors.New("plain failure"), `auth "pat" value: ${exec:} failed: plain failure`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				env, stub := newExecEnv(t)
				stub.fn = func(argv []string) (string, error) { return "", tc.err }
				site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "apikey", Value: "${exec:pass show pet/token}"}}}
				_, err := resolveOne(site, env, &model.Secrets{})
				assert.Eq(t, tc.want, err.Error())
				assert.NotContains(t, err.Error(), "pass show")
			})
		}
	})

	t.Run("runner not available", func(t *testing.T) {
		dir := testDir(t)
		env, err := config.LoadEnv(dir)
		assert.Require(t, assert.NoErr(t, err))
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token}"}}}
		_, err = resolveOne(site, env, &model.Secrets{})
		assert.Eq(t, `auth "pat" token: ${exec:} is not available`, err.Error())
	})

	t.Run("zero Env works", func(t *testing.T) {
		env := &config.Env{}
		site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {Type: "bearer", Token: "${exec:get-token}"}}}
		_, err := resolveOne(site, env, &model.Secrets{})
		assert.Eq(t, `auth "pat" token: ${exec:} is not available`, err.Error())
	})
}

func TestExecRefDryRun(t *testing.T) {
	env, stub := newExecEnv(t)
	site := &model.Site{Auths: map[string]model.AuthSpec{"pat": {
		Type: "login", Token: "Bearer ${exec:get-token}", Password: "${env:NOPE}-${exec:get-pass}",
		Exec:  model.ExecSpec{Command: []string{"tool"}, Env: map[string]string{"KEY": "${exec:get-key}"}},
		Login: model.LoginSpec{Body: map[string]any{"password": "${exec:get-body}", "list": []any{"${exec:get-item}"}}},
	}}}
	sec := &model.Secrets{}
	resolved, missing, err := config.ResolveAuthDryRun(site, "pat", env, sec)
	assert.Require(t, assert.NoErr(t, err))
	assert.Eq(t, "Bearer <exec>", resolved.Token)
	assert.Eq(t, "${env:NOPE}-<exec>", resolved.Password)
	assert.Eq(t, "<exec>", resolved.Exec.Env["KEY"])
	assert.Eq(t, "<exec>", resolved.Login.Body["password"])
	assert.Eq(t, "<exec>", resolved.Login.Body["list"].([]any)[0])
	assert.Eq(t, []string{"NOPE"}, missing)
	// No command ran and nothing was registered for the placeholders.
	assert.Eq(t, 0, stub.calls)
	assert.Eq(t, "<exec>", (&model.Redactor{Secrets: sec}).Redact("<exec>"))
}

func TestCheckExecRef(t *testing.T) {
	cfg := &model.Config{Sites: map[string]*model.Site{"pet": {Auths: map[string]model.AuthSpec{
		"pat":  {Type: "apikey", In: "header", Name: "X-Key", Value: "Bearer ${exec:pass show pet/token}"},
		"sso":  {Type: "exec", Exec: model.ExecSpec{Command: []string{"${exec:get-token}"}}},
		"bad":  {Type: "basic", Username: "${exec:whoami}", Login: model.LoginSpec{Body: map[string]any{"password": "${exec:get-pass}", "list": []any{"${exec:get-item}"}}}},
		"open": {Type: "bearer", Token: "${exec:get-token"},
	}}}}
	warnings := config.Check(cfg)
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{
		"site pet auth pat value: runs ${exec:}",
		"site pet auth sso exec.command[0]: ${exec:} is only allowed in credential values",
		"site pet auth sso: runs ***",
		"site pet auth bad username: ${exec:} is only allowed in credential values",
		"site pet auth bad login.body.password: runs ${exec:}",
		"site pet auth bad login.body.list[0]: runs ${exec:}",
		"site pet auth open token: malformed ${exec:} reference",
	} {
		t.Run(want, func(t *testing.T) {
			assert.Contains(t, joined, want)
		})
	}
	// A mixed value is a reference, so it is not reported as plaintext.
	assert.False(t, strings.Contains(joined, "plaintext credential"))
	// No hint ever holds the command text.
	for _, secret := range []string{"pass show", "get-pass", "get-item", "whoami", "get-token"} {
		assert.False(t, strings.Contains(joined, secret))
	}
}
