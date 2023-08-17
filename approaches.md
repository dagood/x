# Maintaining a modified x/crypto for swap-in use

## Fork

More about why microsoft/go uses a "submodule fork" at https://github.com/microsoft/go-infra/tree/main/docs/fork.

### Ordinary fork, "branch-style" fork

Make diverging commits, merge in upstream periodically.
Conflicts manually maintained over time.
No patch files.

#### One merge commit per upstream commit?
We should consider only merging one upstream commit per merge commit.
Otherwise, if upstream makes two commits then tags the older of the two, we might end up in a situation where it's harder to go back in time and make a release based on that tag.
Basically, the situations listed here: https://github.com/microsoft/go/tree/microsoft/release-branch.go1.17/eng/doc/branches.

With a low-volume like x/crypto it's not as much as a concern to me as it is with the Go repo.
We can pretty easily see history and create a new branch at the right point and re-merge, and we probably wouldn't have to redo all that much conflict resolution work.

### Submodule fork

Use a submodule to track the upstream repo.
Use patch files to hold the changes.

Isn't all that compatible with Go modules.
A Go module download doesn't include the submodule, and certainly doesn't know how to apply loose patch files on top.
We need some infra to create a "raw" copy of the submodule+patches in the repository for Go to use.
Example: [submodule](submodule/).

Tooling that has to be used during ordinary development cycles to "apply" changes to the module isn't ideal for a shared project.

### Multi-repo

One repo maintains the logic that can take x/crypto and produce a Go module codebase.
Another repo gets a "baked" commits on top of each upstream tag.
Those can be referenced by `go.mod`.

If our tests don't work on a new baked commit, the infra needs to be fixed up.

This has a similar feel to https://github.com/golang-fips/go.
In theory this can use patch files, but other approaches fit in nicely and would be more flexible when it comes to conflicts.

## Wrapper

We could make a wrapper around x/crypto that has the same API as x/crypto.
The wrapper would internally fall back to standard x/crypto if the build isn't using a backend.

We could probably generate wrapper code and provide places where humans can fill in replacements for specific functionality.

TODO:
* Can we fall back to the crypto module at its ordinary location?
    * No: "replace x/crypto with golang-fips/xcrypto then golang-fips/xcrypto uses x/crypto" is a circular dependency.
    * Make a local copy as e.g. `internal/x/crypto`? Vendoring somehow?
        * How do we maintain/update *that*?
* Are there any scenarios that are impossible to pass through using a wrapper?
    * x/crypto is changed into a wrapper when the impl moves to the standard library, so surely we can do this too?

# Swapping it in

We have users who expect to be able to use the fork without changing the code they are building.
Could we vendor one version of our x/crypto fork into the Go toolset and have all usage point to that one version?

Something we have in mind is adding a GOEXPERIMENT that automatically adds a `replace` directive pointing to the vendored library.
(Along with other work to make this possible. The vendoring prefix for the standard library may add difficulties.)

No matter how this is done, if we end up with an x/crypto fork being shipped along with the toolset, we need to be sure we can update it along with each toolset release, and make sure that the inability to do updates *without* updating the toolset doesn't cause problems.
E.g. we don't support people using out-of-date major versions of Go, but maybe the situation is significantly worse if we make it so they can't get security fixes to x/crypto?

## One version

Can we rely on x/crypto APIs being backward compatible?
~~Consider finding policy or evidence of this.~~
All but assured: https://github.com/golang/go/issues/56325.

> In practice, we've been upholding the Go Compatibility Promise in x/crypto for years, so we're effectively already at v1, we just need to call it that.

## One wrapper with version-specific behavior

We could have the toolset specify a tag with the requested version of x/crypto to enable/disable very specific behaviors, if there are very subtle quirks we must emulate.

## Multiple versions

Perhaps we could vendor multiple versions.

# Accessing the internal FIPS backend

Generate code that accesses the internal FIPS backend through `//go:linkname`?

Seems specific to the way FIPS is implemented--branch-specific and fork author specific.
Use tags to pick an implementation based on who's using the repo?
Does this make it too hard to share?

Is there anything we can do on the microsoft/go side (and other forks) to make this more pluggable?
