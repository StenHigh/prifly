# Безопасность поставки Pri-Fly

Pri-Fly F1 — **core-local/cooperative**, один доверенный OS owner. Пользовательские executables имеют права этого пользователя. Immutable refs, scoped API и process ownership защищают от ошибок протокола; они не являются sandbox от процесса с прямым доступом к тем же файлам/UID. Не исполняйте недоверенные scripts, LLM-generated shell без проверки или команды с production credentials под видом изолированного workflow.

## Куда сообщать

Используйте **private vulnerability reporting** репозитория [StenHigh/prifly](https://github.com/StenHigh/prifly/security/advisories/new) (Security → Report a vulnerability); адресат — владелец `StenHigh`. Скрипты выпуска ничего не публикуют автоматически.

Отдельный публичный security email/bug bounty владелец не назначал; не выдумываем такой канал. Если у вас нет доступа к проекту, сначала согласуйте закрытый канал с владельцем. Не публикуйте secrets, `.prifly/installation.json`, bearer tokens, raw production artifacts или полную SQLite в публичных issues/чатах.

## Учётные данные выпуска

`PRIFLY_RELEASE_SIGNING_KEY` — secret environment `release` в GitHub Actions:
private Ed25519 key только для signing manifest. Он доступен единственному
publication job и только после approve владельца в этом environment.
`PRIFLY_RELEASE_PUBLIC_KEY` — repository variable; он встраивается в binary
при сборке и проверяет подпись при `prifly update`. Publisher credential —
job-scoped `GITHUB_TOKEN` с `contents: write` только в publication job;
долгоживущего publish token нет, а создание release tags `v*` ограничено
ruleset владельца. Не выводите ни одно значение в log, не используйте private
key локально и немедленно rotate его при подозрении на раскрытие.

Полезные данные: version/binary SHA, OS/arch, semantics/trust profile, минимальный sanitized workflow, command/Run/Attempt IDs, ожидаемое и фактическое поведение. Raw evidence передавайте только по согласованному закрытому каналу. Hash или скриншот не заменяет воспроизводимый сценарий, если его можно безопасно подготовить.

## Проверить подпись выпуска вручную

`prifly update` проверяет подпись сам. Проверка со стороны — аудит, безопасник
заказчика, кто угодно с публичным ключом — возможна, но **наивный способ даёт
«подпись неверна» на исправном выпуске**, и это стоило зависимой сессии получаса
и почти отправленного отчёта о несуществующем дефекте.

Причина: подписаны не байты опубликованного файла. `release-manifest.json`
записан как канонические байты **плюс концевой перевод строки**, а подписи
покрывают их без него.

Публикуются две подписи, над разными сообщениями:

- `release-manifest.sig` — над каноническими байтами, то есть содержимым
  `release-manifest.json` **без концевого `\n`**;
- `release-manifest.jcs.sig` — над формой RFC 8785 (JCS) того же документа,
  которую любой читатель вычисляет из файла сам. Это подпись, предназначенная
  для внешней проверки: она не требует воспроизводить порядок полей чужой
  реализации.

Байты этих двух сообщений **не совпадают**, поэтому подпись и сообщение нельзя
переставлять местами.

Ключ — публичная переменная репозитория `PRIFLY_RELEASE_PUBLIC_KEY` (hex).

```python
import base64, json
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

key = Ed25519PublicKey.from_public_bytes(bytes.fromhex(PUBLIC_KEY_HEX))
raw = open("release-manifest.json", "rb").read()

# release-manifest.sig — канонические байты без концевого перевода строки
key.verify(base64.b64decode(open("release-manifest.sig").read().strip()),
           raw.rstrip(b"\n"))

# release-manifest.jcs.sig — форма RFC 8785, вычисленная из файла
jcs = json.dumps(json.loads(raw), sort_keys=True,
                 separators=(",", ":"), ensure_ascii=False).encode()
key.verify(base64.b64decode(open("release-manifest.jcs.sig").read().strip()), jcs)
```

Обе проверки должны пройти. `key.verify` бросает `InvalidSignature` при
несовпадении и ничего не возвращает при успехе.

После подписи сверьте SHA-256 архива с полем `sha256` соответствующего asset в
manifest — подпись удостоверяет manifest, а не архив.

## Ограничить проблемную сборку

1. Прекратите **новые** Start/Drive/admissions. Сохраните version, SHA и status/receipts. Не удаляйте историю.
2. Для ещё живого собственного driver используйте pause либо cancel по задаче. Сообщение об отправке сигнала не означает settlement. При uncertainty не запускайте повтор.
3. Сохраните authority и внешние эффекты для разбора. Не редактируйте snapshots, stop generations, dedup receipts или lock-файлы вручную.
4. Соберите/получите исправленный проверенный binary, сверьте checksum, notices, capabilities и compatibility evidence. Сначала проверьте на отдельной пустой authority и тестовых данных.
5. `prifly update` меняет только официально установленный binary после проверки signed release; он не меняет authority, package, configuration или уже загруженные bytes работающего driver. Не начинайте новую работу другой версией против той же authority, пока не понятен статус текущей; automatic downgrade, force-upgrade и удаление несовместимых событий не поддержаны.
6. Если подозрение падает на проверку схем, `PRIFLY_SCHEMA_WORKER=1` возвращает валидацию каждого значения в отдельный helper с жёстким deadline — так работали прежние релизы. Это временный клапан на один релиз, а не настройка.

Pri-Fly не содержит remote kill switch, network control plane или автоматического исправления state. `prifly update` запускается только явно, а первая bootstrap-установка доверяет official GitHub HTTPS Release asset; не заменяйте этот адрес branch, job artifact или сторонним URL. Это и есть вся граница доверия первой установки: HTTPS до GitHub. Подписи проверять нечем — ключа на машине ещё нет, — поэтому installer делает единственное, что здесь имеет смысл: скачивает `release-manifest.json` того же Release и сверяет SHA-256 архива до установки, отказываясь при несовпадении. Это ловит обрезанную загрузку, устаревшее зеркало и подменённый asset, но не скомпрометированный GitHub-аккаунт. `prifly update`, у которого ключ уже есть, проверяет подпись manifest. Полный rollback всех файлов по прежнему пути не распознаётся без внешнего fencing и не является разрешённым recovery. Новая версия продукта не снимает прежние restrictions сама.
