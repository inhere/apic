package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
)

// dryRunEnv loads an Env whose only variables are SET_ONE and SET_TWO.
func dryRunEnv(t *testing.T) *config.Env {
	t.Helper()
	dir := testDir(t)
	putFile(t, filepath.Join(dir, ".env"), "SET_ONE=value-one\nSET_TWO=value-two\n")
	env, err := config.LoadEnv(dir)
	assert.Require(t, assert.NoErr(t, err))
	return env
}

func TestResolveAuthDryRun(t *testing.T) {
	env := dryRunEnv(t)
	resolve := func(auth model.AuthSpec) (model.AuthSpec, []string, *model.Secrets, error) {
		sec := &model.Secrets{}
		site := &model.Site{Auths: map[string]model.AuthSpec{"a": auth}}
		got, missing, err := config.ResolveAuthDryRun(site, "a", env, sec)
		return got, missing, sec, err
	}

	t.Run("credential fields tolerate a missing variable", func(t *testing.T) {
		cases := map[string]model.AuthSpec{
			"token":    {Type: "bearer", Token: "${env:DRY_NOPE_A}"},
			"password": {Type: "basic", Username: "u", Password: "${env:DRY_NOPE_A}"},
			"value":    {Type: "apikey", In: "header", Name: "X-Key", Value: "${env:DRY_NOPE_A}"},
			"command":  {Type: "exec", Exec: model.ExecSpec{Command: []string{"tool", "${env:DRY_NOPE_A}"}}},
			"exec.env": {Type: "exec", Exec: model.ExecSpec{Command: []string{"tool"}, Env: map[string]string{"K": "${env:DRY_NOPE_A}"}}},
			"body":     {Type: "login", Login: model.LoginSpec{Body: map[string]any{"o": map[string]any{"p": "${env:DRY_NOPE_A}"}}}},
		}
		for name, auth := range cases {
			t.Run(name, func(t *testing.T) {
				_, missing, _, err := resolve(auth)
				assert.NoErr(t, err)
				assert.Eq(t, []string{"DRY_NOPE_A"}, missing)
			})
		}
	})

	t.Run("placeholder kept and set variable registered", func(t *testing.T) {
		got, missing, sec, err := resolve(model.AuthSpec{Type: "bearer", Token: "${env:SET_ONE}-${env:DRY_NOPE_A}"})
		assert.NoErr(t, err)
		assert.Eq(t, "value-one-${env:DRY_NOPE_A}", got.Token)
		assert.Eq(t, []string{"DRY_NOPE_A"}, missing)
		red := &model.Redactor{Secrets: sec}
		assert.Eq(t, "***", red.Redact("value-one"))
		// The unset variable's placeholder is not a secret.
		assert.Eq(t, "${env:DRY_NOPE_A}", red.Redact("${env:DRY_NOPE_A}"))
	})

	t.Run("deduplicated and sorted", func(t *testing.T) {
		_, missing, _, err := resolve(model.AuthSpec{Type: "bearer",
			Token: "${env:DRY_ZED}${env:DRY_ALPHA}${env:DRY_ZED}", Password: "${env:DRY_ALPHA}"})
		assert.NoErr(t, err)
		assert.Eq(t, []string{"DRY_ALPHA", "DRY_ZED"}, missing)
	})

	t.Run("control characters removed from names", func(t *testing.T) {
		_, missing, _, err := resolve(model.AuthSpec{Type: "bearer", Token: "${env:DRY\x1b[31m_X}"})
		assert.NoErr(t, err)
		assert.Eq(t, []string{"DRY[31m_X"}, missing)
	})

	t.Run("plain fields still fail", func(t *testing.T) {
		cases := map[string]model.AuthSpec{
			"username":      {Type: "basic", Username: "${env:DRY_NOPE_A}"},
			"type":          {Type: "${env:DRY_NOPE_A}"},
			"allow_methods": {Type: "bearer", AllowMethods: []string{"${env:DRY_NOPE_A}"}},
			"apply.header":  {Type: "login", Apply: model.ApplySpec{Header: "${env:DRY_NOPE_A}"}},
			"apply.value":   {Type: "login", Apply: model.ApplySpec{Value: "${env:DRY_NOPE_A}"}},
			"login.path":    {Type: "login", Login: model.LoginSpec{Path: "${env:DRY_NOPE_A}"}},
		}
		for name, auth := range cases {
			t.Run(name, func(t *testing.T) {
				_, _, _, err := resolve(auth)
				localError(t, err, "DRY_NOPE_A")
			})
		}
	})

	t.Run("expanded value is not expanded again", func(t *testing.T) {
		dir := testDir(t)
		putFile(t, filepath.Join(dir, ".env"), "OUTER=${env:INNER}\nINNER=deep\n")
		env, err := config.LoadEnv(dir)
		assert.Require(t, assert.NoErr(t, err))
		site := &model.Site{Auths: map[string]model.AuthSpec{"a": {Type: "bearer", Token: "${env:OUTER}"}}}
		got, missing, err := config.ResolveAuthDryRun(site, "a", env, &model.Secrets{})
		assert.NoErr(t, err)
		assert.Eq(t, "${env:INNER}", got.Token)
		assert.Empty(t, missing)
	})

	t.Run("nothing missing", func(t *testing.T) {
		got, missing, _, err := resolve(model.AuthSpec{Type: "bearer", Token: "${env:SET_TWO}"})
		assert.NoErr(t, err)
		assert.Eq(t, "value-two", got.Token)
		assert.Empty(t, missing)
	})

	t.Run("unknown auth and nil site", func(t *testing.T) {
		_, _, err := config.ResolveAuthDryRun(&model.Site{}, "x", env, &model.Secrets{})
		localError(t, err, "unknown auth")
		_, _, err = config.ResolveAuthDryRun(nil, "x", env, &model.Secrets{})
		localError(t, err, "site is nil")
	})

	t.Run("strict ResolveAuth unchanged", func(t *testing.T) {
		site := &model.Site{Auths: map[string]model.AuthSpec{"a": {Type: "bearer", Token: "${env:DRY_NOPE_A}"}}}
		_, err := config.ResolveAuth(site, "a", env, &model.Secrets{})
		localError(t, err, "DRY_NOPE_A")
	})
}

