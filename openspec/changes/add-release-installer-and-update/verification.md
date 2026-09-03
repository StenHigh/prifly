# Verification

Локальная implementation verification и public qualification выполнены
2026-09-01. Public result относится только к named `linux/amd64` asset
`v0.2.0`, а не к неподдержанным platform или whole-product release.

| Проверка | Результат |
|---|---|
| `go test ./internal/release ./cmd/prifly ./internal/runtime -run 'TestGlossaryBindings|TestCLIUpdateHasStructuredResultAndRejectsArgumentsBeforeNetwork'` | passed |
| `go test ./internal/release ./cmd/prifly ./internal/runtime -run 'TestGlossaryBindings|TestCLIUpdateHasStructuredResultAndRejectsArgumentsBeforeNetwork|TestUpdate|TestInstaller|TestBuild'` | passed |
| `ruby -e 'require "yaml"; YAML.load_file(".gitlab-ci.yml")'`, `sh -n scripts/install.sh`, `git diff --check` | passed |
| Local `go build` + `go run ./cmd/release-build` with a public RFC 8032 test key | passed; created `linux/amd64` assets only in a temporary directory, publication was not attempted |
| `make ci-check` | passed; `internal/runtime` — 428.555s; schema checks passed |
| `make e2e` | passed; examples — 7 tests, authoring — 5 cases, core — 169 commands, context — 75 commands / 10 cases |
| `openspec validate --specs` | 15 capabilities passed |
| `openspec validate add-release-installer-and-update --strict` | passed |
| GitLab MR !3 fast gates | `verify` passed за 377.562s; full manual `race` passed за 1242.780s на том же product source tree |
| GitLab protected tag pipeline `2810428152` | signed `release-build` и `release-upload` passed; он содержал только release jobs, без повторных product gates |
| Public GitLab Release | [v0.2.0](https://gitlab.com/stenhigh/prifly/-/releases/v0.2.0) создан с installer, manifest, signature и `prifly-linux-amd64.tar.gz` |
| Public `linux/amd64` Debian smoke | official `install.sh` установил binary; `prifly update` verified signed latest manifest и вернул `previous_version=version=0.2.0`, `updated=false` |

Первый `release-publish` job честно выявил ограничение protected tag: передача
`--ref` заставляла GitLab повторно создавать существующий tag. Owner создал
тот же public Release без `--ref`; current main убирает этот аргумент и
deprecated `filepath` alias для следующих версий. Отдельные public platform,
automatic update и whole-product qualification в этот срез не входят.
