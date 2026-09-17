#!/usr/bin/env bash
# The single owner of how the CI race suite is split across shards (GDK-1035;
# dealt by measured seconds since GDK-1913).
#
# .github/workflows/ci.yml never spells out a bucket itself — it asks this
# script which tests belong to shard N of T. Test names are discovered from
# the package source at run time (never a checked-in list: a checked-in
# list is the thing that goes stale and silently drops a test), and dealt
# longest-processing-time-first onto the lightest shard, by the measured
# seconds table embedded below — the same design tools/e2e-partition.sh
# proved on the browser suite.
#
# Why a race tier exists at all (GDK-270): a startSyncJob goroutine that
# outlives its test only shows up when the package is repeated under the
# race detector. That is why the tier runs -count=2 — and why splitting it
# must not quietly drop, skip, or double a test. The split is a
# redistribution across runners, never a relaxation.
#
# Why seconds, not counts (GDK-1913): the first draft dealt round-robin by
# count — 142/142/142 tests landing as 387/344/421 s of test time (CI,
# count=2), and with the fixed workspace step (shard 1 only) the decisive
# shard stood at 580 s. The longest shard is the job's wall clock, so the
# deal balances seconds: heaviest test first onto the lightest shard, with
# shard 1 preloaded by the workspace step's cost because that step is
# pinned to it.
#
# The table holds count=1 seconds (-count=1 -race -json, one quiet machine);
# the tier runs -count=2, so the deal projects ×2 — the ratios survive the
# doubling, and the measurement takes half the time. CI absolute seconds
# differ from the measuring machine's; the balance, which is all LPT needs,
# rides on the ratios. A test missing from the table is dealt the table's
# median weight: a new test starts average-heavy until the next refresh,
# never silently free and never skipped.
#
# Usage:
#   tools/race-partition.sh <shard> <total>   print the -run regex for one shard
#   tools/race-partition.sh --check <total>   verify the partition: every test
#                                             in exactly one shard, no empty
#                                             shard, no stale weight row,
#                                             discovery == go test -list
#   tools/race-partition.sh --list <total>    shard / count / seconds map
#   tools/race-partition.sh --measure         re-measure the table on a QUIET
#                                             machine and print fresh rows
#                                             (see the table header for
#                                             splicing; not a CI verb)
#
# Exit: 0 ok, 1 partition broken (--check) or measurement failed (--measure),
#       2 usage error.
set -euo pipefail
export LC_ALL=C # byte-order sort, so every machine deals the same round

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$ROOT/internal/server"
SERVER_PKG="./internal/server/"
WORKSPACE_PKG="./internal/workspace/"
SELF="race-partition"

# The tier runs the server package -count=2 (GDK-270) plus, on shard 1 only,
# the workspace package -count=2 (its own step in ci.yml). The table holds
# count=1 seconds; this factor projects it onto the clock being balanced.
PROJECTION=2
# go test ./internal/workspace/ -count=2 -race, package pass, 2026-09-15,
# same machine and same run as the table (see header below). CI measured
# 133 s for the same step — runners are slower; the preload stays in the
# measuring machine's units so it is coherent with the table.
WORKSPACE_PRELOAD=62.806

