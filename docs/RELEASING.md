# Quantum Runtime release process

Quantum Runtime releases are generated from the repository `VERSION` file.

## Release trigger

A merge to `main` that changes `VERSION`, `CITATION.cff` or the release workflow starts `.github/workflows/release.yml`.

The workflow:

1. validates `VERSION`
2. runs the normal Runtime verification suite
3. builds static Linux archives for `amd64` and `arm64`
4. packages the project legal and notice files with each binary
5. writes `SHA256SUMS`
6. creates tag `v<VERSION>` and the matching GitHub Release if that release does not already exist
7. marks versions containing a hyphen, such as `0.2.0-alpha.1`, as GitHub pre-releases

GitHub also attaches its normal source archives to the release.

## Zenodo

When the repository is enabled in Zenodo's GitHub integration, the GitHub release event is the archival boundary. Zenodo receives the repository snapshot and can mint the persistent DOI for that software version.

`CITATION.cff` contains stable authorship and project metadata. It intentionally does not declare an SPDX license identifier because Quantum Runtime uses the custom Starlight Unit Studios Quantum Runtime Community Source License 1.0. The controlling license text remains `LICENSE.de.md`.

After the first Zenodo archive is created, verify the Zenodo record's displayed license and replace any automatically selected default with the custom project license in Zenodo metadata if necessary.

## Version bump rule

A normal development commit must not change `VERSION` just to force a release. Version changes belong in an explicit release-preparation pull request after the intended milestone is complete and CI is green.

If a matching GitHub Release already exists, rerunning the workflow is idempotent and exits without replacing the published release.
