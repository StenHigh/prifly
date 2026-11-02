# Проверка эксплуатационного справочника

## Область проверки

База исходной инвентаризации: `2060fc9286cff2924e198b0294ba9a2814ad0e90`.
Перед финальной проверкой ветка включает `14145964b3c2fde3a450a11f8c4e8923922fd4d7` из main.
Инвентаризация: 119 dispatch entries, 220 declarations flags, 116 уникальных имён flags, 83 capabilities.
AST guard независимо читает handlers; help и capability guards читают фактическую публичную поверхность.
Намеренно потерянные queue add, state-root и installed hosting.md вызывают отказ соответствующего guard.
Guard проверяет наличие; смысл условий flags и версионных ограничений также проверен по handlers/schema/help.

Существующий prifly-run остаётся единственным skill. Репозиторий хранит canonical страницы в examples/operations.
CLI встраивает страницы и существующие authoring references стандартным embed.
Установка доставляет references/manual, references/examples и receipt со списком owned digests.
Runtime/wire contract, schemas, старые templates и прошедшие Runs не меняются.

## Практическая матрица

CLI fixtures создают отдельные target repositories и authority. Они копируют только declared examples, не Core checkout.
Go harness собирается из Core; оператор не использует исходники Core как инструкцию публичной процедуры.
Отдельный агент работал только с установленной поставкой и public CLI. Его raw report: [forward-evaluation.json](forward-evaluation.json).
Этот проход отдельно проверяет H03/H15/H16, отказ до нового запуска и чтение authoring contracts.
Остальные сложные состояния воспроизводят существующие автоматические fixtures ниже.
Это не утверждение о 16 независимых слепых агентных исследованиях.

| ID | Проверка | Наблюдаемый критерий |
|---|---|---|
| H01 | TestCLIProjectQuestionnaireTypedSelections, TestProjectStandingAnswersInExtendYAML, TestProjectCompositionPrepareAndStartMatchPublishedViews, TestProjectLaunchDigestFollowsTheSourceNotTheValue | Typed values, standing sources, prepare/start shape и отказ stale review; mandatory update расположен перед prepare в runner; signed updater проверяется отдельно |
| H02 | TestCLIProjectCSVReportNoGitNoAI | Program-only Run без Git/AI, declared executable, результат CSV |
| H03 | TestSessionTaskLeavesTheHandoffAndItsReportShapeInReach; два successful Runs независимого агента | Task/take/template/submit, accepted pass, одинаковые bytes результата |
| H04 | TestCLIProjectMixedDecisionResumesWithoutGit, TestCLIProjectTimedMixedDecisionResumesWithoutGit | Digest/version, yield, ожидание и redelivery; timeout не становится ответом |
| H05 | TestCLIBlockedExampleAcceptance | blocked/partial, accepted handoff и declared route |
| H06 | TestCLIContinuationTakesOverTheSourceTree, TestCLIResumeChainNestedDirtyClaim, TestCLIProjectRecoverCarriesAcceptedStagesOntoACorrectedPackage | Prefix, claim и dirty tree сохранены |
| H07 | TestCLIReviewedCancelledWriterRecovery, TestCLIRecoverNestedPreparationPreservesDirtyClaimAndHistory | Owner resolution applied/not_applied перед recovery, история и файлы сохранены |
| H08 | TestProjectResourcesKeepTwoRunsOffOneStage, TestCLIStorageBudgetRecoveryPreservesSameRunAndWorkspace, TestAtomicMultiClaimTakesAllOrNothing | Holder, reviewed budget, all-or-nothing claims; нет ручного изменения authority |
| H09 | TestProjectLiveOutputFromBindingToRead, TestProgramProgressStates, TestRenderTimingNamesTheClockBehindEachNumber | Sealed output, freshness/unknown states и источник clock |
| H10 | TestTechnicalRetriesNeedTheStepAuthorsPermission, TestProjectExtensionPointsSealNestedEntryAndRepeatExport, TestEveryDeclaredCapabilityIsInTheAuthorIndex; installed lookup независимого агента | Минимальные контракты, bounds 1..8 и разрешённые retry classes, compile extension |
| H11 | TestProjectRunnerUpdateReplacesEveryReleasedRunner, TestRunnerManualInstallsForEveryHostAndRepairsMissingPages, TestRunnerManualUpgradePreservesCustomFilesAndRetiresOwnedPages | Три hosts, historical recognition, PROJECT/custom bytes и settings остаются |
| H12 | TestUpdateRejectsChangedManifestAndArchiveWithoutReplacingBinary, TestUpdateRefusesSourceBuildBeforeNetwork, TestUpdateRefusesCopiedBinaryBeforeNetwork, TestCLIProjectWorkflowsUpdateAndRemove; isolated copied CLI update | Нет замены при плохой подписи/receipt, modified upstream отказ; новый запуск останавливается перед prepare |
| H13 | TestCLIProjectWorkflowsSearchAndAddByCatalogEntry, TestCLIProjectWorkflowsUpdateAndRemove, TestProjectWorkflowSourceParsing | Recorded origin/ref, branch update, catalog selection и local source границы |
| H14 | TestCLIProjectQueueReviewRejectsStaleTaskBeforePreflightAndReplaysResult, TestCLIProjectQueuePartialCancelContinueKeepsTaskAndRunHistory | Receipt replay/reconcile не создаёт второй dispatch |
| H15 | TestInstalledRunnerLinksNeedNoCoreCheckout, TestOperationsHandbookCoversEveryCLIFlagAndHelpEntry, TestOperationsDispatchCoverageIncludesCommandsOmittedFromShortHelp; installed schema lookup | Missing page/entry обнаруживается; exact WorkspaceTreeLocation и WorkflowRevisionV5 доступны |
| H16 | Successful prestarted Runs независимого агента, включая mismatch controls | Ни update, ни новый Run не вызваны при работе existing Attempt; pinned input сохраняется |

