## Why

GitLab Free даёт namespace 400 compute minutes в месяц. Последний native
pipeline Pri-Fly израсходовал около 30 минут, из которых `make check` занял
28 минут; автоматический полный race-набор на каждом push исчерпает quota до
того, как команда успеет сделать meaningful contributor work.

## What Changes

- Автоматический GitLab job запускает быстрые актуальные gates и e2e только
  для изменений продукта, контрактов или CI; обычная документация runner не
  получает.
- Полный `make race` остаётся воспроизводимой проверкой, но становится явным
  manual job для RC и release batch, а не скрытым расходом каждого push.
- `make check` не меняет свой смысл и остаётся полным локальным/выпускным
  gate.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `delivery-roadmap`: native GitLab verification различает быстрый
  автоматический feedback и явный полный race gate перед RC/release.

## Impact

Изменяются `.gitlab-ci.yml`, `Makefile`, текущий delivery roadmap и внешняя
документация проверки. Runtime, YAML authoring, sealed package bytes, Runs и
зависимости не меняются.
