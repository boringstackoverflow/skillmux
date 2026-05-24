## Summary

What changed?

## Safety

- [ ] Does not touch real user home directories in tests.
- [ ] Preserves or explicitly migrates existing symlink topology.
- [ ] Backs up before replacing, moving, or relinking managed paths.
- [ ] Does not start managing runtime state such as sessions, logs, caches, auth files, telemetry, histories, or local databases.

## Tests

```bash
go test ./...
go vet ./...
```
