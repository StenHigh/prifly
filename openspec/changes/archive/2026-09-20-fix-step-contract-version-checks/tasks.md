## 1. Правка

- [x] 1.1 `stepVersionAtLeast` в `internal/flow/compile.go`; оба правила
  `checkWorkspaceTrees` спрашивают «не меньше», а не «ровно». Тексты отказов
  «v5 or newer» и «v8 or newer».

- [x] 1.2 `TestAMaterializeOnlyTreeSurvivesALaterStepContract` воспроизводит
  случай сессии пакета дословно: v9 + materialise-only дерево + профиль.
  Красный до правки тем же сообщением, что прислали они. Проверено, что тест
  ловит именно эту причину: с возвращённым `!= "8"` он падает снова.
  Удерживает и обратное — v7 форму без `output_port` по-прежнему не принимает.

- [x] 1.3 `examples/authoring/extension-authoring-reference.yaml`: короткие
  имена — хвост `id:` после последнего `/`, не имя файла. Названо следствие:
  переименование хвоста ломает ключи `settings`, смена namespace перед `/`
  безопасна. `python3 -B test/e2e/test_examples.py` — OK.

## 2. Ворота

- [x] 2.1 `make ci-check` rc=0 — vet на linux и darwin, fmt-check 302 файла,
  refusal-check 163, staticcheck 9 пакетов × 2 платформы; `make e2e` rc=0;
  `make race` rc=0 (`internal/flow` 29.5 с, `internal/runtime` 1134.9 с).

- [x] 2.2 Проверено на опубликованных бинарях решающей парой: тот же пакет
  (read-only гейт + materialise-only дерево + `model_profile`) на 0.13.41
  отказывается `invalid_workspace_tree ... requires StepDefinition v8`, на
  0.13.42 компилируется. Архив сошёлся с манифестом (`sha256:259a04f0…`).
