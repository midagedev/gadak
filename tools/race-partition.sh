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
# The workspace step's cost, expressed in the TABLE's units (GDK-2002).
# Measured 124 s on CI (run 36317659899, step "Workspace tests under race",
# the only step pinned to shard 1) and divided by the machine ratio the same
# run gives: CI server seconds 344+451+469 = 1264 over the table's projected
# 850.1, so CI runs this suite 1.487x slower than the measuring machine.
# 124 / 1.487 = 83.4.
#
# Why the ratio and not the raw local number: the preload reserves room on
# shard 1 for a step whose clock is CI's, so it has to be spent in the same
# currency the weights are spent in, or shard 1 under-reserves and carries
# tests it has no room for. The local pass of the same package measured
# 59.595 s on 2026-09-28 — that is the number this constant used to hold
# (62.806 from the 2026-09-15 pass), and holding it was the defect: it
# reserved 60 s of room for 124 s of work. Re-derive the ratio whenever the
# table is re-measured; the run id above is what makes it checkable.
WORKSPACE_PRELOAD=83.400

# ── the measured table ──────────────────────────────────────────────────────
# Splice region: everything between RACE_WEIGHTS_EOF markers, plus the
# WORKSPACE_PRELOAD constant above, is what `--measure` regenerates.
#
# Provenance (2026-09-28, darwin/arm64, go1.26.4, this worktree):
#   go test ./internal/server/ -count=1 -race -json
# -count=1 although CI runs -count=2: the doubling preserves the ratios the
# deal rides on and halves the measuring time. Top-level tests only (the
# deal's unit is the -run name); a parent pass event carries the whole
# test, subtests included.
#
# Witnesses (the quiet-machine rule: numbers from a busy machine are void):
#   before  loadavg 10.60  8.24  8.81
#   after   loadavg 11.21 15.07 12.55
#   foreign load: one GUI renderer at ~100% of one core (Orca Helper), no
#   other test/bench process. The machine is a desktop, not a bench rig —
# treat ±10% on any single row as noise; the deal needs the spread, not the
# digits.
WEIGHTS_TEXT="$(cat <<'RACE_WEIGHTS_EOF'
0.930	TestAbsorbRecentsDoesNotDuplicate
0.940	TestArtifactRouteRefusesNonHTMLAndForeignKeys
0.930	TestArtifactRouteServesHTMLUnderSandboxCSP
0.920	TestArtifactStreamsSamePolicyWhenTooLargeForCache
0.940	TestArtifactStreamsSamePolicyWithoutCache
0.980	TestAssigneeSetAndClear
0.920	TestAttachmentCacheMissesAfterSiteSwitch
0.910	TestAttachmentCacheRejectsForeignIssueKey
0.920	TestAttachmentEmptyExternalIDFallsBackToStoreID
0.900	TestAttachmentIsFetchedOnceThenServedFromDisk
0.000	TestAttachmentMissReasonDistinguishesScopeMismatch
0.920	TestAttachmentProxyNeedsCredential
0.920	TestAttachmentProxyStreamsFromJira
0.900	TestAttachmentStoreIdDoesNotBypassExternalID
0.920	TestAttachmentUnknownIdIsNotFound
0.940	TestAttachmentWarmGoroutinesStopOnShutdown
1.040	TestAvailableProjectsListsTheSite
2.210	TestAvailableProjectsNeedsACredential
1.280	TestBenchSmoke
1.020	TestBindOriginHandlerAfterLazyUseTakesEffect
0.940	TestBindOriginHandlerNilUnbindsBackToLazy
1.410	TestBindOriginHandlerReplacesTheTarget
0.940	TestBoardsEndpoint
0.940	TestBootstrapBodyOmitsLastSessionEnd
0.920	TestBootstrapBoundaryHeaderAbsentWithoutVisits
0.940	TestBootstrapBoundaryHeaderMatchesBody
0.950	TestBootstrapBoundaryHeaderOnHEAD
0.940	TestBootstrapBoundaryHeaderSurvives304
0.950	TestBootstrapCarriesLastSessionEnd
0.930	TestBootstrapLastSessionEndAbsentWithoutVisits
0.940	TestBootstrapShapeAndETag
0.910	TestBootstrapSurvivesABrokenGroupQuery
0.950	TestBootstrapSurvivesLocalReadError
0.950	TestBotActorSurfaces
1.020	TestBotWebPayloads
0.940	TestBrowserGuardAllowedHosts
0.920	TestBrowserGuardForbiddenHost
0.890	TestBrowserGuardForbiddenOrigin
0.880	TestBrowserGuardMatchingOriginAllowed
0.900	TestBrowserGuardNoOriginAllowed
0.880	TestBrowserGuardNullOriginForbidden
0.910	TestBrowserGuardPlainGetForeignOriginStillAllowed
0.910	TestBrowserGuardTauriOriginWithoutPairingForbidden
0.900	TestBrowserGuardWebSocketForeignOriginForbidden
0.890	TestBrowserGuardWebSocketMatchingOriginAllowed
1.000	TestBuiltInAttachmentIsServedNotA502
1.980	TestBuiltInAttachmentKeepsTheOriginsMime
0.990	TestBuiltInAttachmentRevalidatesWithoutA502
0.980	TestBuiltInAttachmentSeeksWithRange
1.840	TestBuiltInInitIsIdempotentOnBuiltInWorkspace
1.190	TestBuiltInInitRefusesConnectedWorkspace
1.280	TestBuiltInInitSeedsWorkspaceAndServesItImmediately
1.020	TestBuiltInOriginPersistFailureIsNotCredentialRequired
1.250	TestBuiltInOriginSecondSessionWrites
1.020	TestBuiltInUploadHTMLAttachmentMirrorsTextHTML
0.960	TestBurnupEndpoint
0.000	TestCachedAttachmentETagCarriesNoSiteOrWorkspace
0.900	TestCachedAttachmentSurvivesCredentialRemoval
0.920	TestCachedSvgIsForcedToDownload
0.910	TestCommentAndCreateRefuseStrayPlaceholders
0.980	TestCommentPassesVisibilityAndInternal
0.980	TestCommentResponseEchoesRestriction
0.910	TestCommentSendsMentionsAsADF
0.920	TestComponentsBuiltinEditMetaAndWrite
0.910	TestConnectDoesNotFireWhenCredentialAlreadyPresent
1.350	TestConnectEmptyBuiltInClearsKind
0.900	TestConnectFiresSyncStarterOnce
5.650	TestConnectNamesTheTokenTrapItCanRecognise
1.960	TestConnectRefusesBuiltInWithLocalData
1.250	TestConnectRefusesDifferentSite
1.000	TestConnectRejectsInvalidExpiry
1.840	TestConnectReplaceBuiltInClearsKind
0.980	TestConnectReplaceBuiltInDropsWatchesFavoritesAndLOC
0.990	TestConnectReportsARejectedCredential
1.650	TestConnectRequiresSiteAndCredential
1.010	TestConnectStoresUserExpiry
0.900	TestConnectSyncStarterUnregisteredIsNoop
0.970	TestConnectVerifiesStoresSiteAndHidesTheToken
0.930	TestContentRouteStillDownloadsHTML
0.910	TestCreateFieldsCarriesKindAndOptions
0.910	TestCreateFieldsMissingQuery
0.890	TestCreateFieldsNoCredential
0.910	TestCreateFieldsOriginErrorDoesNot500
0.920	TestCreateFieldsReturnsRequiredSet
0.940	TestCreateIssue
0.950	TestCreateIssueCustomFieldsEmptyIsOmitted
0.920	TestCreateIssueCustomFieldsRefusesUnknownAndFixed
0.930	TestCreateIssueCustomFieldsWrappedByKind
0.930	TestCreateIssueDefaultProject
0.940	TestCreateIssueEmptyOptionalFieldsOmittedFromPayload
0.950	TestCreateIssueEmptyProjectsAllows
0.890	TestCreateIssueEmptySummaryStillRequired
0.980	TestCreateIssueIgnoresPriorityNameField
0.930	TestCreateIssueNormalizesParentKey
0.960	TestCreateIssueOmitsEmptyDuedate
0.970	TestCreateIssueOmitsEmptyParent
0.910	TestCreateIssueOmitsProjectFailsWhenAmbiguous
0.920	TestCreateIssueOmitsTypeFailsWhenMany
0.920	TestCreateIssueOmitsTypeUsesConfigDefault
0.940	TestCreateIssueOmitsTypeUsesSole
0.910	TestCreateIssueRejectsInvalidDuedateBeforeJira
0.880	TestCreateIssueRejectsInvalidParentBeforeJira
0.940	TestCreateIssueReportsOrigin
0.960	TestCreateIssueSendsDuedate
0.950	TestCreateIssueSendsParent
0.940	TestCreateIssueSendsPriorityByID
0.880	TestCreateIssueStaleDefaultType
0.900	TestCreateMetaForwardsSubtaskAndHierarchyLevel
0.920	TestCreatePairedUnreachableIsNotProjectRequired
0.900	TestCreateRESTErrorBodiesHaveNoCLIFlagTokens
0.970	TestCreateRoutesLinearTeamInMixedWorkspace
0.910	TestCredentialAdvertisesLinear
0.940	TestCredentialLifecycle
0.900	TestDashboardAbsorb
0.940	TestDashboardDataErrors
0.910	TestDashboardDataJQL
0.990	TestDashboardDataSQL
0.940	TestDashboardDataWriteSQLRefused
0.900	TestDashboardGetRow
0.890	TestDashboardLibRoute
0.920	TestDashboardRenderCSP
0.890	TestDashboardRenderCorruptConfig
0.940	TestDashboardRenderLibs
0.920	TestDashboardSaveLibs
0.900	TestDashboardSaveListDelete
0.950	TestDashboardSaveValidation
0.900	TestDashboardVendorRoute
0.910	TestDeferredEndpointsAre404
0.940	TestDeltaCarriesBoundaryHeader
0.940	TestDeltaOmitsLastSessionEnd
1.000	TestDeltaUpsertedAndDeleted
1.000	TestDerivedReleasesLockDuringRebuild
1.050	TestDescriptionPlaceholdersRoundTrip
0.980	TestDescriptionSetAndClear
0.970	TestDetailAssembly
0.950	TestDetailAttachmentsEmptyIsArray
1.040	TestDetailDerivesLinkedPRsFromAttachments
0.970	TestDetailDerivesLinkedPRsFromRemoteLinks
0.950	TestDetailFlagsHTMLAttachmentsAsArtifacts
0.960	TestDetailHistoryReopenVerdict
1.040	TestDetailHistoryReopenWireSpelling
0.980	TestDetailLinkPhraseFromBackend
1.010	TestDetailPresentsBodiesAsMarkdown
1.010	TestDetailSurvivesLocalReadError
0.990	TestDetailVisitsAbsentWithoutVisits
1.000	TestDetailVisitsCarryPersonReadTimestamps
0.910	TestEditDuedateRejectsInvalidBeforeJira
1.010	TestEditDuedateSetAndClear
1.000	TestEditMetaFromFieldSpecs
1.790	TestEditMetaOnlyExposesAllowlistedFields
0.930	TestEditParentRejectsInvalidBeforeJira
0.960	TestEditParentRejectsSelfBeforeJira
1.070	TestEditParentSetAndClear
0.950	TestEnrichmentCannotShadowMirroredFields
1.140	TestEnrichmentsMerge
0.000	TestFailJiraMapsOriginHTTP
0.000	TestFailJiraMapsUnsupportedAndRefused
0.910	TestFavoritesRoundtrip
0.000	TestFeedDefaultFeatureFlag
0.910	TestFeedFocusAssignee
0.940	TestFeedGetAndMarkRead
1.010	TestFeedReopenedEvent
0.000	TestFetchStoredURLRefusesUploadsSubdomainRedirect
0.000	TestFetchStoredURLStripsAuthorizationOnCrossHostRedirect
1.020	TestFieldEditAllowlistAndShapes
1.040	TestFieldEditScalarKinds
1.470	TestFirstSyncIsSingleFlightAndReportsProgress
1.460	TestFlowMemoErrorNotCached
1.240	TestFlowMemoHoldsAcrossBootstrapAndDelta
1.270	TestFlowMemoInvalidatesOnSyncVersion
1.210	TestFlowMemoSurvivesThresholdSet
0.930	TestFrozenWorkspaceRefusesResync
1.150	TestFrozenWorkspaceRefusesWrites
0.080	TestGateLogsNeverFormatTheBearer
0.000	TestGroupFallbackUsesAssigneeAccountID
0.000	TestGuardBrowserHostPolicyAdmitsListedName
0.000	TestGuardBrowserHostPolicyRejectsUnlistedDNS
0.000	TestGuardBrowserNilPolicyBehavesAsToday
0.000	TestGuardBrowserWrapsNext
0.890	TestHandlerShutdownCancelsSyncJob
1.910	TestHealthExposesConfluenceWhenSourcePresent
0.960	TestHistoryCursorLimit
0.990	TestHistoryDeleteClearsVisitsAndSearches
0.920	TestHistoryInvalidCursor
0.940	TestHistoryPatchMissingSearch
0.900	TestHistoryRejectsBadKind
0.960	TestHistorySearchAndOpened
0.960	TestHistoryVisitAppendAndList
1.020	TestHistoryVisitedKeysFoldsPerKey
0.000	TestHostPolicyAllowsIsCaseInsensitiveAndUnlistedStaysFalse
0.000	TestHostPolicyCarriesOwnerLogin
0.000	TestHostPolicyDescribeNamesSources
0.930	TestHydrateRefs
0.940	TestIdentityHeadersEmptyProfile
0.940	TestIdentityHeadersOnAPIResponses
0.920	TestIdentityHeadersOnForbiddenHost
0.890	TestImportManifestEmptySiteKeepsLegacyKey
0.930	TestImportManifestRejectsForeignIssueKey
0.900	TestImportManifestServesFromCacheWithoutUpstream
0.000	TestImportManifestSkipsIDMissingFromMirror
0.000	TestIsLinearUploadsURL
0.930	TestIssueLiteFieldNames
0.920	TestIssueLiteHierarchyLevelOnBootstrap
0.920	TestIssueResyncNotFound
0.960	TestIssueResyncRefreshesMirror
0.920	TestJiraAttachmentStillUsesSiteBasicAuth
0.930	TestJiraErrorsPassThrough
0.970	TestJqlCurrentUserResolvesToAccountID
0.000	TestJqlDoesNotFullScanIssueLites
0.980	TestJqlParseAndEmit
0.950	TestJqlReporterNameResolvesToAccountID
1.140	TestKeyPrioritiesRoutesBySource
0.910	TestKeyUsersMatchesGlobalOnJiraRow
0.980	TestLabelsSetAndClear
0.940	TestLinearAttachmentFetchesStoredURLWithoutAuth
0.930	TestLinearAttachmentOtherHostIsNotFetched
0.970	TestLinearAttachmentPassesThrough401
0.910	TestLinearKeyPrioritiesWithoutJiraCredential
0.960	TestLinearOnlyCreateRoutesToLinear
0.960	TestLinearOnlyFieldEditRoutesToLinear
0.960	TestLinearUploadsAttachmentSendsBareAPIKey
3.170	TestLinkRESTAmbiguousPhraseRefusesAndTheChosenIDDoesNot
0.980	TestLinkRESTInwardDescriptionMakesPathIssueDisplayInwardDescription
2.680	TestLinkRESTMachinePathRefusesAnUnknownIDAndABadDirection
1.270	TestLinkRESTOriginFailureLeavesMirrorUnchanged
1.020	TestLinkRESTOutwardDescriptionMakesPathIssueDisplayOutwardDescription
1.240	TestLinkRESTRequiresCredential
1.170	TestLinkRESTSelfRefusedNoOrigin
0.990	TestLinkRESTUnknownTypeDoesNotPOST
0.940	TestLinkTypesREST
0.900	TestMe
0.960	TestMembersIncludeAccountIDOnlyRosterRows
1.080	TestMirrorAllowlistTable
0.990	TestMirrorGateCommentWriteGoesThroughOrigin
1.170	TestMirrorGateCredentialHintedNoToken
0.880	TestMirrorGateDemandsBearer
1.020	TestMirrorGateHonorsHostPolicy
1.050	TestMirrorGateLoopbackUnchanged
0.960	TestMirrorGateScopeBoundaries
1.100	TestMirrorGateServeBearerOpensBootstrap
0.870	TestMirrorGateUnpairedDNSHostStaysForbidden
0.000	TestNewHostPolicyParsesOriginsToNames
0.000	TestNewHostPolicySkipsUnparseableEntries
0.000	TestNilAndEmptyHostPolicyAdmitNothing
0.000	TestNormalizeSiteAcceptsWhatPeoplePaste
0.000	TestOriginClientLinearOnlyNeedsAtlassian
0.910	TestOriginExportConnectedIsRefused
0.930	TestOriginExportServesSeedYAML
0.930	TestOriginRESTBuiltInPOSTWithoutOriginAllowed
0.950	TestOriginRESTBuiltInPassesThrough
0.940	TestOriginRESTConnectedIs404
0.960	TestOriginRESTPreservesActorHeaderWithoutGate
0.000	TestOsNotifySupportedJSONIncludesFalse
1.050	TestOverlappingSyncKicksRunOnce
0.910	TestPageArtifactRouteServesFromCacheAndRefusesNonHTML
0.980	TestPageAttachmentWireMatchesIssueDetail
0.940	TestPageCommentMissingIsNotFound
1.000	TestPageCreateInvalidADF
1.160	TestPageCreateUpdatesMirrorAfterOrigin
0.960	TestPageDetail200And404
1.040	TestPageEditExplicitVersionConflict
0.940	TestPageEditInvalidADF
1.530	TestPageEditTextFormatLoss
0.970	TestPageResyncNotFound
1.070	TestPageResyncRefreshesMirror
0.940	TestPagesListAndETag
0.940	TestPagesResponseIncludesAuthorID
0.930	TestPagesResponseIncludesLabels
0.930	TestPagesResponseIncludesSpaceHomepageID
0.930	TestPagesResponseIncludesSpaceName
0.930	TestPairedCredentialSurfaceIsMeasured
1.000	TestPairedHostExemptLetsTailnetNameThrough
0.880	TestPairedWrite401IsPairingErrorNotCredentialRejected
1.020	TestPairingGate401CarriesRejectReason
1.150	TestPairingGateAcceptsValidBearer
1.550	TestPairingGateExpiredAndRevokedRejected
0.900	TestPairingGateKeepsActorHeaderWhileRewritingAuth
1.010	TestPairingGateOffWithoutTokens
0.920	TestPairingGateRejectsWithoutValidBearer
0.990	TestPairingTokenNeverReachesTheLog
0.960	TestParentBuiltinEditMetaAndWrite
0.900	TestPeopleCommentsEmptyAuthor
0.940	TestPeopleCommentsLimitAndInvalid
0.910	TestPeopleCommentsOK
0.910	TestPersonalStateRoundtrip
0.000	TestPhoneUIHandlerMissingBundleIs503
0.000	TestPhoneUIHandlerServesIndexAssetsAndFallback
0.000	TestPhoneURLs
0.910	TestPostRecentRejectsEmpty
0.910	TestPreviewRendersMarkdownAsADF
0.000	TestPrimaryFocusTakesProcessProfileFile
0.960	TestPrioritySetAndClear
1.780	TestPutCredentialKeepsStoredTokenOnEmpty
0.910	TestPutCredentialStoresUserExpiry
0.950	TestPutSettingsAppearanceRejectsShape
0.940	TestPutSettingsAppearanceRoundtrip
0.950	TestPutSettingsConfluenceDisable
0.950	TestPutSettingsConfluenceEnableFromOff
0.960	TestPutSettingsConfluenceEnableReplacesSpaces
0.940	TestPutSettingsConfluenceSpacesNotConfigured
1.060	TestPutSettingsConfluenceSpacesOnlyWhileOff
1.040	TestPutSettingsConfluenceSpacesOnlyWhileOn
0.940	TestPutSettingsConfluenceSpacesRoundtrip
1.270	TestPutSettingsDoesNotClearFrozen
0.950	TestPutSettingsNonScopeNoKick
0.940	TestPutSettingsOmitsAppearancePreserves
0.900	TestPutSettingsOmitsConfluenceKeyLeavesSpaces
0.970	TestPutSettingsOmitsConfluenceKeyPreserves
0.910	TestPutSettingsScopeChangeKicksFullSync
0.920	TestPutSettingsScopeChangeNoCredentialNoKick
0.920	TestPutSettingsTerminalDisplayOnly
0.900	TestPutSettingsUIJudgmentWarnsAndSaves
0.000	TestRESTContractGoldensListed
0.940	TestRESTParentRejectionCreateCarriesHierarchyHint
0.930	TestRESTParentRejectionEditCarriesHierarchyHint
0.950	TestRESTParentRejectionUnrelated400HasNoHint
0.960	TestRESTResponseGoldens
0.920	TestRangedViewOfACacheableAttachmentFillsTheCache
0.940	TestRangedViewOfAnOversizeAttachmentStreamsPastTheCache
0.910	TestRecentsAPIParityWithSQL
0.920	TestRecentsRouteExists
0.880	TestRejectedCredentialIsNotStored
0.080	TestRequestPathWallClockAllowlist
0.000	TestResolveLinkTypeMatchesCLI
0.990	TestRetroAmbiguousBoardCarriesTheBoards
1.040	TestRetroEndpoint
0.970	TestRetroEndpointCarriesTheMaterials
1.010	TestRetroEndpointSessionGapConfigDefault
0.000	TestRoutesRegister
1.070	TestRunSyncJobPullsLinear
0.970	TestRuntimeInfoReportsOriginAndAttachmentBytes
0.940	TestSearchHitsCommentText
0.970	TestSearchResponseIncludesPages
0.930	TestServeAnnouncesVersionHeader
0.890	TestServeHTTPAttachesViewerActorContext
0.000	TestServeScopeIsDefaultClosed
0.000	TestServedConfigCarriesBothAxes
0.000	TestServerClockPinning
0.000	TestSettingsCatalogCoversPUTFields
0.930	TestSettingsFieldSpecsAndUsageReadOnly
0.920	TestSettingsIntervalFloorsReject
0.920	TestSettingsOsNotifySupportedFollowsInjectedNotifier
0.950	TestSettingsRoundtripPreservesCredential
0.970	TestSettingsRuntimeCountsMatchIssueLites
0.890	TestSettingsRuntimeProfileName
0.940	TestSettingsRuntimeReadOnlyNoSecrets
0.920	TestSettingsSpacesCarriesPageCount
0.910	TestSettingsSpacesCountingIsBounded
0.900	TestSettingsSpacesEmptyMeansAllGlobalFlag
0.910	TestSettingsSpacesListsSelectedAndSorts
0.920	TestSettingsSpacesListsWhenCountingFails
0.910	TestSettingsSpacesOffListsWithEnabledFalse
0.910	TestSettingsSpacesOffNoCredential
0.930	TestSettingsSpacesOmitsUncountableSpace
0.000	TestSettingsSpacesOriginSentinelIsNotCredentialRequired
0.920	TestSettingsSyncIntervalsRoundtrip
0.950	TestSettingsUIRoundtripAndRefusal
0.890	TestSnapshotSyncMatchesProgressEndpoint
1.330	TestStartSyncAcceptsIncrementalMode
1.400	TestStartSyncFrozenIsNotInProgress
1.280	TestStartSyncJobNoopsWhenWatchActivityRunning
0.990	TestStartSyncRefusesAnIncompleteSetup
0.930	TestSummarySet
0.900	TestSyncActivityDoesNotImplyJobRunning
0.930	TestSyncActivityHooksOnProgress
0.900	TestSyncActivityPhaseIdleCloses
0.890	TestSyncActivitySourceChangeResetsCounters
0.940	TestSyncHealth
0.920	TestSyncHealthTokenExpiry
0.950	TestSyncProgressCarriesFirstSync
0.970	TestSyncProgressClassifiesARejectedCredential
1.020	TestSyncProgressFirstSyncDocumentsPhase
1.000	TestSyncProgressLeavesATransportFailureUnclassified
1.380	TestSyncProgressStartsIdle
0.950	TestSyncRunsLastCheckedAt
0.930	TestSyncRunsSourceFilter
0.260	TestTailscaleDNSNameEmptyWhenNoSelfName
0.260	TestTailscaleDNSNameGarbageOutputIsEmpty
0.000	TestTailscaleDNSNameNotOnPathIsEmpty
0.270	TestTailscaleDNSNameParsesSelfDNSName
0.050	TestTailscaleDNSNameTimeoutIsEmpty
0.250	TestTailscaleSelfParsesOwnerLogin
0.260	TestTailscaleSelfTaggedNodeHasNoOwner
0.250	TestTailscaleSelfUnknownUserHasNoOwner
0.260	TestTailscaleSelfZeroUserIDHasNoOwner
0.880	TestTerminalAllowRemoteAddressNeedsAToken
0.890	TestTerminalCreateBindsIssueAndName
0.890	TestTerminalCreateResponseBehaviorDefaults
0.920	TestTerminalCreateResponseCarriesBehavior
0.920	TestTerminalCreateUsesConfiguredShellAndDir
1.960	TestTerminalCreateWorkingDirMissingFallsBack
0.900	TestTerminalDNSHostDemandsBearer
0.880	TestTerminalGateCoversTheWholeSubtree
0.860	TestTerminalGateIgnoresDeclaredActorName
1.780	TestTerminalInputBoundsAndDoors
2.330	TestTerminalInputPlacesTextWithoutRunningIt
1.810	TestTerminalIssueBinding
0.890	TestTerminalListIsMetadataOnly
0.000	TestTerminalLocalHostIsLoopbackOnly
0.000	TestTerminalLocalNeedsLoopbackPeer
1.940	TestTerminalLoopbackOpensAndEchoes
0.890	TestTerminalOtherViewerIsRefusedByName
0.940	TestTerminalOwnerViewerOpensShellWithoutBearer
0.000	TestTerminalPathIsNotMirrorREST
0.920	TestTerminalRename
0.930	TestTerminalRevokeCutsLiveSession
1.020	TestTerminalRevokeWatchLeavesOwnerSessionsAlone
0.880	TestTerminalScopeIsAOneWayDoor
3.920	TestTerminalSessionNamesItsWorkspace
0.870	TestTerminalSessionsAreScopedToTheirToken
0.910	TestTerminalShutdownReapsSessions
2.000	TestTerminalSocketGoroutinesStopOnShutdown
0.000	TestTerminalSurfaceNeverWritesToOrigin
0.890	TestTerminalTaggedNodeKeepsBearerRules
0.880	TestTerminalTailnetSessionsAreScopedToTheirLogin
0.000	TestTerminalTokenLogIsHashedNotPlaintext
0.880	TestTerminalTokenOpensShellOverDNSHost
0.930	TestTerminalViewerHeaderFromUntrustedPeerIsIgnored
0.910	TestTerminalWebviewOriginCannotOpenTheSocket
0.940	TestTokenNeverReachesResponsesOrLogs
0.930	TestTransitionRESTFieldAliasRemap
0.950	TestTransitionRESTForwardsFieldsAndComment
5.620	TestTransitionRESTIsTheMachineIDContract
0.980	TestTransitionRESTPickedIDDoesNotFoldWhenAlreadyThere
0.990	TestTransitionRESTPickedIDIsExactOnFoldedNames
0.920	TestTransitionRESTRequiredResolutionAcceptsFieldsID
0.910	TestTransitionRESTRequiredResolutionRefusesWithoutValue
0.930	TestTransitionRESTResolutionNameUsesAllowedValues
0.990	TestTransitionRESTWithoutExtrasOmitsFieldsAndUpdate
0.930	TestTransitionWritesThroughToTheMirror
0.890	TestTransitionsAndUsersAndCreateMeta
0.900	TestTransitionsGETIncludesTargetStatusID
0.900	TestTransitionsGETRequiredFieldsOnly
0.900	TestUIFocusAlwaysCarriesConfigVersion
1.220	TestUIFocusCarriesMirrorVersionThatMovesWithTheMirror
0.010	TestUIFocusLogsOncePerProfileNotPerPoll
0.000	TestUIFocusPeekReturnsHashAndAtTwice
0.000	TestUIFocusReadDoesNotConsume
0.000	TestUIFocusWithoutAMirrorStillServesFocus
0.900	TestUncacheableAttachmentIsFetchedExactlyOnce
0.920	TestUncachedAttachmentWithoutCredentialAsksForOne
0.890	TestUploadMirrorRereadFailureIs502
0.920	TestUploadProxiesAndReturnsContentURL
0.000	TestViewerActorDerivation
0.000	TestViewerDecodesEncodedName
0.910	TestViewerDocumentCarriesDeclaredName
0.000	TestViewerFromLoopbackTailscaleHeaders
0.000	TestViewerFromLoopbackWithoutHeadersIsNone
0.000	TestViewerFromNonLoopbackHeadersIgnored
0.900	TestViewerRouteAnswersDocument
0.000	TestViewerTrustsOwnInterfaceAddress
0.950	TestViewsIncludeSourceQueries
0.950	TestWebCommentCarriesNoActorTrailer
0.000	TestWebConfigCapabilitiesBlock
0.000	TestWebConfigCarriesPhoneURLs
0.920	TestWebConfigCarriesUI
0.920	TestWebConfigHidesCredential
0.000	TestWebConfigOriginWritableMirrorsHasAtlassianCredential
0.000	TestWebConfigProfileName
0.000	TestWebConfigUIDimensionVars
0.000	TestWebConfigUIFonts
0.000	TestWebConfigWorkspaceKindFromDescribe
0.940	TestWikiServerDetailCarriesVerbatim
0.960	TestWikiServerPreviewIsCodeBlock
1.010	TestWikiServerWritesSendVerbatimString
0.930	TestWikiWritesRequireCredential
0.900	TestWithViewerActorHonoursDeclaredName
0.900	TestWithViewerActorIgnoresDeclaredNameOnNonGadak
0.900	TestWithViewerActorIgnoresUnusableDeclaredName
0.910	TestWithViewerActorVerifiedViewerWins
0.000	TestWorkspaceFocusTakesProfileFile
3.630	TestWriteAppliedMirrorStaleStatusUnchanged
0.020	TestWriteHandlersDoNotCallClient
0.910	TestWritesRequireACredential
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
