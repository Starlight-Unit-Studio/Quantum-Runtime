# Quantum Runtime Third-Party Notices

Quantum Runtime project-owned code is governed by the license identified in `LICENSE_HISTORY.md`. Third-party material is not relicensed by that license.

## Current foundation

The `0.1.0-alpha.1` Go source uses the Go standard library and does not vendor an external Go module dependency.

Quantum Runtime can connect to an existing Ollama service in adoption mode. Ollama is not distributed by this repository and remains subject to its own license and notices.

Quantum Runtime does not currently distribute Gemma, other model weights, datasets, or a third-party inference engine. Any model or engine selected by an operator remains subject to its own terms, acceptable-use rules, attribution requirements, and distribution restrictions.

## Future bundled components

Before a release bundles a third-party library, inference engine, model artifact, font, package, or other component, the release process must record at least:

- component name and version
- source or project location
- applicable license identifier or license file
- required copyright and attribution notices
- whether modification or redistribution is permitted
- any model-specific or dataset-specific conditions

Component-specific license texts may be stored below `third_party/licenses/` in a future release. This file must be updated when bundled third-party content changes.
