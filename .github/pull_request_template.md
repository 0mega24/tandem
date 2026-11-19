## Summary

<!-- What does this PR change, and why? One or two sentences. -->

Related: <!-- issue, PR or spec section (docs/spec/...). Delete the line if there is none. -->

## Changes

<!-- List the changes a reviewer needs to know: packages, commands, behavior or dependencies. -->

-

## Changes from the spec

<!-- What differs from docs/spec/ and why. The same rows go in docs/DESIGN.md "Changes from the spec". Write "None." if nothing differs. -->

None.

## Testing

<!-- List each check command and its result. Add manual checks when tests do not cover the change. -->

- [ ] `make check`

## Checklist

<!-- Check what applies. Delete an item that does not apply. -->

- [ ] CHANGELOG.md `[Unreleased]` has the user-visible changes
- [ ] If `go.mod` changed, `vendorHash` in `nix/package.nix` is updated (`make nix-vendor-hash`)
- [ ] Generated files come from `make generate`, not from hand edits
