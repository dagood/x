# Maintaining a modified x/crypto for swap-in use

These are rough notes I took while looking into forking x/crypto and going over the ideas with the team.

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
We need some way to have a "raw" copy.

* We could have the patching tool create a copy during `git go-patch extract` so it's automatically maintained when changes are made. Example: [submodule](submodule/).
    * Complicated tooling that has to be used during ordinary development cycles to "apply" changes to the module isn't ideal for a shared project. We can at least add a check in CI to point out when it hasn't been done properly, and mention the step needed to take changes applied to the copy and convert them into changes to the patch.
* We could also periodically "publish" to an external repo that only contains the end result.
* Best, we think: we could generate the end result directly in our toolset's vendoring directory.
    * This is also good because it keeps the module as an implementation detail of a specific toolset. The repo binds tightly to specific Go toolset backend internal APIs. If we can prevent someone from referencing it outside this context, that's good for everyone.
    * We expect that each fork will have different internal calls. So: we can share some basic patches, but patch in our internal implementation on top of that with additional patches just before the result is created in our vendor directory.

### Multi-repo

One repo maintains the logic that can take x/crypto and produce a Go module codebase.
Another repo gets a "baked" commits on top of each upstream tag.
Those can be referenced by `go.mod`.

If our tests don't work on a new baked commit, the infra needs to be fixed up.

This has a similar feel to https://github.com/golang-fips/go.
In theory this can use patch files, but other approaches would also fit in nicely and could be more flexible when it comes to conflicts.
Or: generate some patch files.

## Wrapper

We could make a wrapper around x/crypto that has the same API as x/crypto.
The wrapper would internally fall back to standard x/crypto if the build isn't using a backend.

We could probably generate wrapper code and provide places where humans can fill in replacements for specific functionality.

⚠️ This may cause more work.
In some cases (e.g. `pkcs12`) the package with the public API doesn't do the crypto work, it's all in an internal package.
The public API doesn't need to be rewritten for FIPS compliance, just the internal package.
To avoid forcing a human to rewrite the outer code in the wrapper layer, the wrapper generator needs to be very smart or it needs the edge cases to be specified by hand--and it still needs to be smart to generate the right end result given edge case rules.
(With a direct fork or patch files, the internal implementation can simply be replaced without any fuss.)

Notes:
* Can we fall back to the crypto module at its ordinary location?
    * No: "replace x/crypto with golang-fips/xcrypto then golang-fips/xcrypto uses x/crypto" is a circular dependency.
        * ```
          $ go run .
          package cryptowrap-example
                  imports golang.org/x/crypto/sha3
                  imports golang.org/x/crypto/sha3: import cycle not allowed
          ```
    * Make a local copy as e.g. `internal/x/crypto`? Vendoring somehow?
        * We still need to maintain/update that. Similar effort as maintaining a fork.
* Are there any scenarios that are impossible to pass through using a wrapper?
    * x/crypto is changed into a wrapper when the impl moves to the standard library, so surely we can do this too?
    * We need to be careful with passthrough.
        * If we define an error in the wrapper's package and x/crypto returns an error defined in its own package, they need to be identical so `errors.Is` works in the user's code.

# Swapping it in

We have users who expect to be able to use the fork without changing the code they are building.
Could we vendor one version of our x/crypto fork into the Go toolset and have all usage point to that one version?

Something we have in mind is adding a GOEXPERIMENT/tag that automatically adds a `replace` directive pointing to the vendored library.
(Along with other work to make this possible. The vendoring prefix for the standard library may add difficulties.)

We could use go build's `-modfile file` to make this happen.
Read the existing go.mod (or what was already passed to `-modfile` by the caller), write a new go.mod in temp with the `replace` appended, then pass that in as `-modfile`.
This seems like it will tie in nicely with the rest of the build system: it would show up in buildinfo and work in other places we don't even know about.

A deeper patch might work and might seem cleaner, but may miss important details and would likely be harder to trace by inspecting the Go binary and other logs.

No matter how this is done, if we end up with an x/crypto fork being shipped along with the toolset, we need to be sure we can update it along with each toolset release, and make sure that the inability to do updates *without* updating the toolset doesn't cause problems.
We don't support people using out-of-date major versions of Go, but maybe not being able to get x/crypto updates is bad enough that we will be asked to support old versions for this reason?

## One version

Can we rely on x/crypto APIs being backward compatible?
~~Consider finding policy or evidence of this.~~
All but assured: https://github.com/golang/go/issues/56325.

> In practice, we've been upholding the Go Compatibility Promise in x/crypto for years, so we're effectively already at v1, we just need to call it that.

We need to keep up with the latest x/crypto versions and ensure this is true

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
