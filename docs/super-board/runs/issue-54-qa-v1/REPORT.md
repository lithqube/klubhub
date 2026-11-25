# QA v1 for #54 (docs only, no UI: screenshots omitted on purpose)
AC1: docs/adr/0007-one-shared-release-version.md states one shared v* version (Decision) and why (Context, Consequences, Alternatives); indexed in docs/adr/README.md.
AC2: docs/container-images.md release-tags bullet says the Promoter image of a release is a preview until the Promoter MVP.
Cross-check: containers-ci.yml and promoter-ci.yml both trigger on tags ['v*']; images klubhub-dj-api and klubhub-promoter-api match the ADR.
Link check: ADR link target from container-images.md and README.md exists.
Diff vs main: 3 files, +25 -1; no workflow change.