# ── the measured table ──────────────────────────────────────────────────────
# Splice region: everything between RACE_WEIGHTS_EOF markers, plus the
# WORKSPACE_PRELOAD constant above, is what `--measure` regenerates.
#
# Provenance (2026-09-15, darwin/arm64, go1.26.4, this worktree):
#   go test ./internal/server/ -count=1 -race -json
# -count=1 although CI runs -count=2: the doubling preserves the ratios the
# deal rides on and halves the measuring time. Top-level tests only (the
# deal's unit is the -run name); a parent pass event carries the whole
# test, subtests included.
#
# Witnesses (the quiet-machine rule: numbers from a busy machine are void):
#   before  loadavg 10.90 17.43 18.06, CPU 52.4% idle
#   after   loadavg  6.16 11.08 14.75, CPU 54.6% idle
#   foreign load: one GUI renderer at ~100% of one core (Orca Helper), no
#   other test/bench process. The machine is a desktop, not a bench rig —
# treat ±10% on any single row as noise; the deal needs the spread, not the
# digits.
WEIGHTS_TEXT="$(cat <<'RACE_WEIGHTS_EOF'
0.930	TestAbsorbRecentsDoesNotDuplicate
0.940	TestArtifactRouteRefusesNonHTMLAndForeignKeys
0.920	TestArtifactRouteServesHTMLUnderSandboxCSP
1.020	TestArtifactStreamsSamePolicyWhenTooLargeForCache
0.960	TestArtifactStreamsSamePolicyWithoutCache
0.970	TestAssigneeSetAndClear
1.000	TestAttachmentCacheMissesAfterSiteSwitch
0.950	TestAttachmentCacheRejectsForeignIssueKey
0.970	TestAttachmentEmptyExternalIDFallsBackToStoreID
1.080	TestAttachmentIsFetchedOnceThenServedFromDisk
0.000	TestAttachmentMissReasonDistinguishesScopeMismatch
0.910	TestAttachmentProxyNeedsCredential
0.980	TestAttachmentProxyStreamsFromJira
0.990	TestAttachmentStoreIdDoesNotBypassExternalID
0.990	TestAttachmentUnknownIdIsNotFound
0.900	TestAvailableProjectsListsTheSite
0.900	TestAvailableProjectsNeedsACredential
1.340	TestBenchSmoke
0.980	TestBindOriginHandlerAfterLazyUseTakesEffect
0.970	TestBindOriginHandlerNilUnbindsBackToLazy
0.900	TestBindOriginHandlerReplacesTheTarget
0.950	TestBoardsEndpoint
0.940	TestBootstrapBodyOmitsLastSessionEnd
0.970	TestBootstrapBoundaryHeaderAbsentWithoutVisits
0.930	TestBootstrapBoundaryHeaderMatchesBody
1.000	TestBootstrapBoundaryHeaderOnHEAD
0.970	TestBootstrapBoundaryHeaderSurvives304
0.930	TestBootstrapCarriesLastSessionEnd
0.920	TestBootstrapLastSessionEndAbsentWithoutVisits
0.960	TestBootstrapShapeAndETag
0.960	TestBootstrapSurvivesABrokenGroupQuery
0.960	TestBootstrapSurvivesLocalReadError
0.980	TestBotActorSurfaces
1.040	TestBotWebPayloads
0.980	TestBrowserGuardAllowedHosts
0.950	TestBrowserGuardForbiddenHost
0.940	TestBrowserGuardForbiddenOrigin
0.930	TestBrowserGuardMatchingOriginAllowed
0.920	TestBrowserGuardNoOriginAllowed
1.030	TestBrowserGuardNullOriginForbidden
0.940	TestBrowserGuardPlainGetForeignOriginStillAllowed
0.910	TestBrowserGuardTauriOriginWithoutPairingForbidden
0.940	TestBrowserGuardWebSocketForeignOriginForbidden
0.930	TestBrowserGuardWebSocketMatchingOriginAllowed
1.030	TestBuiltInAttachmentIsServedNotA502
2.060	TestBuiltInAttachmentKeepsTheOriginsMime
1.050	TestBuiltInAttachmentRevalidatesWithoutA502
1.090	TestBuiltInAttachmentSeeksWithRange
1.180	TestBuiltInInitIsIdempotentOnBuiltInWorkspace
0.900	TestBuiltInInitRefusesConnectedWorkspace
1.050	TestBuiltInInitSeedsWorkspaceAndServesItImmediately
0.860	TestBuiltInOriginPersistFailureIsNotCredentialRequired
0.970	TestBuiltInOriginSecondSessionWrites
1.080	TestBuiltInUploadHTMLAttachmentMirrorsTextHTML
1.020	TestBurnupEndpoint
0.000	TestCachedAttachmentETagCarriesNoSiteOrWorkspace
0.950	TestCachedAttachmentSurvivesCredentialRemoval
0.890	TestCachedSvgIsForcedToDownload
0.950	TestCommentAndCreateRefuseStrayPlaceholders
0.970	TestCommentPassesVisibilityAndInternal
1.000	TestCommentResponseEchoesRestriction
0.990	TestCommentSendsMentionsAsADF
0.970	TestComponentsBuiltinEditMetaAndWrite
0.930	TestConnectDoesNotFireWhenCredentialAlreadyPresent
0.940	TestConnectEmptyBuiltInClearsKind
0.890	TestConnectFiresSyncStarterOnce
2.810	TestConnectNamesTheTokenTrapItCanRecognise
1.930	TestConnectRefusesBuiltInWithLocalData
0.930	TestConnectRefusesDifferentSite
0.990	TestConnectRejectsInvalidExpiry
1.850	TestConnectReplaceBuiltInClearsKind
1.040	TestConnectReplaceBuiltInDropsWatchesFavoritesAndLOC
0.940	TestConnectReportsARejectedCredential
0.930	TestConnectRequiresSiteAndCredential
0.900	TestConnectStoresUserExpiry
0.930	TestConnectSyncStarterUnregisteredIsNoop
0.950	TestConnectVerifiesStoresSiteAndHidesTheToken
0.950	TestContentRouteStillDownloadsHTML
0.950	TestCreateFieldsCarriesKindAndOptions
0.890	TestCreateFieldsMissingQuery
0.930	TestCreateFieldsNoCredential
0.950	TestCreateFieldsOriginErrorDoesNot500
1.000	TestCreateFieldsReturnsRequiredSet
0.990	TestCreateIssue
0.990	TestCreateIssueCustomFieldsEmptyIsOmitted
0.960	TestCreateIssueCustomFieldsRefusesUnknownAndFixed
0.960	TestCreateIssueCustomFieldsWrappedByKind
0.970	TestCreateIssueDefaultProject
0.960	TestCreateIssueEmptyOptionalFieldsOmittedFromPayload
0.980	TestCreateIssueEmptyProjectsAllows
0.890	TestCreateIssueEmptySummaryStillRequired
0.940	TestCreateIssueIgnoresPriorityNameField
0.950	TestCreateIssueNormalizesParentKey
0.930	TestCreateIssueOmitsEmptyDuedate
1.050	TestCreateIssueOmitsEmptyParent
0.890	TestCreateIssueOmitsProjectFailsWhenAmbiguous
0.890	TestCreateIssueOmitsTypeFailsWhenMany
0.980	TestCreateIssueOmitsTypeUsesConfigDefault
0.990	TestCreateIssueOmitsTypeUsesSole
0.890	TestCreateIssueRejectsInvalidDuedateBeforeJira
0.900	TestCreateIssueRejectsInvalidParentBeforeJira
0.970	TestCreateIssueReportsOrigin
0.930	TestCreateIssueSendsDuedate
0.960	TestCreateIssueSendsParent
0.940	TestCreateIssueSendsPriorityByID
0.950	TestCreateIssueStaleDefaultType
0.920	TestCreateMetaForwardsSubtaskAndHierarchyLevel
0.920	TestCreatePairedUnreachableIsNotProjectRequired
0.940	TestCreateRESTErrorBodiesHaveNoCLIFlagTokens
1.000	TestCreateRoutesLinearTeamInMixedWorkspace
1.090	TestCredentialAdvertisesLinear
0.950	TestCredentialLifecycle
0.890	TestDashboardAbsorb
0.920	TestDashboardDataErrors
0.980	TestDashboardDataJQL
0.960	TestDashboardDataSQL
0.950	TestDashboardDataWriteSQLRefused
0.890	TestDashboardGetRow
0.900	TestDashboardLibRoute
0.890	TestDashboardRenderCSP
0.900	TestDashboardRenderCorruptConfig
1.000	TestDashboardRenderLibs
0.920	TestDashboardSaveLibs
0.910	TestDashboardSaveListDelete
0.970	TestDashboardSaveValidation
0.920	TestDashboardVendorRoute
0.950	TestDeferredEndpointsAre404
0.980	TestDeltaCarriesBoundaryHeader
0.940	TestDeltaOmitsLastSessionEnd
1.040	TestDeltaUpsertedAndDeleted
0.940	TestDerivedReleasesLockDuringRebuild
1.080	TestDescriptionPlaceholdersRoundTrip
1.020	TestDescriptionSetAndClear
0.930	TestDetailAssembly
1.050	TestDetailAttachmentsEmptyIsArray
1.030	TestDetailDerivesLinkedPRsFromAttachments
1.020	TestDetailDerivesLinkedPRsFromRemoteLinks
0.930	TestDetailFlagsHTMLAttachmentsAsArtifacts
0.990	TestDetailHistoryReopenVerdict
0.990	TestDetailHistoryReopenWireSpelling
1.000	TestDetailLinkPhraseFromBackend
1.040	TestDetailPresentsBodiesAsMarkdown
1.090	TestDetailSurvivesLocalReadError
1.110	TestDetailVisitsAbsentWithoutVisits
1.090	TestDetailVisitsCarryPersonReadTimestamps
0.900	TestEditDuedateRejectsInvalidBeforeJira
0.990	TestEditDuedateSetAndClear
1.060	TestEditMetaFromFieldSpecs
1.880	TestEditMetaOnlyExposesAllowlistedFields
0.900	TestEditParentRejectsInvalidBeforeJira
0.900	TestEditParentRejectsSelfBeforeJira
1.080	TestEditParentSetAndClear
0.920	TestEnrichmentCannotShadowMirroredFields
1.110	TestEnrichmentsMerge
0.000	TestFailJiraMapsOriginHTTP
0.000	TestFailJiraMapsUnsupportedAndRefused
0.900	TestFavoritesRoundtrip
0.000	TestFeedDefaultFeatureFlag
1.000	TestFeedFocusAssignee
1.110	TestFeedGetAndMarkRead
0.990	TestFeedReopenedEvent
0.000	TestFetchStoredURLRefusesUploadsSubdomainRedirect
0.000	TestFetchStoredURLStripsAuthorizationOnCrossHostRedirect
1.050	TestFieldEditAllowlistAndShapes
1.040	TestFieldEditScalarKinds
1.020	TestFirstSyncIsSingleFlightAndReportsProgress
1.050	TestFlowMemoErrorNotCached
0.970	TestFlowMemoHoldsAcrossBootstrapAndDelta
0.940	TestFlowMemoInvalidatesOnSyncVersion
0.940	TestFlowMemoSurvivesThresholdSet
0.940	TestFrozenWorkspaceRefusesResync
0.940	TestFrozenWorkspaceRefusesWrites
0.080	TestGateLogsNeverFormatTheBearer
0.000	TestGroupFallbackUsesAssigneeAccountID
0.000	TestGuardBrowserWrapsNext
0.960	TestHandlerShutdownCancelsSyncJob
1.920	TestHealthExposesConfluenceWhenSourcePresent
0.990	TestHistoryCursorLimit
0.990	TestHistoryDeleteClearsVisitsAndSearches
0.940	TestHistoryInvalidCursor
1.000	TestHistoryPatchMissingSearch
0.930	TestHistoryRejectsBadKind
0.960	TestHistorySearchAndOpened
0.990	TestHistoryVisitAppendAndList
1.000	TestHistoryVisitedKeysFoldsPerKey
0.910	TestHydrateRefs
0.930	TestIdentityHeadersEmptyProfile
0.950	TestIdentityHeadersOnAPIResponses
0.950	TestIdentityHeadersOnForbiddenHost
0.950	TestImportManifestEmptySiteKeepsLegacyKey
0.940	TestImportManifestRejectsForeignIssueKey
0.970	TestImportManifestServesFromCacheWithoutUpstream
0.000	TestImportManifestSkipsIDMissingFromMirror
0.000	TestIsLinearUploadsURL
0.940	TestIssueLiteFieldNames
0.920	TestIssueLiteHierarchyLevelOnBootstrap
0.890	TestIssueResyncNotFound
0.930	TestIssueResyncRefreshesMirror
0.920	TestJiraAttachmentStillUsesSiteBasicAuth
0.950	TestJiraErrorsPassThrough
0.940	TestJqlCurrentUserResolvesToAccountID
0.000	TestJqlDoesNotFullScanIssueLites
0.950	TestJqlParseAndEmit
0.950	TestJqlReporterNameResolvesToAccountID
1.000	TestKeyPrioritiesRoutesBySource
0.970	TestKeyUsersMatchesGlobalOnJiraRow
1.020	TestLabelsSetAndClear
0.940	TestLinearAttachmentFetchesStoredURLWithoutAuth
0.940	TestLinearAttachmentOtherHostIsNotFetched
0.970	TestLinearAttachmentPassesThrough401
1.010	TestLinearKeyPrioritiesWithoutJiraCredential
0.950	TestLinearOnlyCreateRoutesToLinear
1.010	TestLinearOnlyFieldEditRoutesToLinear
0.970	TestLinearUploadsAttachmentSendsBareAPIKey
1.000	TestLinkRESTInwardDescriptionMakesPathIssueDisplayInwardDescription
0.980	TestLinkRESTOriginFailureLeavesMirrorUnchanged
1.020	TestLinkRESTOutwardDescriptionMakesPathIssueDisplayOutwardDescription
0.920	TestLinkRESTRequiresCredential
0.980	TestLinkRESTSelfRefusedNoOrigin
0.970	TestLinkRESTUnknownTypeDoesNotPOST
0.980	TestLinkTypesREST
0.990	TestMe
0.960	TestMembersIncludeAccountIDOnlyRosterRows
1.110	TestMirrorAllowlistTable
1.000	TestMirrorGateCommentWriteGoesThroughOrigin
0.920	TestMirrorGateCredentialHintedNoToken
0.870	TestMirrorGateDemandsBearer
0.980	TestMirrorGateLoopbackUnchanged
0.960	TestMirrorGateScopeBoundaries
0.900	TestMirrorGateServeBearerOpensBootstrap
0.870	TestMirrorGateUnpairedDNSHostStaysForbidden
0.000	TestNormalizeSiteAcceptsWhatPeoplePaste
0.000	TestOriginClientLinearOnlyNeedsAtlassian
0.910	TestOriginExportConnectedIsRefused
0.960	TestOriginExportServesSeedYAML
0.940	TestOriginRESTBuiltInPOSTWithoutOriginAllowed
0.950	TestOriginRESTBuiltInPassesThrough
0.930	TestOriginRESTConnectedIs404
0.920	TestOriginRESTPreservesActorHeaderWithoutGate
0.000	TestOsNotifySupportedJSONIncludesFalse
1.220	TestOverlappingSyncKicksRunOnce
1.010	TestPageArtifactRouteServesFromCacheAndRefusesNonHTML
0.990	TestPageAttachmentWireMatchesIssueDetail
1.100	TestPageCommentMissingIsNotFound
0.950	TestPageCreateInvalidADF
1.300	TestPageCreateUpdatesMirrorAfterOrigin
0.960	TestPageDetail200And404
1.150	TestPageEditExplicitVersionConflict
0.930	TestPageEditInvalidADF
1.400	TestPageEditTextFormatLoss
0.910	TestPageResyncNotFound
1.050	TestPageResyncRefreshesMirror
0.980	TestPagesListAndETag
0.970	TestPagesResponseIncludesAuthorID
1.000	TestPagesResponseIncludesLabels
0.960	TestPagesResponseIncludesSpaceHomepageID
0.960	TestPagesResponseIncludesSpaceName
0.900	TestPairedCredentialSurfaceIsMeasured
1.010	TestPairedHostExemptLetsTailnetNameThrough
0.900	TestPairedWrite401IsPairingErrorNotCredentialRejected
0.920	TestPairingGate401CarriesRejectReason
0.990	TestPairingGateAcceptsValidBearer
0.970	TestPairingGateExpiredAndRevokedRejected
0.920	TestPairingGateKeepsActorHeaderWhileRewritingAuth
0.990	TestPairingGateOffWithoutTokens
0.940	TestPairingGateRejectsWithoutValidBearer
1.010	TestPairingTokenNeverReachesTheLog
1.020	TestParentBuiltinEditMetaAndWrite
0.940	TestPeopleCommentsEmptyAuthor
0.950	TestPeopleCommentsLimitAndInvalid
0.940	TestPeopleCommentsOK
0.900	TestPersonalStateRoundtrip
0.890	TestPostRecentRejectsEmpty
1.000	TestPreviewRendersMarkdownAsADF
0.000	TestPrimaryFocusTakesProcessProfileFile
0.970	TestPrioritySetAndClear
1.830	TestPutCredentialKeepsStoredTokenOnEmpty
0.970	TestPutCredentialStoresUserExpiry
0.930	TestPutSettingsAppearanceRejectsShape
0.940	TestPutSettingsAppearanceRoundtrip
0.980	TestPutSettingsConfluenceDisable
0.940	TestPutSettingsConfluenceEnableFromOff
0.970	TestPutSettingsConfluenceEnableReplacesSpaces
0.910	TestPutSettingsConfluenceSpacesNotConfigured
0.920	TestPutSettingsConfluenceSpacesOnlyWhileOff
0.950	TestPutSettingsConfluenceSpacesOnlyWhileOn
0.980	TestPutSettingsConfluenceSpacesRoundtrip
0.950	TestPutSettingsDoesNotClearFrozen
0.910	TestPutSettingsNonScopeNoKick
0.990	TestPutSettingsOmitsAppearancePreserves
0.890	TestPutSettingsOmitsConfluenceKeyLeavesSpaces
0.880	TestPutSettingsOmitsConfluenceKeyPreserves
0.950	TestPutSettingsScopeChangeKicksFullSync
0.960	TestPutSettingsScopeChangeNoCredentialNoKick
0.910	TestPutSettingsTerminalDisplayOnly
0.930	TestPutSettingsUIJudgmentWarnsAndSaves
0.000	TestRESTContractGoldensListed
0.920	TestRESTParentRejectionCreateCarriesHierarchyHint
0.910	TestRESTParentRejectionEditCarriesHierarchyHint
0.920	TestRESTParentRejectionUnrelated400HasNoHint
0.990	TestRESTResponseGoldens
0.960	TestRangedViewOfACacheableAttachmentFillsTheCache
0.950	TestRangedViewOfAnOversizeAttachmentStreamsPastTheCache
0.890	TestRecentsAPIParityWithSQL
0.890	TestRecentsRouteExists
0.890	TestRejectedCredentialIsNotStored
0.000	TestResolveLinkTypeMatchesCLI
1.040	TestRetroAmbiguousBoardCarriesTheBoards
1.020	TestRetroEndpoint
0.980	TestRetroEndpointCarriesTheMaterials
1.010	TestRetroEndpointSessionGapConfigDefault
0.000	TestRoutesRegister
1.230	TestRunSyncJobPullsLinear
1.030	TestRuntimeInfoReportsOriginAndAttachmentBytes
0.950	TestSearchHitsCommentText
0.950	TestSearchResponseIncludesPages
0.960	TestServeAnnouncesVersionHeader
0.000	TestServeScopeIsDefaultClosed
0.000	TestServedConfigCarriesBothAxes
0.000	TestSettingsCatalogCoversPUTFields
1.010	TestSettingsFieldSpecsAndUsageReadOnly
0.900	TestSettingsIntervalFloorsReject
1.010	TestSettingsOsNotifySupportedFollowsInjectedNotifier
0.930	TestSettingsRoundtripPreservesCredential
1.010	TestSettingsRuntimeCountsMatchIssueLites
0.920	TestSettingsRuntimeProfileName
0.930	TestSettingsRuntimeReadOnlyNoSecrets
0.890	TestSettingsSpacesCarriesPageCount
0.960	TestSettingsSpacesCountingIsBounded
0.930	TestSettingsSpacesEmptyMeansAllGlobalFlag
0.960	TestSettingsSpacesListsSelectedAndSorts
0.960	TestSettingsSpacesListsWhenCountingFails
0.950	TestSettingsSpacesOffListsWithEnabledFalse
0.960	TestSettingsSpacesOffNoCredential
0.940	TestSettingsSpacesOmitsUncountableSpace
0.000	TestSettingsSpacesOriginSentinelIsNotCredentialRequired
0.900	TestSettingsSyncIntervalsRoundtrip
0.980	TestSettingsUIRoundtripAndRefusal
0.950	TestSnapshotSyncMatchesProgressEndpoint
1.020	TestStartSyncAcceptsIncrementalMode
0.950	TestStartSyncFrozenIsNotInProgress
1.560	TestStartSyncJobNoopsWhenWatchActivityRunning
0.930	TestStartSyncRefusesAnIncompleteSetup
0.930	TestSummarySet
1.000	TestSyncActivityDoesNotImplyJobRunning
0.930	TestSyncActivityHooksOnProgress
0.940	TestSyncActivityPhaseIdleCloses
0.950	TestSyncActivitySourceChangeResetsCounters
0.940	TestSyncHealth
0.930	TestSyncHealthTokenExpiry
0.970	TestSyncProgressCarriesFirstSync
0.990	TestSyncProgressClassifiesARejectedCredential
1.010	TestSyncProgressFirstSyncDocumentsPhase
0.960	TestSyncProgressLeavesATransportFailureUnclassified
0.890	TestSyncProgressStartsIdle
0.970	TestSyncRunsLastCheckedAt
0.930	TestSyncRunsSourceFilter
0.900	TestTerminalAllowRemoteAddressNeedsAToken
0.930	TestTerminalCreateBindsIssueAndName
0.870	TestTerminalCreateResponseBehaviorDefaults
0.880	TestTerminalCreateResponseCarriesBehavior
0.930	TestTerminalCreateUsesConfiguredShellAndDir
2.270	TestTerminalCreateWorkingDirMissingFallsBack
0.870	TestTerminalDNSHostDemandsBearer
0.900	TestTerminalGateCoversTheWholeSubtree
1.840	TestTerminalInputBoundsAndDoors
2.050	TestTerminalInputPlacesTextWithoutRunningIt
1.860	TestTerminalIssueBinding
0.880	TestTerminalListIsMetadataOnly
0.000	TestTerminalLocalHostIsLoopbackOnly
0.000	TestTerminalLocalNeedsLoopbackPeer
2.550	TestTerminalLoopbackOpensAndEchoes
0.000	TestTerminalPathIsNotMirrorREST
0.890	TestTerminalRename
0.970	TestTerminalRevokeCutsLiveSession
0.860	TestTerminalScopeIsAOneWayDoor
4.210	TestTerminalSessionNamesItsWorkspace
0.940	TestTerminalSessionsAreScopedToTheirToken
0.930	TestTerminalShutdownReapsSessions
2.340	TestTerminalSocketGoroutinesStopOnShutdown
0.000	TestTerminalSurfaceNeverWritesToOrigin
0.000	TestTerminalTokenLogIsHashedNotPlaintext
0.890	TestTerminalTokenOpensShellOverDNSHost
0.880	TestTerminalWebviewOriginCannotOpenTheSocket
0.970	TestTokenNeverReachesResponsesOrLogs
0.950	TestTransitionRESTFieldAliasRemap
0.990	TestTransitionRESTForwardsFieldsAndComment
4.650	TestTransitionRESTIsTheMachineIDContract
0.920	TestTransitionRESTPickedIDDoesNotFoldWhenAlreadyThere
0.930	TestTransitionRESTPickedIDIsExactOnFoldedNames
0.940	TestTransitionRESTRequiredResolutionAcceptsFieldsID
0.920	TestTransitionRESTRequiredResolutionRefusesWithoutValue
1.000	TestTransitionRESTResolutionNameUsesAllowedValues
0.940	TestTransitionRESTWithoutExtrasOmitsFieldsAndUpdate
1.010	TestTransitionWritesThroughToTheMirror
0.890	TestTransitionsAndUsersAndCreateMeta
0.930	TestTransitionsGETIncludesTargetStatusID
0.900	TestTransitionsGETRequiredFieldsOnly
0.990	TestUIFocusAlwaysCarriesConfigVersion
0.950	TestUIFocusCarriesMirrorVersionThatMovesWithTheMirror
0.000	TestUIFocusLogsOncePerProfileNotPerPoll
0.000	TestUIFocusPeekReturnsHashAndAtTwice
0.000	TestUIFocusReadDoesNotConsume
0.000	TestUIFocusWithoutAMirrorStillServesFocus
0.950	TestUncacheableAttachmentIsFetchedExactlyOnce
0.970	TestUncachedAttachmentWithoutCredentialAsksForOne
0.900	TestUploadMirrorRereadFailureIs502
0.930	TestUploadProxiesAndReturnsContentURL
1.030	TestViewsIncludeSourceQueries
0.980	TestWebCommentCarriesNoActorTrailer
0.000	TestWebConfigCapabilitiesBlock
0.920	TestWebConfigCarriesUI
1.010	TestWebConfigHidesCredential
0.000	TestWebConfigOriginWritableMirrorsHasAtlassianCredential
0.000	TestWebConfigProfileName
0.000	TestWebConfigUIDimensionVars
0.000	TestWebConfigUIFonts
0.000	TestWebConfigWorkspaceKindFromDescribe
0.950	TestWikiServerDetailCarriesVerbatim
0.920	TestWikiServerPreviewIsCodeBlock
1.010	TestWikiServerWritesSendVerbatimString
1.020	TestWikiWritesRequireCredential
0.000	TestWorkspaceFocusTakesProfileFile
3.690	TestWriteAppliedMirrorStaleStatusUnchanged
0.020	TestWriteHandlersDoNotCallClient
0.960	TestWritesRequireACredential
RACE_WEIGHTS_EOF
)"

