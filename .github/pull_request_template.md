## Summary

-

## Safety checklist

- [ ] `plan` / `apply --dry-run` behavior is unchanged or tested.
- [ ] No silent SSH lockout-risk behavior was introduced.
- [ ] Existing host config files are preserved or explicit confirmation is required.
- [ ] New commands use argv execution or justify shell usage.
- [ ] User-facing docs/examples are updated.

## Verification

```sh
go test ./...
go vet ./...
```

Docker smoke tests, if relevant:

```sh
tests/docker/run.sh
```