Mandatory update — правило хоста. Прямой CLI start не получает скрытый updater.
Положительный binary update и signature/network failures проверяются signed HTTP fixtures internal/release.
Forward-test остановился на автоматическом approval-review до исполнения updater. Это не CLI Problem и не сетевой отказ продукта.
Основной агент отдельно выполнил update скопированного development binary: exit 2, code invalid_input, без нового Run.
Реальный public-release smoke выполняется после публикации по стандартным release gates, а не подменяется этим fixture.

## Независимый результат и измерения

Старый tracked Codex skill: 20 673 bytes. Проверенная агентом редакция skill: 10 007 bytes, index: 2 843 bytes.
Предыдущая кандидатная редакция successful comparison: skill+index 12 655 bytes против 20 673.
Оба successful Runs завершились completed/succeeded и записали ровно `default-123\n` (12 bytes).
Каждый использовал 7 public CLI calls; pinned context — 4 376 bytes.
Текущий skill после исправления полного имени команды project runners: 10,031 bytes; с index: 12,874 bytes.
Новый успешный Run для этих узких текстовых правок не запускался.

Первый mismatch fixture требовал отсутствующий input.title. Оба hosts честно вернули no_work → partial.
В этом контроле агент прочитал hosting целиком: 23 067 bytes против 20 673, то есть чтение выросло.
После этого runner явно направляет ordinary issued Attempt в свой core loop и предлагает искать только нужный heading/field.
Сравнение не было слепым: агент видел начальные start responses и сохранял знания предыдущего прохода.
Измерения отражают bytes/calls; actual model tokens, provider cost и гарантированный процент экономии неизвестны.

Forward-test обнаружил отсутствие technical_retries в обещанном workflow reference.
Исправление добавляет пример у stages.start, minimum revision, budget и retry_class constraints.
Агент повторно проверил маршрут в финальной установленной копии без Core sources.
Второй проверенный fragment — top-level workspace_trees, а не ArtifactRef внутри result.outputs.

## Ворота и сохранность

Контекст полных локальных ворот: codex-app-escalated. Метка описывает фактические shell permissions, не даёт полномочий.
Команды: make test-changed; make ci-check-once CHECK_CONTEXT=codex-app-escalated CHECK_FOR_COMMIT=1; openspec validate --all --strict --no-interactive; git diff --check.
Действительный exit и logs хранятся в локальном CI evidence; успешный результат требуется перед commit.
Review, security и verify оформляются отдельными принятыми gate artifacts текущего Run.

789 historical files сверены с baseline SHA-256: изменений нет.
TestProjectFrozenRunnerTextIsPinned проверяет все прежние templates, включая прежний current.
Новый current pin меняется отдельно, recognition chain только дополняется.
PROJECT.md и author-owned files сохраняются; symlink/conflict/concurrency/failure сценарии проверены.
Проверка документации не закрывает F2, roadmap или production qualification.

## Размер поставки

Baseline Linux binary: 40 491 112 bytes, build -trimpath -buildvcs=false.
Окончательный Linux binary: 41,179,600 bytes. Один installed bundle содержит 89 reference assets, всего 605,404 bytes.
Это не compressed release size.
Raw programs в examples остаются инертными assets. Установка справочника их не исполняет.