usage() {
  cat >&2 <<'EOF'
usage:
  tools/race-partition.sh <shard> <total>   print the -run regex for one shard
  tools/race-partition.sh --check <total>   verify the partition (exit 1 if broken)
  tools/race-partition.sh --list <total>    shard / count / seconds map
  tools/race-partition.sh --measure         re-measure the table (quiet machine)
EOF
  exit 2
}

# Every top-level test function in the package, one per line, sorted, with
# the Test prefix kept — go test matches -run against full test names, and
# the go-test -list comparison below holds discovery to exactly that.
# `sort -u` is belt-and-braces: Go does not compile a package that declares
# the same test function name twice.
test_names() {
  sed -n -E 's/^func (Test[A-Za-z0-9_]+)\(.*/\1/p' "$SERVER_DIR"/*_test.go |
    sort -u
}

# "weight<TAB>name" for every discovered test, median fallback for a name
# the table does not carry yet. The table rides in WEIGHTS_TEXT (embedded
# on purpose: one file owns the deal, the weights, and their provenance —
# and a stray tsv beside it cannot drift from the script that reads it),
# and reaches awk through the environment: `awk -v` rejects a value with
# embedded newlines on BSD awk (macOS /bin/bash's awk is the floor here).
weighted_tests() {
  test_names | WEIGHTS_TEXT="$WEIGHTS_TEXT" awk '
    BEGIN {
      n = split(ENVIRON["WEIGHTS_TEXT"], lines, "\n")
      m = 0
      for (i = 1; i <= n; i++) {
        if (lines[i] ~ /^#/ || lines[i] ~ /^$/) continue
        split(lines[i], f, "\t")
        w[f[2]] = f[1] + 0
        m++
        v[m] = f[1] + 0
      }
      if (m > 0) {
        # insertion sort — m is a test count, and this stays awk-portable
        for (i = 2; i <= m; i++) {
          x = v[i]
          for (j = i - 1; j >= 1 && v[j] > x; j--) v[j + 1] = v[j]
          v[j + 1] = x
        }
        median = (m % 2) ? v[int((m + 1) / 2)] : (v[m / 2] + v[m / 2 + 1]) / 2
      } else {
        median = 0
      }
    }
    { printf "%.3f\t%s\n", (($0 in w) ? w[$0] : median), $0 }
  '
}

# LPT deal: heaviest first (weight ties break by name), each onto the
# lightest shard (load ties by the lower shard number), shard 1 preloaded
# with the workspace step that ci.yml pins to it. The assignment is a
# function of the table alone. stdout: "shard<TAB>name<TAB>projected s".
deal() { # $1 = total
  weighted_tests |
    sort -t"$(printf '\t')" -k1,1nr -k2,2 |
    awk -F'\t' -v t="$1" -v proj="$PROJECTION" -v preload="$WORKSPACE_PRELOAD" '
      BEGIN { load[1] = preload }
      {
        b = 1
        for (i = 2; i <= t; i++) if (load[i] < load[b]) b = i
        load[b] += $1 * proj
        printf "%d\t%s\t%.3f\n", b, $2, $1 * proj
      }'
}

# The names dealt to one shard.
shard_names() { # $1 = shard (1-based), $2 = total
  deal "$2" | awk -F'\t' -v s="$1" '$1 == s { print $2 }'
}

# The go test -run regex for one shard. The anchors are load-bearing:
# -run matches unanchored, so without ^( )$ the shard holding
# TestCreateIssue would also run TestCreateIssueDefaultProject (prefix pairs
# exist in this package), and one test would land in two shards.
# An empty shard prints ^()$, which matches no test name — --check is what
# turns that into a failure.
shard_regex() { # $1 = shard, $2 = total
  local body
  body="$(shard_names "$1" "$2" | paste -sd '|' -)"
  if [[ -n "$body" ]]; then
    printf '^(%s)$\n' "$body"
  else
    printf '^()$\n'
  fi
}

# ── --measure: regenerate the table on a quiet machine ──────────────────────
# Not a CI verb — a runner is never quiet, and numbers from a busy machine
# are void (the quiet-machine rule). Prints splice-ready rows plus the
# workspace preload, with loadavg witnesses around the runs.
measure() {
  command -v go >/dev/null 2>&1 || {
    echo "$SELF: go is not on PATH; --measure needs the toolchain" >&2
    return 1
  }
  local tmp la_before la_after
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/race-measure.XXXXXX")" || return 1
  la_before="$(uptime)"
  if ! (cd "$ROOT" && go test "$SERVER_PKG" -count=1 -race -json \
        > "$tmp/server.json" 2> "$tmp/server.err"); then
    echo "$SELF: server measurement failed:" >&2
    sed 's/^/  /' "$tmp/server.err" >&2
    rm -rf "$tmp"
    return 1
  fi
  if ! (cd "$ROOT" && go test "$WORKSPACE_PKG" -count=2 -race -json \
        > "$tmp/ws.json" 2> "$tmp/ws.err"); then
    echo "$SELF: workspace measurement failed:" >&2
    sed 's/^/  /' "$tmp/ws.err" >&2
    rm -rf "$tmp"
    return 1
  fi
  la_after="$(uptime)"
  python3 - "$tmp/server.json" "$tmp/ws.json" <<'MEASUREPY'
import json, sys

def pass_events(path):
    with open(path, encoding="utf-8") as fh:
        for raw in fh:
            raw = raw.strip()
            if not raw:
                continue
            try:
                ev = json.loads(raw)
            except json.JSONDecodeError:
                continue
            if ev.get("Action") == "pass":
                yield ev

rows = {}
for ev in pass_events(sys.argv[1]):
    t = ev.get("Test")
    el = ev.get("Elapsed")
    # top-level only: subtest names carry a '/', and the parent pass event
    # already carries the whole test, subtests included.
    if t and "/" not in t and isinstance(el, (int, float)):
        rows[t] = float(el)

preload = 0.0
for ev in pass_events(sys.argv[2]):
    if not ev.get("Test") and isinstance(ev.get("Elapsed"), (int, float)):
        preload = float(ev["Elapsed"])

for name in sorted(rows):
    print(f"{rows[name]:.3f}\t{name}")
print(f"# {len(rows)} top-level tests, count=1 -race sum {sum(rows.values()):.3f} s")
print(f"# workspace -count=2 -race package pass: {preload:.3f} s")
print(f"#   -> WORKSPACE_PRELOAD={preload:.3f}")
MEASUREPY
  rm -rf "$tmp"
  echo "# witnesses — before: $la_before"
  echo "# witnesses — after:  $la_after"
  echo "# splice the rows over the RACE_WEIGHTS_EOF region and update"
  echo "# WORKSPACE_PRELOAD; re-date the provenance header."
}

# ── argument parsing ────────────────────────────────────────────────────────
if [[ "${1:-}" = "--measure" ]]; then
  [[ $# -eq 1 ]] || usage
  measure
  exit $?
fi

[[ $# -eq 2 ]] || usage
mode=shard
case "$1" in
  --check) mode=check ;;
  --list) mode=list ;;
  -*) usage ;;
  *) shard="$1" ;;
esac
total="$2"
[[ "$total" =~ ^[1-9][0-9]*$ ]] || {
  echo "$SELF: total must be a positive integer, got '$total'" >&2
  exit 2
}
if [[ "$mode" = shard ]]; then
  [[ "$shard" =~ ^[1-9][0-9]*$ ]] || {
    echo "$SELF: shard must be a positive integer, got '$shard'" >&2
    exit 2
  }
  [[ "$shard" -le "$total" ]] || {
    echo "$SELF: shard $shard is out of range 1..$total" >&2
    exit 2
  }
fi

if [[ "$mode" = shard ]]; then
  re="$(shard_regex "$shard" "$total")"
  if [[ "$re" = '^()$' ]]; then
    echo "$SELF: warning: shard $shard of $total selects no test" >&2
  fi
  printf '%s\n' "$re" # stdout carries the regex and nothing else
  exit 0
fi

names="$(test_names)"
if [[ -z "$names" ]]; then
  echo "$SELF: no test functions found in $SERVER_DIR/*_test.go" >&2
  exit 1
fi

all=()
while IFS= read -r n; do all+=("$n"); done <<<"$names"

# (a) stale or malformed weight rows: a row naming a test that no longer
# exists is the table rotting in place — the deal stays valid while lying
# about what it balances. Non-numeric seconds make the LPT sort garbage.
bad_rows=()
while IFS="$(printf '\t')" read -r secs tname; do
  [[ -z "${tname:-}" ]] && continue
  if ! printf '%s' "$secs" | grep -Eq '^[0-9]+([.][0-9]+)?$'; then
    bad_rows+=("$tname (seconds '$secs' is not a number)")
  elif ! printf '%s\n' "$names" | grep -qx "$tname"; then
    bad_rows+=("$tname (no such test in the package)")
  fi
done < <(printf '%s\n' "$WEIGHTS_TEXT" | grep -v -E '^(#|$)')

# The deal under test is built by the same generator the workflow calls;
# the assertions below interrogate its output, not the LPT arithmetic that
# produced it, so a generator bug cannot pass its own check.
assignment="$(deal "$total")"
regexes=()
shard_hit_count=()
counts=()
seconds=()
s=1
while [[ "$s" -le "$total" ]]; do
  regexes[s]="$(shard_regex "$s" "$total")"
  shard_hit_count[s]=0
  counts[s]="$(printf '%s\n' "$assignment" | awk -F'\t' -v s="$s" '$1 == s { n++ } END { print n + 0 }')"
  seconds[s]="$(printf '%s\n' "$assignment" | awk -F'\t' -v s="$s" '$1 == s { sum += $3 } END { printf "%.1f", sum + 0 }')"
  s=$((s + 1))
done
# Shard 1's clock also carries the workspace step pinned to it.
seconds[1]="$(awk -v a="${seconds[1]}" -v p="$WORKSPACE_PRELOAD" 'BEGIN { printf "%.1f", a + p }')"

uncovered=()
doubled=()
covered=0
for name in "${all[@]}"; do
  hits=()
  s=1
  while [[ "$s" -le "$total" ]]; do
    if [[ "$name" =~ ${regexes[s]} ]]; then
      hits+=("$s")
    fi
    s=$((s + 1))
  done
  case "${#hits[@]}" in
    0) uncovered+=("$name") ;;
    1)
      covered=$((covered + 1))
      shard_hit_count[${hits[0]}]=$((shard_hit_count[${hits[0]}] + 1))
      ;;
    *)
      hit_list="$(IFS=','; printf '%s' "${hits[*]}")"
      doubled+=("$name (shards $hit_list)")
      ;;
  esac
done

empty=()
s=1
while [[ "$s" -le "$total" ]]; do
  if [[ "${shard_hit_count[s]}" -eq 0 ]]; then
    empty+=("$s")
  fi
  s=$((s + 1))
done

# (e) first: discovery must agree with what the Go toolchain actually runs.
# `go test -list` is generated by the compiler from the same source, so it
# catches drift between the grep pattern and Go's own notion of a test name.
# The first draft of this script captured names without the Test prefix —
# internally consistent, (a)–(d) green, while every regex would have
# selected zero tests. Only the toolchain comparison sees that class.
if [[ "$mode" = check ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "$SELF: go is not on PATH; --check needs 'go test -list' to verify discovery" >&2
    exit 1
  fi
  if ! go_raw="$(cd "$ROOT" && go test "$SERVER_PKG" -list '.*' 2>&1)"; then
    echo "$SELF: go test -list failed, cannot verify discovery against the toolchain:" >&2
    printf '%s\n' "$go_raw" | sed 's/^/  /' >&2
    exit 1
  fi
  go_names="$(printf '%s\n' "$go_raw" | { grep -E '^Test[A-Za-z0-9_]+$' || true; } | sort -u)"

  only_grep=() # discovered from source, but the toolchain would not run it
  only_go=()   # runnable per the toolchain, but the discovery missed it
  while IFS= read -r line; do only_grep+=("$line"); done \
    < <(comm -23 <(printf '%s\n' "$names") <(printf '%s\n' "$go_names"))
  while IFS= read -r line; do only_go+=("$line"); done \
    < <(comm -13 <(printf '%s\n' "$names") <(printf '%s\n' "$go_names"))
fi

if [[ "$mode" = check ]]; then
  fail=0
  if [[ ${#bad_rows[@]} -gt 0 ]]; then
    echo "$SELF: weight rows that are stale or not numeric:" >&2
    printf '  %s\n' "${bad_rows[@]}" >&2
    fail=1
  fi
  if [[ ${#only_go[@]} -gt 0 ]]; then
    for n in "${only_go[@]}"; do
      echo "$SELF: $n is runnable per go test -list but not discovered from source — the partition would skip it" >&2
    done
    fail=1
  fi
  if [[ ${#only_grep[@]} -gt 0 ]]; then
    for n in "${only_grep[@]}"; do
      echo "$SELF: $n is discovered from source but not runnable per go test -list — no regex would ever select it" >&2
    done
    fail=1
  fi
  if [[ ${#uncovered[@]} -gt 0 ]]; then
    for n in "${uncovered[@]}"; do
      echo "$SELF: $n is in no shard (${#uncovered[@]} of ${#all[@]} uncovered)" >&2
    done
    fail=1
  fi
  if [[ ${#doubled[@]} -gt 0 ]]; then
    for d in "${doubled[@]}"; do
      echo "$SELF: $d — must be in exactly one shard" >&2
    done
    fail=1
  fi
  if [[ ${#empty[@]} -gt 0 ]]; then
    for s in "${empty[@]}"; do
      echo "$SELF: shard $s of $total selects no test" >&2
    done
    fail=1
  fi
  if [[ "$fail" -ne 0 ]]; then
    echo "$SELF: partition broken" >&2
    exit 1
  fi
  echo "$SELF: ${#all[@]} tests covered exactly once across $total shards (covered=$covered; discovery matches go test -list; ${#bad_rows[@]} stale weight row(s))"
  s=1
  while [[ "$s" -le "$total" ]]; do
    extra=""
    if [[ "$s" = 1 ]]; then extra=" (includes the ${WORKSPACE_PRELOAD} s workspace step)"; fi
    echo "$SELF: shard $s/$total: ${shard_hit_count[s]} tests, ${seconds[s]} s$extra"
    s=$((s + 1))
  done
  exit 0
fi

# --list: the balance, stated. Seconds are CI-projected (table × $PROJECTION,
# plus the workspace preload on shard 1); the spread, not the count, is the
# contract (GDK-1913: ≤ 30 s across the three shards). A spread over the
# contract is a stale table, not a broken partition — refresh with --measure
# rather than reddening CI on timing drift.
total_seconds="$(printf '%s\n' "$assignment" | awk -F'\t' -v p="$WORKSPACE_PRELOAD" '{ sum += $3 } END { printf "%.1f", sum + p }')"
median_fallback="$(comm -23 <(test_names) <(printf '%s\n' "$WEIGHTS_TEXT" | grep -v -E '^(#|$)' | cut -f2 | sort) | grep -c . || true)"
echo "$SELF: ${#all[@]} tests across $total shards, ${total_seconds} s CI-projected (table × $PROJECTION + ${WORKSPACE_PRELOAD} s workspace preload); $median_fallback test(s) on median fallback; stale weight rows: ${#bad_rows[@]}"
lo="${seconds[1]}"
hi="${seconds[1]}"
s=1
while [[ "$s" -le "$total" ]]; do
  extra=""
  if [[ "$s" = 1 ]]; then extra="  (includes the workspace step)"; fi
  first="$(printf '%s\n' "$(shard_names "$s" "$total")" | sed -n '1p')"
  last="$(printf '%s\n' "$(shard_names "$s" "$total")" | sed -n '$p')"
  printf 'shard %d of %d: %3d tests  %6.1f s  %s ... %s%s\n' \
    "$s" "$total" "${counts[s]}" "${seconds[s]}" "$first" "$last" "$extra"
  lo="$(awk -v a="$lo" -v b="${seconds[s]}" 'BEGIN { print (b + 0 < a + 0) ? b : a }')"
  hi="$(awk -v a="$hi" -v b="${seconds[s]}" 'BEGIN { print (b + 0 > a + 0) ? b : a }')"
  s=$((s + 1))
done
awk -v lo="$lo" -v hi="$hi" 'BEGIN { printf "spread: %.1f s (contract: <= 30 s)\n", hi - lo }'
