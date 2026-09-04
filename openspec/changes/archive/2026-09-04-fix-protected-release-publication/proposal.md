## Why

Тег `v0.3.0` прошёл required product gates, а signed assets успешно
собрались и загрузились. Последний job отказал: текущий publisher token с
ролью Developer не может создать GitLab Release для protected tag `v*`,
который создают только Maintainers. Публичная поставка осталась незавершённой.

## What Changes

- Закрепить, что отдельный protected publisher token для protected release
  tags должен иметь роль Maintainer и `api` scope; signing key остаётся
  отдельным credential.
- Добавить в release-publish preflight явную проверку прав token до попытки
  создать Release, с диагностикой вместо неясного отказа GitLab API.
- Добавить статическую проверку CI contract и привести current release
  specification в соответствие с работающим GitLab permission model.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `release-distribution`: publication credential и release job должны быть
  совместимы с protected-tag policy и отказывать до publication при
  недостаточных правах.

## Impact

Затрагиваются `.gitlab-ci.yml`, локальная проверка CI-contract и текущая
OpenSpec specification release distribution. Runtime, YAML authoring,
сохранённые Runs и уже опубликованный `v0.2.0` не меняются.
