package link_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/link"
	"github.com/inhere/apic/internal/model"
)

func TestLinkNames(t *testing.T) {
	for _, tc := range []struct{ site, name, env, auth string }{
		{"pet", "../x", "", ""}, {"../pet", "x", "", ""},
		{"pet", "x", "bad env", ""}, {"pet", "x", "", "$(bad)"},
	} {
		t.Run(tc.name+tc.site+tc.env+tc.auth, func(t *testing.T) {
			err := link.Names(tc.site, tc.name, tc.env, tc.auth)
			var coded *model.CodedError
			assert.True(t, errors.As(err, &coded))
			assert.Eq(t, model.ExitUsage, coded.Code())
		})
	}
}

func TestLinkScripts(t *testing.T) {
	win := link.Scripts("windows", "pet", "prod", "ro")
	assert.Eq(t, "#!/bin/sh\nexec apic --api pet --env prod --auth ro \"$@\"\n", win[""])
	assert.Eq(t, "@apic --api pet --env prod --auth ro %*\r\n", win[".cmd"])
	assert.Len(t, win, 2)
	assert.Len(t, link.Scripts("linux", "pet", "", ""), 1)
}

func TestLinkCreateRemove(t *testing.T) {
	name := "petctl"
	t.Run("unowned destination", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, name)
		assert.Require(t, assert.NoError(t, os.WriteFile(path, []byte("mine"), 0600)))
		records := link.Records{}
		_, err := link.Create(dir, name, "pet", "", "", &records)
		var coded *model.CodedError
		assert.True(t, errors.As(err, &coded))
		assert.Eq(t, model.ExitLocal, coded.Code())
		got, err := os.ReadFile(path)
		assert.NoError(t, err)
		assert.Eq(t, "mine", string(got))
		assert.Len(t, records, 0)
	})
	t.Run("changed file", func(t *testing.T) {
		dir := t.TempDir()
		records := link.Records{}
		paths, err := link.Create(dir, name, "pet", "prod", "", &records)
		assert.Require(t, assert.NoError(t, err))
		if runtime.GOOS == "windows" {
			assert.Len(t, paths, 2)
		} else {
			assert.Len(t, paths, 1)
			fi, err := os.Stat(paths[0])
			assert.NoError(t, err)
			assert.Eq(t, os.FileMode(0755), fi.Mode().Perm())
		}
		_, err = link.Create(dir, name, "pet", "prod", "", &records)
		assert.NoError(t, err)
		assert.NoError(t, os.WriteFile(paths[0], []byte("changed"), 0600))
		err = link.Remove(name, &records)
		var coded *model.CodedError
		assert.True(t, errors.As(err, &coded))
		assert.Eq(t, model.ExitLocal, coded.Code())
		assert.Len(t, records, len(paths))
		_, err = os.Stat(paths[0])
		assert.NoError(t, err)
		assert.NoError(t, os.Remove(paths[0]))
		assert.NoError(t, link.Remove(name, &records))
		assert.Len(t, records, 0)
		for _, path := range paths {
			_, err := os.Stat(path)
			assert.True(t, os.IsNotExist(err))
		}
	})
}

func TestLinkPassThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	output := filepath.Join(dir, "args")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LINK_ARGS\"\n"
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(dir, "apic"), []byte(stub), 0755)))
	records := link.Records{}
	paths, err := link.Create(dir, "petctl", "pet", "prod", "", &records)
	assert.Require(t, assert.NoError(t, err))
	cmd := exec.Command(paths[0], "list-users", "--x", "a b")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "LINK_ARGS="+output)
	data, err := cmd.CombinedOutput()
	assert.Require(t, assert.NoError(t, err), string(data))
	args, err := os.ReadFile(output)
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, "--api\npet\n--env\nprod\nlist-users\n--x\na b\n", string(args))
	assert.Eq(t, 7, len(strings.Split(strings.TrimSuffix(string(args), "\n"), "\n")))
}
