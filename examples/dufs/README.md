# dufs example

A [dufs](https://github.com/sigoden/dufs) file server as a site named `mirror`,
built from a [simple definition](../../README.md#simple-definitions). `make publish`
uses it to upload `dist/`.

```bash
dir=$(apic config path)                 # your apic config directory
mkdir -p "$dir/defs" "$dir/sites"
cp defs/dufs.yaml   "$dir/defs/"
cp sites/mirror.yaml "$dir/sites/"      # then change base_url
echo 'MIRROR_PASS=...' >> "$dir/.env"

apic config check
apic mirror health
apic mirror root --pick 'paths.*.name'
apic mirror ls --path tools/apic --pick 'paths.*.{name,size}' -o table
apic mirror find --path tools -p q=latest --pick 'paths.*.name'
apic mirror hash --path tools/apic/latest.yaml
apic mirror get --path tools/apic/latest.yaml > latest.yaml
apic mirror put --path tools/demo/a.zip -d @./a.zip
apic mirror rm --path tools/demo/a.zip
```

- `q` is a one-letter name, so it gets no option; pass it with `-p q=...`.
- Reads are often anonymous on dufs: `--auth anon` works for `ls`, `find` and `get`.
- `put --dry-run` prints the whole file as `--data-raw`; avoid it for binaries.
- A second server is another environment, e.g.
  `site2: {base_url: http://mirror2.example.com, protect: true, auth: rw2}`;
  with `protect: true`, `put` and `rm` need `--yes`.
