## 1. Pipeline

- [x] 1.1 Добавить `.gitlab-ci.yml` с независимыми jobs `check` и `e2e`,
  использующими Go 1.27.0, Python и C compiler; проверить `glab ci lint`.
- [x] 1.2 Настроить cache только для `.cache/go-build` и `.cache/go-mod` и
  проверить, что pipeline не публикует artifacts, binary, Run state или
  historical evidence.

## 2. Проверка

- [x] 2.1 Выполнить `make check` и `make e2e` с CI-совместимым Go path и
  зафиксировать точные результаты.
- [x] 2.2 Push change, дождаться GitLab pipeline и подтвердить success обоих
  jobs через `glab ci`; pipeline `2809007025` подтвердил `check` и `e2e`.
  Проверены `git diff --check` и неизменность archived OpenSpec changes.
