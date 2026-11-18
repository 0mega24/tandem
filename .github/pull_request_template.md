## Summary

<!-- What does this PR change, and why? One or two sentences. -->

Related: <!-- issue, PR or spec section (docs/spec/...). Delete the line if there is none. -->

## Changes

<!-- The main points for a reviewer: new packages, commands, behavior, dependencies. -->

-

## Changes from the spec

<!-- What differs from docs/spec/ and why. The same rows go in docs/DESIGN.md "Changes from the spec". Write "None." if nothing differs. -->

None.

## Testing

<!-- The commands that you ran and their result. Add manual steps when a test cannot show the change. -->

- [ ] `make check`

## Checklist

<!-- Check what applies. Delete an item that does not apply. -->

- [ ] CHANGELOG.md `[Unreleased]` has the user-visible changes
- [ ] go.mod changed: `vendorHash` in nix/package.nix is updated (`make nix-vendor-hash`)
- [ ] Generated files come from `make generate`, not from hand edits
