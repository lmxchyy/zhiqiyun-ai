# PR81 acceptance matrix

Authoritative traceability for provider single-submit and recovery scope. CI runs the named tests uncached in `.github/workflows/user-core.yml`.

| ID | Acceptance evidence |
|---|---|
| F001 | `providerexecution.TestTEST_A_ProviderSucceededLocalVideoCompletionFailurePreservesRecovery` |
| F002 | `providerexecution.TestTEST_B_DurableSuccessRecoveryAfterLocalCrashZeroAdditionalProviderSubmit` |
| F003 | `providerexecution.TestTEST_C_UnknownCancelNoReleaseOrResubmit` |
| F004 | `providerexecution.TestTEST_D_SubmittedProcessingStaleRepairNoFailOrRelease` |
| F005 | `providerexecution.TestTEST_E_ClaimPreparedVsStaleFailureRace` |
| F006 | `storage.TestArtifactConcurrencyClaimIdentityPostgres` (migration 115, four-way race, three replays) |
| F007 | `providerexecution.TestTEST_F_SucceededStaleRepairLocalOnlyRecoveryNoProviderCall` |
| F008 | `providerexecution.TestTEST_H_DuplicateLocalRecoveryCaptureAtMostOnce` |
| F009 | `messaging.TestRabbitMQGenerationManualAckRedelivery` |
| F010 | `messaging.TestRabbitMQRuntimeRecoveryRetryDLQAndChannelRecreation` |
| F011 | `providerexecution.TestCrashMatrixCSubprocessRecovery` |
| F012 | `storage.TestArtifactConcurrencyClaimIdentity` |

A test is not considered passing when its required CI service or DSN is absent; local runs may skip integration tests only outside CI.
