## 1. Независимые fixtures

- [x] 1.1 Добавить один accepted workflow folder и четыре negative authoring
  cases с author-visible YAML и `expect.json`; проверить, что они не зависят
  от Go unit fixture.

## 2. Black-box проверка

- [x] 2.1 Добавить standard-library verifier, который запускает собранный CLI
  для каждого case и проверяет accepted sealed package либо declared refusal.
- [x] 2.2 Включить verifier в `make e2e` и обновить `test/README.md` с его
  независимой границей.

## 3. Приёмка

- [x] 3.1 Выполнить verifier, `make e2e`, OpenSpec strict validation и
  `git diff --check`; убедиться, что historical evidence не изменён.
