package link

// Scripts returns a sh shortcut and, on Windows, a cmd shortcut.
// Names must be validated before this function is called.
func Scripts(goos, site, env, auth string) map[string]string {
	args := " --api " + site
	if env != "" {
		args += " --env " + env
	}
	if auth != "" {
		args += " --auth " + auth
	}
	out := map[string]string{"": "#!/bin/sh\nexec apix" + args + " \"$@\"\n"}
	if goos == "windows" {
		out[".cmd"] = "@apix" + args + " %*\r\n"
	}
	return out
}
