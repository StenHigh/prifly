## Why

Сейчас Pri-Fly можно получить только сборкой из исходников. Это мешает обычному
пользователю начать работу одной командой и не даёт безопасного, понятного
пути к новой версии.

## What Changes

- Ввести public release distribution: проверяемые архивы binary для поддержанных
  платформ, опубликованные как assets versioned GitLab Release.
- Добавить одну copyable команду установки с `curl`, которая ставит binary в
  user-writable каталог без `sudo`; это не будет неявным `curl | sh`.
- Добавить явную команду `prifly update`: она получает только latest stable
  tagged GitLab Release, проверяет его identity и целостность, затем атомарно
  заменяет binary управляемой установки. Автоматических обновлений и загрузки
  bytes из branch/commit не будет.
- Закрепить release manifest, подпись и trust boundary первого bootstrap.
  Сборка из исходников и чужой binary не становятся «управляемой установкой»
  и получают честный отказ от `prifly update`.
- Добавить воспроизводимые проверки release assets, installer и updater, а
  README перевести с source-only quick start на поддерживаемую установку.

## Capabilities

### New Capabilities

- `release-distribution`: Поставка signed GitLab Release, установка в каталог
  пользователя и ручное безопасное обновление Pri-Fly.

### Modified Capabilities

- `cli-protocol`: CLI получает versioned команду явного обновления с
  определёнными результатами и отказами.
- `delivery-roadmap`: В текущую post-RC последовательность добавляется
  высокоприоритетная release distribution до следующих расширений продукта.

## Impact

Затрагиваются Go CLI и его тесты, release tooling, GitLab CI/release
configuration, README, SECURITY policy и OpenSpec source map. Новых runtime
dependencies, сетевого control plane для Run и изменений saved Run contracts
не появляется.
