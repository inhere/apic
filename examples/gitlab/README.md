# GitLab example

A light read-only GitLab site built from a [simple definition](../../README.md#simple-definitions).

```bash
dir=$(apic config path)                 # your apic config directory
mkdir -p "$dir/defs" "$dir/sites"
cp defs/gitlab.yaml  "$dir/defs/"
cp sites/gitlab.yaml "$dir/sites/"
echo 'GITLAB_TOKEN=glpat-...' >> "$dir/.env"   # read_api scope is enough

apic config check
apic gitlab version
apic gitlab projects --search apic --per_page 5 --pick '*.{id,path_with_namespace}' -o table
apic gitlab mrs --id 123 --state opened --pick '*.{iid,title,state}' -o table
apic gitlab projects --auth anon --search apic --per_page 3   # public data, no token
apic gitlab branches --id 123 --all-pages --pick '*.name'      # every page, merged
```

For a self-hosted GitLab change `base_url` in `sites/gitlab.yaml`, or add
another environment, e.g. `self: {base_url: https://gitlab.example.com/api/v4, auth: pat}`,
and pick it with `--env self`.
