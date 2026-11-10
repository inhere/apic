package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apix/internal/config"
	"github.com/inhere/apix/internal/link"
	"github.com/inhere/apix/internal/model"
	"github.com/inhere/apix/internal/wiring"
)

type linkOptions struct{ name, env, auth, dir string }

func linkCommand(g *globalOpts) *gcli.Command {
	opt := &linkOptions{}
	return &gcli.Command{Name: "link", Desc: "Generate, list or remove shortcut scripts (.cmd JSON arguments: use -d @file)",
		Config: func(c *gcli.Command) {
			c.AddArg("site", "Site name", true)
			c.StrOpt(&opt.name, "name", "", "", "Shortcut name")
			c.StrOpt(&opt.env, "env", "", "", "Environment to pin")
			c.StrOpt(&opt.auth, "auth", "", "", "Authentication to pin")
			c.StrOpt(&opt.dir, "dir", "", "", "Shortcut directory")
		},
		Func: func(c *gcli.Command, _ []string) error {
			if g.Yes {
				return model.Errorf(model.ExitUsage, "link cannot pin --yes")
			}
			return createLink(c.Arg("site").String(), opt)
		},
		Subs: []*gcli.Command{
			{Name: "ls", Desc: "List generated shortcuts", Func: func(*gcli.Command, []string) error { return listLinks() }},
			{Name: "rm", Desc: "Remove a generated shortcut", Config: func(c *gcli.Command) {
				c.AddArg("name", "Shortcut name", true)
			}, Func: func(c *gcli.Command, _ []string) error { return removeLink(c.Arg("name").String()) }},
		},
	}
}

// linkRecords adapts the persistent state without adding a config dependency to link.
func linkRecords(st *config.State) link.Records {
	records := make(link.Records, len(st.Links))
	for name, rec := range st.Links {
		records[name] = link.Record{Path: rec.Path, Hash: rec.Hash, Site: rec.Site}
	}
	return records
}

func saveLinks(d *wiring.Deps, records link.Records) error {
	links := make(map[string]config.LinkRec, len(records))
	for name, rec := range records {
		links[name] = config.LinkRec{Path: rec.Path, Hash: rec.Hash, Site: rec.Site}
	}
	d.State.Links = links
	return config.SaveState(d.Dir, d.State)
}

func createLink(site string, opt *linkOptions) error {
	if err := link.Names(site, opt.name, opt.env, opt.auth); err != nil {
		return err
	}
	if opt.name == "" {
		return model.Errorf(model.ExitUsage, "--name is required")
	}
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	s, err := siteLookup(d, site)
	if err != nil {
		return err
	}
	if opt.env != "" {
		if _, ok := s.Envs[opt.env]; !ok {
			return model.Errorf(model.ExitLocal, "site %q has no environment %q", site, opt.env)
		}
	}
	if opt.auth != "" {
		if _, ok := s.Auths[opt.auth]; !ok {
			return model.Errorf(model.ExitLocal, "site %q has no auth %q", site, opt.auth)
		}
	}
	dir := opt.dir
	if dir == "" {
		dir = d.Cfg.Defaults.LinkDir
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return localError(err)
		}
		if runtime.GOOS == "windows" {
			dir = filepath.Join(home, "bin")
		} else {
			dir = filepath.Join(home, ".local", "bin")
		}
	}
	if strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return localError(err)
		}
		dir = filepath.Join(home, dir[2:])
	}
	records := linkRecords(d.State)
	paths, err := link.Create(dir, opt.name, site, opt.env, opt.auth, &records)
	if err != nil {
		return err
	}
	if err := saveLinks(d, records); err != nil {
		return err
	}
	for _, path := range paths {
		if err := writeString(os.Stdout, path+"\n"); err != nil {
			return err
		}
	}
	if runtime.GOOS == "windows" {
		return writeString(os.Stdout, ".cmd 版传 JSON 参数请用 -d @file\n")
	}
	return nil
}

func listLinks() error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(d.State.Links))
	for name := range d.State.Links {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rec := d.State.Links[name]
		if err := writeString(os.Stdout, fmt.Sprintf("%s\t%s\t%s\n", name, rec.Site, rec.Path)); err != nil {
			return err
		}
	}
	return nil
}

func removeLink(name string) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	records := linkRecords(d.State)
	if err := link.Remove(name, &records); err != nil {
		return err
	}
	return saveLinks(d, records)
}
