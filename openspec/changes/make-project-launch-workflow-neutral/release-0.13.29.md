# Выпуск Pri-Fly 0.13.29

Блокирующая починка, найденная пилотом чтением кода на первом старте после
паузы (#137, 17.09).

## Что было

`project start` отказывал `invalid_input: The command could not be applied.
Check its arguments and selected files.` — без claim'а, без пути, без слов
git. Причина: старт освобождает claim прошлого Run'а и снимает его worktree
командой `git worktree remove --force`; GUI-обёртка владельца (supacode)
лочит все worktree репозитория, какие видит, включая claim'ы движка — через
секунды после создания (`.git/worktrees/<имя>/locked`, `"owner":"supacode"`).
git отказывает «cannot remove a locked working tree», `e.git` заворачивает
stderr в обычную ошибку, `ProblemFor` отдаёт родовой отказ. Claim оставался в
`releasing`, каждый следующий старт того же репозитория падал так же, и
`claim release` упирался в тот же лок — штатного выхода не было.

## Что стало

- Каталог claim'а проверен по inode как свой, поэтому лок чужого
  инструмента снимается вторым `--force` (`git worktree remove --force --force`).
  Тест: `git worktree lock --reason supacode` на claim-worktree → release
  проходит, репозиторий свободен; разрез с одним `--force` — прежний отказ git.
- Отказ, который git всё же даёт (в тесте — parent без прав на запись),
  становится `claim_worktree_removal_failed` с id claim'а, путём и словами
  git, с выходом руками; разрез — ровно прежний `invalid_input … Check its
  arguments`.
- Запись в `examples/troubleshooting.md` с выходом на любой версии:
  `git worktree unlock <путь claim>` и повторить старт.

Почему release 13.09 записал fence и не записал complete до появления
лока — не установлено; теперь любой такой отказ будет назван.

## Ворота

`make check` (race), `make e2e`, CI `verify`.
