# Registry files

CLIProxyAPI's plugin registry schema does not define localized fields. This project therefore includes three valid schema v1 files:

- `registry.json`: bilingual Chinese/English listing, suitable as the default custom registry.
- `registry.zh-CN.json`: Chinese-only listing.
- `registry.en.json`: English-only listing.

Before publishing, replace every occurrence of:

```text
https://github.com/OWNER/warp-egress
```

with the actual public GitHub repository URL.

For the official plugin store, normally submit only the plugin object from `registry.json` to the store's root `registry.json`.

The GitHub release tag should be `v0.2.0`. The Linux AMD64 release asset must be named:

```text
warp-egress_0.2.0_linux_amd64.zip
```

The zip root must directly contain:

```text
warp-egress.so
```

The same release must include `checksums.txt` in standard `sha256sum` format.
