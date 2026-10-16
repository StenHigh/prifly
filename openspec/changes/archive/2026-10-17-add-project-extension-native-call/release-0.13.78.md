# Выпуск Pri-Fly 0.13.78: Core и native Call extensions

Published stable: [v0.13.78](https://github.com/StenHigh/prifly/releases/tag/v0.13.78), `2026-10-16T21:54:51Z`.
Тег закреплён на `201d4428b60e6afc9aef84fc5197a3185cf2f20b`. Implementation #9 — `963b9b579af0c13298d0dbb75cce0e687b4ca439`; дополнительный `201d4428` обновляет только stale editor-test expectations и фиксирует этот regression в evidence.

В main также интегрированы #8 (unbound claim ownership scan) и #7 (reviewed finite storage budget recovery); их changes уже архивированы. Их исходные пользовательские authority/Run не открывались и operational recovery не выполнялся.

## Required exact-SHA gates и публикация

- [verify](https://github.com/StenHigh/prifly/actions/runs/38970868425): PASS, прямой полный batch на release SHA.
- [qualify](https://github.com/StenHigh/prifly/actions/runs/38970886907): PASS, race suite и полный e2e на том же SHA.
- [release](https://github.com/StenHigh/prifly/actions/runs/38972184928): PASS, native linux/amd64 и darwin/arm64 builds, macOS init/doctor qualification, signature и publication.
- Пользователь явно поручил merge и release; environment `release` одобрен через owner account после зелёных проверок и обеих сборок. Signing key оставался внутри protected job, локально не читался.

Первый candidate `963b9b57` не был опубликован: его qualification выявила устаревший editor inventory в `test/e2e/test_examples.py`. Local RED воспроизведён, ожидания приведены к selecting extension schema, uniqueness/modeline проверены. Затем staged CI и полный local e2e PASS; для нового SHA required gates выполнены заново. Старое падение не использовано как успешное evidence.

## Signed assets

| Platform | Archive | SHA256 |
|---|---|---|
| darwin/arm64 | `prifly-darwin-arm64.tar.gz` | `380971230436aae69fe240efb62aaf1a00cef71c47d9a31710579beae200d443` |
| linux/amd64 | `prifly-linux-amd64.tar.gz` | `dd885053538bdc8514c6542519da3acf777deef3181585853e3cf660b2510369` |

`release-manifest.json`: `sha256:eef734f13f5e9820a38826828c16f0b9fdee7576415874a716aba7fb1c2998d0`.
RFC 8785 canonical bytes и Ed25519 `release-manifest.jcs.sig` проверены независимо с существующей jsoncanonicalizer library и public key `521250b29cb38544a7ec7fc1a2ee12b738cbcc3aca78d3bdc860ea438b5435b4`. Оба archive digest совпали с подписанным manifest; matrix ровно linux/amd64 + darwin/arm64, stable version 0.13.78.

Официальный installer из этого Release установил binary в отдельный temporary user directory. `version --json`: **0.13.78**, linux/amd64, Go 1.27.2. Binary SHA256: `sha256:60a6dd1ca0524b4927418a64311207b4d6739080a8f518374d1889726d68ab5f`. Signed `update --json` через latest stable вернул `updated=false`, version 0.13.78; bytes не изменились. Установленный CLI пользователя и его authority не менялись.

## Проверки exact released CLI

- Public E5 harness `python3 -B test/e2e/verify-call-extensions.py --binary RELEASED_BINARY`: **51/51 PASS**, exit 0. Тот же harness и fixtures, binary взят из подписанного stable GitHub Release, а не из source-built candidate. Raw log `/tmp/prifly-release-0.13.78-e5.log` сохранён без переписывания; его generic scope label не является доказательством origin — origin/version/hash подтверждены manifest, installer и отдельным version read выше. Это released CLI / synthetic native-session evidence, не live actor qualification.
- Legacy public authoring corpus / workspace modes / workflow catalog: PASS, `/tmp/prifly-release-0.13.78-authoring.log`.
- Проверены реальные inputs/exports, invocation/claim ancestry, outcomes/errors/timeouts/cancel, shared ceilings/resources, review freshness, accepted-prefix reuse и whole-child replay, compile refusals. Полная traceability E5-A1…A10 остаётся в [implementation-evidence.md](implementation-evidence.md).
- Source-built finite-storage и inventory regressions входят в exact-SHA Go/race gates; public E5 не подменяет operational recovery #7/#8.

Author index/reference теперь называют подтверждённую минимальную CLI version **0.13.78+** вместо development label. Marker `prifly-extension/2`, Project `/3`, native Call contract minima и legacy v1 semantics сохранены.

## Открытая приёмка и handoff

**7.1 выполнен.** 7.2 остаётся pending: synthetic generated-runner cases трёх hosts прошли, отдельный live handoff/control check по каждому host здесь не выполнялся. 7.3 остаётся pending для итоговой consumer-приёмки/handoff W6. Это не исключение hosts из scope и не закрытие E5 release acceptance, issue #9 или продуктового milestone.

Для последующей независимой consumer-приёмки: released Core **v0.13.78**, source SHA `201d4428b60e6afc9aef84fc5197a3185cf2f20b`, assets и signatures — по ссылке stable Release выше; native call authoring — `examples/authoring/extension-authoring-reference.yaml`. Full native plan сохранён до будущего archive change.
