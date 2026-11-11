package link

import (
	"fmt"
	"sort"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// shortcut is one shortcut that gets a completion function.
type shortcut struct{ name, site string }

// completionShortcuts returns the records that are safe to embed in a shell
// script: the name (file name without the .cmd suffix) and the site must both
// pass Names. Others are skipped. The result is sorted and has no duplicates.
func completionShortcuts(recs Records) []shortcut {
	seen := map[string]bool{}
	var out []shortcut
	for key, rec := range recs {
		name := strings.TrimSuffix(key, ".cmd")
		if seen[name] || Names(rec.Site, name, "", "") != nil {
			continue
		}
		seen[name] = true
		out = append(out, shortcut{name: name, site: rec.Site})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// CompletionScript returns the completion script of every valid shortcut for
// shell (bash, zsh or pwsh). The command is always the fixed name apic; the
// functions rewrite the typed words into `apic --in-completion <site> <words>`.
func CompletionScript(shell string, recs Records) (string, error) {
	var render func(shortcut) string
	var head string
	switch shell {
	case "bash":
		head, render = "# apic shortcut completion for bash; source it or eval it\n", bashFunc
	case "zsh":
		head, render = "# apic shortcut completion for zsh; source it or eval it (after compinit)\n", zshFunc
	case "pwsh":
		head, render = "# apic shortcut completion for PowerShell; run it or dot-source it\n", pwshFunc
	default:
		return "", model.Errorf(model.ExitUsage, "unsupported shell %q, use bash, zsh or pwsh", shell)
	}
	var b strings.Builder
	b.WriteString(head)
	for _, s := range completionShortcuts(recs) {
		b.WriteString("\n")
		b.WriteString(render(s))
	}
	return b.String(), nil
}

func funcName(name string) string { return "_apic_link_" + strings.ReplaceAll(name, "-", "_") }

func bashFunc(s shortcut) string {
	return fmt.Sprintf(`%[1]s () {
    local cur words
    cur="${COMP_WORDS[COMP_CWORD]}"
    # --opt=value is split by bash into "--opt" "=" "value"; a lone "=" means an empty value
    [ "$cur" = "=" ] && cur=""
    words=("${COMP_WORDS[@]:1:$COMP_CWORD}")
    local IFS=$'\n'
    COMPREPLY=( $(compgen -W "$(apic --in-completion %[2]s "${words[@]}" 2>/dev/null)" -- "$cur") )
}
complete -F %[1]s %[3]s
`, funcName(s.name), s.site, s.name)
}

func zshFunc(s shortcut) string {
	return fmt.Sprintf(`%[1]s () {
    local -a candidates
    candidates=("${(@f)$(apic --in-completion %[2]s "${words[@]:1}" 2>/dev/null)}")
    compadd -- $candidates
}
compdef %[1]s %[3]s
`, funcName(s.name), s.site, s.name)
}

func pwshFunc(s shortcut) string {
	return fmt.Sprintf(`Register-ArgumentCompleter -Native -CommandName '%[1]s' -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $words = @()
    for ($i = 1; $i -lt $commandAst.CommandElements.Count; $i++) {
        $words += $commandAst.CommandElements[$i].ToString()
    }
    if ([string]::IsNullOrEmpty($wordToComplete)) {
        $words += ''
    }
    & 'apic' --in-completion '%[2]s' @words 2>$null | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`, s.name, s.site)
}
