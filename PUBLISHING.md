# Publishing

The repository includes CI and signed release automation. No release, signing key, or Registry registration is created by this setup.

**Public Registry blocker:** GitHub repository `Cloady/terraform-provider` must be renamed to `Cloady/terraform-provider-cloady` and be public before the Terraform Registry can discover it. The Registry requires the repository name `terraform-provider-{NAME}` in lowercase. The intended provider address is `registry.terraform.io/cloady/cloady`. See [HashiCorp's publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing).

## One-time setup

1. Rename the repository in GitHub Settings and point the local origin at `git@github.com:Cloady/terraform-provider-cloady.git`. The Go module path, imports and artifact names already use the final name. The release workflow derives the target repository from its checkout.
2. Allow GitHub Actions and the pinned actions in the repository/organization settings. Configure protection for the default branch and version tags so only release maintainers can create tags.
3. Choose a Cloady-owned GPG signing key, or have a release maintainer create one using [HashiCorp's signing-key instructions](https://developer.hashicorp.com/terraform/registry/providers/publishing#preparing-and-adding-a-signing-key). **The key must be RSA or DSA — the Registry API rejects ECC**, which is what a modern `gpg --full-generate-key` produces by default; generate it non-interactively with `Key-Type: RSA` and `Key-Length: 4096`. GoReleaser must also emit a binary signature, not an armored one, so never add `--armor` to the `signs` args. Retain the key and its recovery material securely; use the same key for future releases.
4. In repository Settings → Secrets and variables → Actions, add `GPG_PRIVATE_KEY` containing the ASCII-armored private signing key and `PASSPHRASE` containing its passphrase. Export the private key directly into a secure secret-entry workflow; do not commit it or print it in CI. `GITHUB_TOKEN` is supplied by GitHub automatically. The GPG import action unlocks the key for the signing step and removes it afterward.
5. Sign into the [Terraform Registry](https://registry.terraform.io/) with a GitHub account that can administer the Cloady repository. Add the corresponding ASCII-armored **public** key under the namespace's signing keys. If Cloady uses an HCP-managed Registry namespace, use that namespace's publishing permissions.

## Verify locally

Install the Go version from `go.mod`, Terraform 1.16.1, and [GoReleaser 2.18.1](https://github.com/goreleaser/goreleaser/releases/tag/v2.18.1). From a Git checkout with at least one commit:

```sh
make check
make testacc
terraform fmt -check -recursive examples
make release-check snapshot
```

The acceptance tests use an in-process HTTP API fixture and the real Terraform CLI; they require no Cloady credentials or cloud resources. The snapshot creates all six platform archives and checksums in `dist/` without signing or publishing. A snapshot is a packaging check, not a Registry release.

CI runs these same checks for pull requests and branch pushes. Each version tag runs them again before the release job can reach the GPG key. Actions are pinned to major-version tags and GoReleaser to `~> v2`; the acceptance job pins Terraform exactly. Pin actions to commit SHAs if you want the stronger supply-chain guarantee.

## Release

After setup and a passing CI run, tag the commit to release with a new semantic version:

```sh
git tag -a v0.1.0 -m 'Release v0.1.0'
git push origin v0.1.0
```

Pushing the tag starts the Release workflow. GoReleaser validates the version, builds static binaries for Linux, macOS, and Windows on amd64 and arm64, and publishes a GitHub Release containing:

- Six `terraform-provider-cloady_0.1.0_OS_ARCH.zip` archives, each containing `terraform-provider-cloady_v0.1.0` (with `.exe` on Windows).
- `terraform-provider-cloady_0.1.0_manifest.json`, advertising Terraform protocol 6.
- `terraform-provider-cloady_0.1.0_SHA256SUMS`, covering every ZIP and the manifest.
- `terraform-provider-cloady_0.1.0_SHA256SUMS.sig`, a detached GPG signature over the checksums.

For the first release, choose **Publish → Provider** in the Terraform Registry, select the renamed repository, and complete registration. This creates the GitHub release webhook used to ingest subsequent versions. Check that `cloady/cloady` shows the new version and that `terraform init` installs it before announcing availability.

Prerelease tags such as `v0.2.0-rc.1` create prereleases. Never move a released tag or replace published binaries: Terraform users retain checksums in their lock files. Release a new version for corrections, including documentation fixes. If publishing fails, inspect the workflow and the Registry's webhook response; a GitHub release alone does not mean the Registry has accepted the provider.