func TestCheckAuthNames(t *testing.T) {
	cfg := &model.Config{Sites: map[string]*model.Site{
		"zeta": {
			Auths:       map[string]model.AuthSpec{"ok": {Type: "bearer", Token: "${env:T}"}},
			DefaultAuth: "typo",
			Envs:        map[string]model.Env{"prod": {Auth: "ok"}, "dev": {Auth: "nope"}, "qa": {}},
		},
		"alpha": {
			Auths:       map[string]model.AuthSpec{"ok": {Type: "bearer", Token: "${env:T}"}},
			DefaultAuth: "ok",
			Envs:        map[string]model.Env{"e": {Auth: "bad"}},
		},
	}}
	got := config.Check(cfg)
	var hints []string
	for _, w := range got {
		if strings.Contains(w, "not defined") {
			hints = append(hints, w)
		}
	}
	assert.Eq(t, []string{
		`site alpha env e: auth "bad" is not defined`,
		`site zeta env dev: auth "nope" is not defined`,
		`site zeta: default_auth "typo" is not defined`,
	}, hints)

	t.Run("valid names give no hint", func(t *testing.T) {
		cfg := &model.Config{Sites: map[string]*model.Site{"s": {
			Auths: map[string]model.AuthSpec{"a": {Type: "bearer", Token: "${env:T}"}}, DefaultAuth: "a",
			Envs: map[string]model.Env{"e": {Auth: "a"}},
		}}}
		assert.Empty(t, config.Check(cfg))
	})
}
