/*
 * Settings, theme, palette, sync.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const settings = {
  /* ── Server settings ── */
  'settings.title': {
    en: 'Settings',
    ko: '설정',
    ja: '設定',
  },
  'settings.tabSync': {
    en: 'Sync',
    ko: '동기화',
    ja: '同期',
  },
  'settings.tabSources': {
    en: 'Sources',
    ko: '소스',
    ja: 'ソース',
  },
  'settings.tabFeatures': {
    en: 'Features',
    ko: '기능',
    ja: '機能',
  },
  'settings.tabTeams': {
    en: 'Teams / groups',
    ko: '팀/그룹',
    ja: 'チーム / グループ',
  },
  'settings.tabMembers': {
    en: 'Members',
    ko: '멤버',
    ja: 'メンバー',
  },
  'settings.tabFields': {
    en: 'Field mapping',
    ko: '필드 매핑',
    ja: 'フィールドマップ',
  },
  'settings.tabIntegrations': {
    en: 'Integrations',
    ko: '연동',
    ja: '連携',
  },
  'settings.tabAbout': {
    en: 'About',
    ko: '정보',
    ja: '情報',
  },
  'settings.aboutFeedback': {
    en: 'Feedback',
    ko: '피드백',
    ja: 'フィードバック',
  },
  'settings.aboutGithub': {
    en: 'GitHub repository',
    ko: 'GitHub 저장소',
    ja: 'GitHub リポジトリ',
  },
  'settings.aboutIssues': {
    en: 'Report an issue',
    ko: '버그·기능 요청',
    ja: '課題を報告',
  },
  'settings.aboutEmail': {
    en: 'Contact by email',
    ko: '이메일',
    ja: 'メールで連絡',
  },
  'settings.aboutX': {
    en: '@midagedev on X',
    ko: '@midagedev on X',
    ja: '@midagedev on X',
  },
  'settings.intro': {
    en: 'Choose what this workspace mirrors, how often it syncs, and which features are on. Saving re-reads this window.',
    ko: '이 워크스페이스가 미러링할 대상, 동기화 주기, 켤 기능을 정합니다. 저장하면 이 창을 다시 읽습니다.',
    ja: 'このワークスペースがミラーする対象、同期の頻度、オンにする機能を選びます。保存するとこのウィンドウを再読み込みします。',
  },
  'settings.loadFailed': {
    en: 'Could not load settings. Close and reopen.',
    ko: '설정을 불러오지 못했습니다. 닫은 뒤 다시 여세요.',
    ja: '設定を読み込めませんでした。閉じて開き直してください。',
  },
  'settings.saveFailed': {
    en: 'Could not save settings. Try Save again.',
    ko: '설정 저장에 실패했습니다. 저장을 다시 시도하세요.',
    ja: '設定を保存できませんでした。もう一度保存してください。',
  },
  'settings.savedReload': {
    en: 'Settings saved. Reloading…',
    ko: '설정을 저장했습니다. 새로고침합니다…',
    ja: '設定を保存しました。再読み込みしています…',
  },
  'settings.jsonParseError': {
    en: 'JSON parse error — fix it to re-enable save.',
    ko: 'JSON 파싱 실패 — 고치면 저장이 다시 활성화됩니다.',
    ja: 'JSON の解析エラー — 直すと保存が再び有効になります。',
  },
  'settings.projects': {
    en: 'Project keys to mirror (comma-separated)',
    ko: '미러링할 프로젝트 키 (콤마 구분)',
    ja: 'ミラーするプロジェクトキー（カンマ区切り）',
  },
  'settings.projectsManual': {
    en: 'The project list could not be read from the site, so keys are entered by hand here.',
    ko: '사이트에서 프로젝트 목록을 읽지 못해, 여기서는 키를 직접 입력합니다.',
    ja: 'サイトからプロジェクト一覧を読めなかったため、ここではキーを手入力します。',
  },
  'settings.sourcesProjects': {
    en: 'Jira projects',
    ko: 'Jira 프로젝트',
    ja: 'Jira プロジェクト',
  },
  'settings.sourcesProjectsHint': {
    en: 'Only these projects are mirrored.',
    ko: '선택한 프로젝트만 미러링합니다.',
    ja: 'これらのプロジェクトだけをミラーします。',
  },
  'settings.sourcesNoProjects': {
    en: 'Nothing selected — every project this account can see is mirrored.',
    ko: '선택 없음 — 이 계정이 볼 수 있는 모든 프로젝트가 미러링됩니다.',
    ja: '未選択 — このアカウントが見られるすべてのプロジェクトをミラーします。',
  },
  'settings.confluenceTitle': {
    en: 'Confluence',
    ko: 'Confluence',
    ja: 'Confluence',
  },
  'settings.confluenceOffHint': {
    en: 'Off for this workspace — no document is mirrored. Choose spaces below to start, or turn it on for every team space.',
    ko: '이 워크스페이스에서는 꺼져 있어 위키 문서가 미러링되지 않습니다. 아래에서 스페이스를 고르면 시작되고, 팀 스페이스 전체로 켤 수도 있습니다.',
    ja: 'このワークスペースではオフです — ドキュメントはミラーされません。下でスペースを選ぶか、すべてのチームスペースでオンにしてください。',
  },
  'settings.confluenceOnHint': {
    en: 'Documents are mirrored alongside issues.',
    ko: '위키 문서가 이슈와 함께 미러링됩니다.',
    ja: 'ドキュメントは課題と一緒にミラーされます。',
  },
  'settings.confluenceTurnOnCount': {
    en: 'Turn on for {n} spaces',
    ko: '{n}개 스페이스로 켜기',
    ja: '{n}件のスペースでオン',
  },
  'settings.confluenceTurnOnAll': {
    en: 'Turn on for every team space',
    ko: '전체 팀 스페이스로 켜기',
    ja: 'すべてのチームスペースでオン',
  },
  'settings.confluenceTurnOnAllConfirm': {
    en: 'Click again to mirror every team space',
    ko: '한 번 더 누르면 모든 팀 스페이스를 미러링합니다',
    ja: 'もう一度クリックするとすべてのチームスペースをミラーします',
  },
  'settings.confluenceTurnOff': {
    en: 'Turn off',
    ko: '끄기',
    ja: 'オフにする',
  },
  'settings.confluenceAllWarning': {
    en: 'No space selected: every team (global) space will be mirrored to disk. Personal spaces are mirrored only when named.',
    ko: '선택한 스페이스가 없습니다. 모든 팀(global) 스페이스가 디스크로 내려옵니다. 개인 스페이스는 직접 지정할 때만 포함됩니다.',
    ja: 'スペース未選択: すべてのチーム（global）スペースがディスクにミラーされます。個人スペースは名前を指定したときだけです。',
  },
  'settings.sourcesSpaces': {
    en: 'Confluence spaces',
    ko: 'Confluence 스페이스',
    ja: 'Confluence スペース',
  },
  'settings.sourcesSpacesHint': {
    en: 'Only these spaces are mirrored.',
    ko: '선택한 스페이스만 미러링합니다.',
    ja: 'これらのスペースだけをミラーします。',
  },
  'settings.sourcesAllGlobal': {
    en: 'Nothing selected — every team (global) space is mirrored.',
    ko: '선택 없음 — 모든 팀(global) 스페이스를 미러링합니다.',
    ja: '未選択 — すべてのチーム（global）スペースをミラーします。',
  },
  'settings.sourcesNoSpaces': {
    en: 'Nothing selected — no document is mirrored.',
    ko: '선택 없음 — 미러링되는 문서가 없습니다.',
    ja: '未選択 — ドキュメントはミラーされません。',
  },
  'settings.showPersonalSpaces': {
    en: 'Show personal spaces',
    ko: '개인 스페이스도 표시',
    ja: '個人スペースを表示',
  },
  'settings.spacesUnavailable': {
    en: 'Could not read the space list from Confluence.',
    ko: 'Confluence에서 스페이스 목록을 읽지 못했습니다.',
    ja: 'Confluence からスペース一覧を読めませんでした。',
  },
  'settings.sourcesApplyHint': {
    en: 'Saving starts a full sync immediately.',
    ko: '저장하면 전체 동기화를 바로 시작합니다',
    ja: '保存するとすぐに全同期が始まります。',
  },
  'settings.scopeLoading': {
    en: 'Loading the list…',
    ko: '목록을 불러오는 중…',
    ja: '一覧を読み込み中…',
  },
  'settings.scopeNoMatch': {
    en: 'No matches',
    ko: '일치하는 항목 없음',
    ja: '一致なし',
  },
  'settings.scopeHint': {
    en: '↑↓ move · ↵ add · Esc close',
    ko: '↑↓ 이동 · ↵ 추가 · Esc 닫기',
    ja: '↑↓ 移動 · ↵ 追加 · Esc 閉じる',
  },
  'settings.scopeRemove': {
    en: 'Remove {name}',
    ko: '{name} 제거',
    ja: '{name} を削除',
  },
  'settings.scopeProjectPlaceholder': {
    en: 'Type a project key or name…',
    ko: '프로젝트 키나 이름을 입력…',
    ja: 'プロジェクトのキーまたは名前を入力…',
  },
  'settings.scopeSpacePlaceholder': {
    en: 'Type a space key or name…',
    ko: '스페이스 키나 이름을 입력…',
    ja: 'スペースのキーまたは名前を入力…',
  },
  'settings.staleHours': {
    en: 'Stale threshold (hours)',
    ko: '지연 판정 기준 (시간)',
    ja: '滞留のしきい値（時間）',
  },
  'settings.staleHint': {
    en: 'Open issues in the same status longer than this are marked stale.',
    ko: '현재 상태에 이 시간 이상 머문 미해결 이슈를 지연으로 표시합니다.',
    ja: '同じステータスにこれより長くいる未解決課題を滞留とマークします。',
  },
  'settings.syncInterval': {
    en: 'Incremental sync interval',
    ko: '증분 동기화 주기',
    ja: '増分同期の間隔',
  },
  'settings.syncIntervalHint': {
    en: 'How often gadak serve polls Jira for changes. 0 uses the default.',
    ko: 'gadak serve가 Jira 변경을 폴링하는 간격. 0이면 기본값.',
    ja: 'gadak serve が Jira の変化をポーリングする頻度。0 は既定値です。',
  },
  'settings.syncIntervalHintDesktop': {
    en: "How often this window's background sync looks at Jira. 0 uses the default.",
    ko: '이 창의 백그라운드 동기화가 Jira를 보는 간격. 0이면 기본값.',
    ja: 'このウィンドウのバックグラウンド同期が Jira を見る頻度。0 は既定値です。',
  },
  'settings.reconcileInterval': {
    en: 'Reconcile interval (deletions)',
    ko: '삭제 정리(reconcile) 주기',
    ja: 'リコンサイル間隔（削除）',
  },
  'settings.reconcileIntervalHint': {
    en: 'How often gadak serve re-lists keys to drop issues deleted upstream. 0 uses the default.',
    ko: '원본에서 삭제된 이슈를 정리하기 위해 키를 재목록화하는 간격. 0이면 기본값.',
    ja: 'gadak serve がキーを再一覧して上流で削除された課題を落とす頻度。0 は既定値です。',
  },
  'settings.reconcileIntervalHintDesktop': {
    en: "How often this window's background sync re-lists keys to drop issues deleted upstream. 0 uses the default.",
    ko: '이 창의 백그라운드 동기화가 원본에서 삭제된 이슈를 정리하려고 키를 재목록화하는 간격. 0이면 기본값.',
    ja: 'このウィンドウのバックグラウンド同期がキーを再一覧して上流で削除された課題を落とす頻度。0 は既定値です。',
  },
  'settings.intervalApplies': {
    en: 'Applies on the next sync tick; no restart needed.',
    ko: '다음 동기화 틱에 적용됩니다. 재시작 불필요.',
    ja: '次の同期ティックで適用されます。再起動は不要です。',
  },
  'settings.intervalDefault': {
    en: 'Default',
    ko: '기본값',
    ja: '既定',
  },
  'settings.intervalDefaultSeconds': {
    en: '{n}s',
    ko: '{n}초',
    ja: '{n}秒',
  },
  'settings.intervalCustom': {
    en: 'Custom…',
    ko: '직접 입력…',
    ja: 'カスタム…',
  },
  'settings.intervalSeconds': {
    en: 'seconds',
    ko: '초',
    ja: '秒',
  },
  'settings.intervalPreset30s': {
    en: '30s',
    ko: '30초',
    ja: '30秒',
  },
  'settings.intervalPreset1m': {
    en: '1m',
    ko: '1분',
    ja: '1分',
  },
  'settings.intervalPreset5m': {
    en: '5m',
    ko: '5분',
    ja: '5分',
  },
  'settings.intervalPreset15m': {
    en: '15m',
    ko: '15분',
    ja: '15分',
  },
  'settings.intervalPreset1h': {
    en: '1h',
    ko: '1시간',
    ja: '1時間',
  },
  'settings.intervalPreset6h': {
    en: '6h',
    ko: '6시간',
    ja: '6時間',
  },
  'settings.intervalPreset24h': {
    en: '24h',
    ko: '24시간',
    ja: '24時間',
  },
  'settings.updateTitle': {
    en: 'Update',
    ko: '업데이트',
    ja: '更新',
  },
  'settings.updateReleaseNotes': {
    en: 'Release notes',
    ko: '릴리스 노트',
    ja: 'リリースノート',
  },
  'settings.updateCurrent': {
    en: 'This build is the latest published release.',
    ko: '이 빌드가 게시된 최신 릴리스입니다.',
    ja: 'このビルドが公開済みの最新リリースです。',
  },
  'settings.updateFailed': {
    en: 'Could not check for updates.',
    ko: '업데이트를 확인하지 못했습니다.',
    ja: '更新を確認できませんでした。',
  },
  'settings.updateDev': {
    en: 'Dev build — update check skipped.',
    ko: '개발 빌드 — 업데이트 확인을 건너뜁니다.',
    ja: '開発ビルド — 更新確認をスキップしました。',
  },
  'settings.thisMirror': {
    en: 'This local copy',
    ko: '이 로컬 사본',
    ja: 'このローカルコピー',
  },
  'settings.workspaceLocalOrigin': {
    en: 'Built-in',
    ko: '내장',
    ja: '内蔵',
  },
  'settings.workspaceLocalOriginHint': {
    en: "Your issues live in the built-in tracker, running here. Their only copy is this computer's tracker data file, not gadak.db — back that file up.",
    ko: '이슈는 여기서 도는 내장 트래커에 있습니다. 유일한 사본은 이 컴퓨터의 트래커 데이터 파일이며, 백업 대상은 gadak.db가 아니라 그 파일입니다.',
    ja: '課題はここで動いている内蔵トラッカーにあります。唯一のコピーはこのコンピュータのトラッカーデータファイルであり、gadak.db ではありません — そのファイルをバックアップしてください。',
  },
  'settings.localOriginHow': {
    en: 'Create a built-in tracker workspace',
    ko: '내장 트래커 워크스페이스 만들기',
    ja: '内蔵トラッカーのワークスペースを作る',
  },
  'settings.runtimeProfile': {
    en: 'Workspace',
    ko: '워크스페이스',
    ja: 'ワークスペース',
  },
  'settings.runtimeCli': {
    en: 'gadak --workspace {name}',
    ko: 'gadak --workspace {name}',
    ja: 'gadak --workspace {name}',
  },
  'settings.runtimeDb': {
    en: 'Local copy database',
    ko: '로컬 사본 DB',
    ja: 'ローカルコピーのデータベース',
  },
  'settings.runtimeConfig': {
    en: 'Config file',
    ko: '설정 파일',
    ja: '設定ファイル',
  },
  'settings.runtimeCounts': {
    en: 'Rows',
    ko: '행 수',
    ja: '行',
  },
  'settings.runtimeIssues': {
    en: '{n} issues',
    ko: '이슈 {n}건',
    ja: '課題 {n}件',
  },
  'settings.runtimeComments': {
    en: '{n} comments',
    ko: '코멘트 {n}개',
    ja: 'コメント {n}件',
  },
  'settings.runtimeSchema': {
    en: 'Schema version',
    ko: '스키마 버전',
    ja: 'スキーマバージョン',
  },
  'settings.runtimeWatermark': {
    en: 'Watermark',
    ko: '워터마크',
    ja: 'ウォーターマーク',
  },
  'settings.runtimeFullSync': {
    en: 'Last full sync',
    ko: '마지막 전체 동기화',
    ja: '最後の全同期',
  },
  'settings.runtimeLastError': {
    en: 'Last sync error',
    ko: '마지막 동기화 오류',
    ja: '最後の同期エラー',
  },
  'settings.runtimeVersion': {
    en: 'gadak version',
    ko: 'gadak 버전',
    ja: 'gadak バージョン',
  },
  'settings.runtimeApiCalls': {
    en: 'Jira calls',
    ko: 'Jira 호출',
    ja: 'Jira 呼び出し',
  },
  'settings.runtimeApiToday': {
    en: '{n} today',
    ko: '오늘 {n}회',
    ja: '今日 {n}回',
  },
  'settings.runtimeApiWeek': {
    en: '{n} in 7 days',
    ko: '7일간 {n}회',
    ja: '7日間で {n}回',
  },
  'settings.runtimeApiThrottled': {
    en: '{n} throttled',
    ko: '{n}회 제한됨',
    ja: '{n}回スロットル',
  },
  'settings.runtimeModified': {
    en: 'Modified',
    ko: '수정 시각',
    ja: '変更',
  },
  'settings.copy': {
    en: 'Copy',
    ko: '복사',
    ja: 'コピー',
  },
  'settings.copied': {
    en: 'Copied',
    ko: '복사됨',
    ja: 'コピーしました',
  },
  'settings.copySqlite': {
    en: 'Copy sqlite3 command',
    ko: 'sqlite3 명령 복사',
    ja: 'sqlite3 コマンドをコピー',
  },
  'settings.copySqliteDesktop': {
    en: 'Copy sqlite3 command to paste in a terminal',
    ko: '터미널에 붙여넣을 sqlite3 명령 복사',
    ja: 'ターミナルに貼る sqlite3 コマンドをコピー',
  },
  'settings.copySqliteLabelDesktop': {
    en: 'sqlite3 (paste in a terminal)',
    ko: 'sqlite3 (터미널에 붙여넣기)',
    ja: 'sqlite3（ターミナルに貼り付け）',
  },
  'settings.none': {
    en: '—',
    ko: '—',
    ja: '—',
  },
  'settings.personalToken': {
    en: 'Personal Jira API token settings →',
    ko: '개인 Jira API 토큰 설정 →',
    ja: '個人の Jira API token 設定 →',
  },
  'settings.credsElsewhere': {
    en: 'Credentials are managed in a separate dialog, not here.',
    ko: '자격증명은 이 화면이 아니라 별도 다이얼로그에서 관리합니다.',
    ja: '資格情報はこの画面ではなく、別のダイアログで管理します。',
  },
  'settings.featureFeed': {
    en: 'Personal feed',
    ko: '개인 피드',
    ja: '個人フィード',
  },
  'settings.featureFeedDesc': {
    en: 'Activity feed of mentions, watches, and assignee changes',
    ko: '멘션·워치·담당자 변경을 모은 활동 피드',
    ja: 'メンション、ウォッチ、担当者変更のアクティビティフィード',
  },
  'settings.featureFeedDescDesktop': {
    en: "Turns on the list's feed panel. System notifications (menu bar) are separate.",
    ko: '목록의 피드 패널을 켭니다. 시스템 알림(메뉴바)은 별개입니다.',
    ja: '一覧のフィードパネルをオンにします。システム通知（メニューバー）は別です。',
  },
  'settings.browserNotify': {
    en: 'Browser tab notifications',
    ko: '브라우저 탭 알림',
    ja: 'ブラウザタブの通知',
  },
  'settings.browserNotifyDesc': {
    en: 'Show a system notification when new feed items arrive while gadak is open. Does not use web push.',
    ko: 'gadak가 열려 있을 때 새 피드 항목이 오면 시스템 알림을 표시합니다. 웹 푸시는 사용하지 않습니다.',
    ja: 'gadak が開いている間に新しいフィード項目が来たらシステム通知を出します。ウェブプッシュは使いません。',
  },
  'settings.browserNotifyEnable': {
    en: 'Allow notifications',
    ko: '알림 허용',
    ja: '通知を許可',
  },
  'settings.browserNotifyGranted': {
    en: 'Allowed',
    ko: '허용됨',
    ja: '許可済み',
  },
  'settings.browserNotifyDenied': {
    en: 'Blocked in browser settings',
    ko: '브라우저 설정에서 차단됨',
    ja: 'ブラウザ設定でブロックされています',
  },
  'settings.browserNotifyUnsupported': {
    en: 'Not supported in this browser',
    ko: '이 브라우저에서 지원하지 않음',
    ja: 'このブラウザでは未対応',
  },
  'settings.featureDeploy': {
    en: 'Deploy status',
    ko: '배포 상태',
    ja: 'デプロイ状況',
  },
  'settings.featureDeployDesc': {
    en: 'Per-issue deploy stage — needs an external CI/CD index',
    ko: '이슈별 배포 단계 — 외부 CI/CD 인덱스 필요',
    ja: '課題ごとのデプロイ段階 — 外部の CI/CD インデックスが必要です',
  },
  'settings.featureQa': {
    en: 'QA context',
    ko: 'QA 컨텍스트',
    ja: 'QA コンテキスト',
  },
  'settings.featureQaDesc': {
    en: 'Per-issue test runs and suites — needs an external QA tool',
    ko: '이슈별 테스트 런·스위트 — 외부 QA 도구 필요',
    ja: '課題ごとのテストランとスイート — 外部の QA ツールが必要です',
  },
  'settings.featureTeams': {
    en: 'Team grouping',
    ko: '팀 분류',
    ja: 'チームグループ',
  },
  'settings.featureTeamsDesc': {
    en: 'Group members into teams for filters and grouping — configure rules in the Teams / groups tab',
    ko: '멤버를 팀으로 묶어 필터·그룹핑 — 팀/그룹 탭에서 규칙 설정',
    ja: 'メンバーをチームにまとめてフィルターとグループ化に使います — チーム / グループタブで規則を設定',
  },
  'settings.qaDashboardUrl': {
    en: 'QA dashboard URL (optional)',
    ko: 'QA 대시보드 URL (선택)',
    ja: 'QA ダッシュボード URL（任意）',
  },
  /* Integrations tab (desktop app only) */
  'settings.integrationsIntro': {
    en: 'Where your agents reach this local copy. Each install runs the command shown — copy it to run it yourself instead.',
    ko: '에이전트가 이 로컬 사본에 닿는 경로입니다. 설치 버튼은 아래에 적힌 명령을 그대로 실행합니다 — 직접 실행하려면 복사하세요.',
    ja: 'エージェントがこのローカルコピーに届く経路です。インストールは下のコマンドを実行します — 自分で走らせるならコピーしてください。',
  },
  'settings.integrationsLoading': {
    en: 'Reading what is installed…',
    ko: '설치 상태를 읽는 중…',
    ja: 'インストール状況を読み取り中…',
  },
  // GDK-828: the route lives on the desktop app's mux — reaching this error
  // means the app side is older or broken (IntegrationsTab.load), so the
  // next action names the app, not just the retry.
  'settings.integrationsLoadFailed': {
    en: 'Could not read the integration list from the app. Try again, or check the desktop app.',
    ko: '앱에서 연동 목록을 읽지 못했습니다. 다시 시도하거나 데스크톱 앱을 확인하세요.',
    ja: 'アプリから連携一覧を読めませんでした。再試行するか、デスクトップアプリを確認してください。',
  },
  'settings.integrationsEmpty': {
    en: 'This build offers no integrations.',
    ko: '이 빌드에는 연동 항목이 없습니다.',
    ja: 'このビルドには連携がありません。',
  },
  'settings.integrationInstalled': {
    en: 'Installed',
    ko: '설치됨',
    ja: 'インストール済み',
  },
  'settings.integrationNotInstalled': {
    en: 'Not installed',
    ko: '설치되지 않음',
    ja: '未インストール',
  },
  'settings.integrationUnknown': {
    en: 'Status unknown',
    ko: '상태 불명',
    ja: '状態不明',
  },
  'settings.integrationUnknownHint': {
    en: 'Detection here is best-effort, so this may already be in place. Re-check, or just run the command — installing again is safe.',
    ko: '이 항목의 감지는 확정이 아니라 이미 설치돼 있을 수도 있습니다. 다시 확인하거나, 그냥 명령을 실행하세요 — 다시 설치해도 안전합니다.',
    ja: 'ここでの検出は最善努力なので、すでに入っていることもあります。再確認するか、コマンドを実行してください — 再インストールは安全です。',
  },
  'settings.integrationResultUnknown': {
    en: 'Result unknown',
    ko: '결과 불명',
    ja: '結果不明',
  },
  'settings.integrationChecking': {
    en: 'Checking…',
    ko: '확인 중…',
    ja: '確認中…',
  },
  'settings.integrationRunning': {
    en: 'Installing…',
    ko: '설치 중…',
    ja: 'インストール中…',
  },
  'settings.integrationFailed': {
    en: 'Setup failed',
    ko: '설치 실패',
    ja: 'セットアップ失敗',
  },
  'settings.integrationInstall': {
    en: 'Install',
    ko: '설치',
    ja: 'インストール',
  },
  'settings.integrationUpdate': {
    en: 'Update',
    ko: '업데이트',
    ja: '更新',
  },
  'settings.integrationRecheck': {
    en: 'Re-check',
    ko: '다시 확인',
    ja: '再確認',
  },
  'settings.integrationCopyCommand': {
    en: 'Copy command',
    ko: '명령 복사',
    ja: 'コマンドをコピー',
  },
  'settings.integrationOutput': {
    en: 'Command output',
    ko: '명령 출력',
    ja: 'コマンド出力',
  },
  'settings.integrationExitCode': {
    en: 'The command exited with code {code}.',
    ko: '명령이 종료 코드 {code}으로 끝났습니다.',
    ja: 'コマンドは終了コード {code} で終わりました。',
  },
  'settings.integrationBusy': {
    en: 'An install is already running. Its output is not shown here.',
    ko: '이미 설치가 실행 중입니다. 그 출력은 여기에 표시되지 않습니다.',
    ja: 'インストールがすでに実行中です。その出力はここには出ません。',
  },
  'settings.integrationUnknownId': {
    en: 'This app does not know that integration.',
    ko: '이 앱이 모르는 연동입니다.',
    ja: 'このアプリはその連携を知りません。',
  },
  'settings.integrationStartFailed': {
    en: 'Could not start the install. Try again.',
    ko: '설치를 시작하지 못했습니다. 다시 시도하세요.',
    ja: 'インストールを開始できませんでした。再試行してください。',
  },
  'settings.integrationNoExit': {
    en: 'The output stopped before the command reported a status, so whether it worked is unknown. Re-check to ask again.',
    ko: '명령이 상태를 알리기 전에 출력이 끊겼습니다. 성공했는지 알 수 없으니 다시 확인해 주세요.',
    ja: 'コマンドが状態を報告する前に出力が止まったため、成功したかは不明です。再確認してもう一度聞いてください。',
  },
  'settings.integrationOkUndetected': {
    en: 'The command reported success, but this still is not detected. It may need the target app restarted — Re-check after that. The output above is what ran.',
    ko: '명령은 성공했다고 보고했지만 아직 감지되지 않습니다. 대상 앱을 재시작한 뒤 다시 확인해 보세요. 위 출력이 실제로 실행된 내용입니다.',
    ja: 'コマンドは成功と報告しましたが、まだ検出されていません。対象アプリの再起動が必要なことがあります — そのあと再確認してください。上の出力が実際に走った内容です。',
  },
  'settings.integrationPrereq': {
    en: 'Something has to be set up before this can be installed.',
    ko: '설치하기 전에 먼저 준비해야 할 것이 있습니다.',
    ja: 'インストールの前に用意が必要なものがあります。',
  },

  /* ── Devices (desktop only, GDK-1047) ── */
  'settings.tabDevices': {
    en: 'Devices',
    ko: '기기',
    ja: 'デバイス',
  },
  'settings.devicesIntro': {
    en: 'Pair a phone or another gadak with this workspace: mint a pairing offer, let the device scan its QR, and revoke any device here once it is gone.',
    ko: '휴대폰이나 다른 gadak를 이 워크스페이스에 연결합니다: 페어링 오퍼를 발급해 기기가 QR을 스캔하게 하고, 떠난 기기는 여기서 해지하세요.',
    ja: 'スマホや別のgadakをこのワークスペースに組み込みます：ペアリングオファーを発行して端末にQRをスキャンさせ、いなくなった端末はここで失効させます。',
  },
  'settings.devicesLoadFailed': {
    en: 'Could not read the device list from the app. Try again, or check the desktop app.',
    ko: '앱에서 기기 목록을 읽지 못했습니다. 다시 시도하거나 데스크톱 앱을 확인하세요.',
    ja: 'アプリから端末一覧を読めませんでした。再試行するか、デスクトップアプリを確認してください。',
  },
  'settings.devicesLoading': {
    en: 'Loading devices…',
    ko: '기기를 불러오는 중…',
    ja: '端末を読み込み中…',
  },
  'settings.devicesEmpty': {
    en: 'No paired devices yet. Pair one below.',
    ko: '아직 연결된 기기가 없습니다. 아래에서 연결하세요.',
    ja: 'まだ組み込んだ端末がありません。下から組み込んでください。',
  },
  'settings.devicesUnavailableNotConfigured': {
    en: 'This workspace has no tracker yet — set it up before pairing devices.',
    ko: '이 워크스페이스에는 아직 트래커가 없습니다 — 기기를 연결하기 전에 먼저 설정하세요.',
    ja: 'このワークスペースにはまだトラッカーがありません — 端末を組み込む前に設定してください。',
  },
  'settings.devicesUnavailablePairedAway': {
    en: 'This workspace is itself paired with another machine. Devices are minted on the home machine.',
    ko: '이 워크스페이스는 다른 머신에 연결되어 있습니다. 기기 발급은 홈 머신에서 합니다.',
    ja: 'このワークスペース自体が別のマシンに組み込まれています。端末の発行はホームマシンで行います。',
  },
  'settings.devicesColLabel': {
    en: 'Device',
    ko: '기기',
    ja: '端末',
  },
  'settings.devicesColScope': {
    en: 'Scope',
    ko: '권한',
    ja: 'スコープ',
  },
  'settings.devicesColExpires': {
    en: 'Expires',
    ko: '만료',
    ja: '有効期限',
  },
  'settings.devicesColState': {
    en: 'State',
    ko: '상태',
    ja: '状態',
  },
  'settings.devicesScopeServe': {
    en: 'Local copy API (phone app)',
    ko: '로컬 사본 API (휴대폰 앱)',
    ja: 'ローカルコピーAPI（スマホアプリ）',
  },
  'settings.devicesScopeOrigin': {
    en: 'Tracker passthrough (another gadak)',
    ko: '트래커 패스스루 (다른 gadak)',
    ja: 'トラッカーパススルー（別のgadak）',
  },
  'settings.devicesScopeLocalRouting': {
    en: 'This machine (routing key)',
    ko: '이 머신 (라우팅 키)',
    ja: 'このマシン（ルーティングキー）',
  },
  'settings.devicesLabelLabel': {
    en: 'Device name',
    ko: '기기 이름',
    ja: '端末名',
  },
  'settings.devicesScopeLabel': {
    en: 'What the token opens',
    ko: '토큰이 여는 것',
    ja: 'トークンが開くもの',
  },
  'settings.devicesEndpointLabel': {
    en: 'Endpoint',
    ko: '엔드포인트',
    ja: 'エンドポイント',
  },
  'settings.devicesEndpointHint': {
    en: 'Empty uses the live serve address of this machine.',
    ko: '비워 두면 이 머신의 실행 중인 serve 주소를 사용합니다.',
    ja: '空欄にすると、このマシンの稼働中のserveアドレスを使います。',
  },
  'settings.devicesTtlLabel': {
    en: 'Lifetime',
    ko: '수명',
    ja: '有効期間',
  },
  'settings.devicesMint': {
    en: 'Pair device',
    ko: '기기 연결',
    ja: '端末を組み込む',
  },
  'settings.devicesMintBusy': {
    en: 'Minting…',
    ko: '발급 중…',
    ja: '発行中…',
  },
  'settings.devicesMinted': {
    en: 'Paired {label}. The offer is reusable until {expires} — scan it with the phone now.',
    ko: '{label} 연결됨. 오퍼는 {expires}까지 재사용 가능합니다 — 지금 휴대폰으로 스캔하세요.',
    ja: '{label} を組み込みました。オファーは {expires} まで再利用できます — 今すぐスマホでスキャンしてください。',
  },
  'settings.devicesLoopbackWarning': {
    en: 'This endpoint is loopback — only a device on this machine can reach it. Use the tailnet URL instead (e.g. https://<machine>.<tailnet>.ts.net).',
    ko: '이 엔드포인트는 루프백입니다 — 이 머신의 기기만 접근할 수 있습니다. 테일넷 URL(예: https://<machine>.<tailnet>.ts.net)을 사용하세요.',
    ja: 'このエンドポイントはループバックです — このマシンの端末しか届きません。TailnetのURL（例: https://<machine>.<tailnet>.ts.net）を使ってください。',
  },
  'settings.devicesOfferLabel': {
    en: 'Pairing offer',
    ko: '페어링 오퍼',
    ja: 'ペアリングオファー',
  },
  'settings.devicesOfferShow': {
    en: 'Show',
    ko: '표시',
    ja: '表示',
  },
  'settings.devicesOfferHide': {
    en: 'Hide',
    ko: '숨기기',
    ja: '隠す',
  },
  'settings.devicesCopyOffer': {
    en: 'Copy offer',
    ko: '오퍼 복사',
    ja: 'オファーをコピー',
  },
  'settings.devicesQrAlt': {
    en: 'Pairing QR code',
    ko: '페어링 QR 코드',
    ja: 'ペアリングQRコード',
  },
  'settings.devicesErrLabelRequired': {
    en: 'Name the device first.',
    ko: '먼저 기기 이름을 입력하세요.',
    ja: 'まず端末名を入力してください。',
  },
  'settings.devicesErrReservedLabel': {
    en: 'The name _home is the routing key of this machine — pick another name.',
    ko: '_home는 이 머신의 라우팅 키입니다 — 다른 이름을 쓰세요.',
    ja: '_home はこのマシンのルーティングキーです — 別の名前にしてください。',
  },
  'settings.devicesErrBadScope': {
    en: 'That scope cannot be minted here.',
    ko: '이곳에서 발급할 수 없는 권한입니다.',
    ja: 'ここでは発行できないスコープです。',
  },
  'settings.devicesErrBadEndpoint': {
    en: 'The endpoint must be an http(s) URL.',
    ko: '엔드포인트는 http(s) URL이어야 합니다.',
    ja: 'エンドポイントはhttp(s) URLである必要があります。',
  },
  'settings.devicesErrBadTtl': {
    en: 'Lifetime must look like 90d, 12h, 30m, or 45s.',
    ko: '수명은 90d, 12h, 30m, 45s 형식이어야 합니다.',
    ja: '有効期間は 90d、12h、30m、45s の形である必要があります。',
  },
  'settings.devicesErrNoServe': {
    en: 'No live serve found for this workspace — start one, or fill in the endpoint.',
    ko: '이 워크스페이스의 실행 중인 serve를 찾지 못했습니다 — 하나 띄우거나 엔드포인트를 입력하세요.',
    ja: 'このワークスペースの稼働中のserveが見つかりません — 起動するか、エンドポイントを入力してください。',
  },
  'settings.devicesErrLabelExists': {
    en: 'An active token named {label} already exists — revoke it first, or pick another name.',
    ko: '이름이 {label}인 활성 토큰이 이미 있습니다 — 먼저 해지하거나 다른 이름을 쓰세요.',
    ja: '{label} という名前の有効なトークンがすでにあります — 先に失効させるか、別の名前にしてください。',
  },
  'settings.devicesErrFailed': {
    en: 'The mint failed.',
    ko: '발급에 실패했습니다.',
    ja: '発行に失敗しました。',
  },
  'settings.devicesRevoke': {
    en: 'Revoke',
    ko: '해지',
    ja: '失効',
  },
  'settings.devicesRevoked': {
    en: 'Revoked {label}.',
    ko: '{label} 해지됨.',
    ja: '{label} を失効させました。',
  },
  'settings.devicesHomeRowHint': {
    en: 'The routing key of this machine. Rotate it from the terminal: gadak pairing mint --label _home',
    ko: '이 머신의 라우팅 키입니다. 터미널에서 교체하세요: gadak pairing mint --label _home',
    ja: 'このマシンのルーティングキーです。ターミナルから交代してください: gadak pairing mint --label _home',
  },
  'settings.devicesStateActive': {
    en: 'Active',
    ko: '활성',
    ja: 'アクティブ',
  },
  'settings.devicesStateExpired': {
    en: 'Expired',
    ko: '만료됨',
    ja: '期限切れ',
  },
  'settings.devicesStateRevoked': {
    en: 'Revoked',
    ko: '해지됨',
    ja: '失効',
  },

  /* ── Workspaces (GDK-1096) ── */
  'settings.tabWorkspaces': {
    en: 'Workspaces',
    ko: '워크스페이스',
    ja: 'ワークスペース',
  },
  'settings.workspacesIntro': {
    en: 'Each workspace is its own local copy, served at /w/<name>/. Creating one seeds a built-in tracker; removing one deletes this machine’s copy of it.',
    ko: '각 워크스페이스는 자체 로컬 사본을 가지며 /w/<name>/ 으로 서빙됩니다. 만들면 내장 트래커가 시드되고, 지우면 이 머신의 사본이 삭제됩니다.',
    ja: '各ワークスペースは独自のローカルコピーを持ち、/w/<name>/ で serve されます。作成すると内蔵トラッカーがシードされ、削除するとこのマシンのコピーが消えます。',
  },
  'settings.workspacesLoading': {
    en: 'Loading workspaces…',
    ko: '워크스페이스를 불러오는 중…',
    ja: 'ワークスペースを読み込み中…',
  },
  'settings.workspacesLoadFailed': {
    en: 'Could not read the workspace list.',
    ko: '워크스페이스 목록을 읽지 못했습니다.',
    ja: 'ワークスペース一覧を読めませんでした。',
  },
  'settings.workspacesEmpty': {
    en: 'No workspaces besides this one yet. Create one below.',
    ko: '아직 이 워크스페이스 외에는 없습니다. 아래에서 만드세요.',
    ja: 'このワークスペース以外はまだありません。下から作成してください。',
  },
  'settings.workspacesColName': {
    en: 'Workspace',
    ko: '워크스페이스',
    ja: 'ワークスペース',
  },
  'settings.workspacesColSite': {
    en: 'Site',
    ko: '사이트',
    ja: 'サイト',
  },
  'settings.workspacesColProjects': {
    en: 'Projects',
    ko: '프로젝트',
    ja: 'プロジェクト',
  },
  'settings.workspacesActiveBadge': {
    en: 'This serve',
    ko: '현재 서빙',
    ja: 'このサーブ',
  },
  'settings.workspacesUnreadable': {
    en: 'config unreadable',
    ko: '설정을 읽을 수 없음',
    ja: '設定を読み取れません',
  },
  'settings.workspacesActiveHint': {
    en: 'This is the workspace this serve is running on. Stop serve first, then remove it from a terminal.',
    ko: '현재 serve가 이 워크스페이스 위에서 돌고 있습니다. serve를 멈춘 뒤 터미널에서 삭제하세요.',
    ja: 'このserveはこのワークスペース上で動いています。先にserveを止め、端末から削除してください。',
  },
  'settings.workspacesNameLabel': {
    en: 'Workspace name',
    ko: '워크스페이스 이름',
    ja: 'ワークスペース名',
  },
  'settings.workspacesProjectsLabel': {
    en: 'Project keys (optional, comma-separated)',
    ko: '프로젝트 키 (선택, 콤마 구분)',
    ja: 'プロジェクトキー（任意・カンマ区切り）',
  },
  'settings.workspacesCreate': {
    en: 'Create workspace',
    ko: '워크스페이스 생성',
    ja: 'ワークスペースを作成',
  },
  'settings.workspacesModeLocalOrigin': {
    en: 'New, built-in tracker',
    ko: '새 워크스페이스 (내장 트래커)',
    ja: '新規（内蔵トラッカー）',
  },
  'settings.workspacesModePaired': {
    en: 'Register remote',
    ko: '원격 등록',
    ja: 'リモート登録',
  },
  'settings.workspacesOfferLabel': {
    en: 'Pairing code',
    ko: '페어링 코드',
    ja: 'ペアリングコード',
  },
  'settings.workspacesPairHint': {
    en: 'Paste the one-line code `gadak pairing mint` printed on the home machine.',
    ko: '홈 머신에서 `gadak pairing mint`가 출력한 한 줄 코드를 붙여넣으세요.',
    ja: 'ホームマシンで `gadak pairing mint` が出力した一行コードを貼り付けてください。',
  },
  'settings.workspacesPair': {
    en: 'Register workspace',
    ko: '워크스페이스 등록',
    ja: 'ワークスペースを登録',
  },
  'settings.workspacesPairing': {
    en: 'Registering…',
    ko: '등록 중…',
    ja: '登録中…',
  },
  'settings.workspacesPairFailed': {
    en: 'Could not register the workspace. Try again.',
    ko: '워크스페이스를 등록하지 못했습니다. 다시 시도하세요.',
    ja: 'ワークスペースを登録できませんでした。もう一度お試しください。',
  },
  'settings.workspacesCreateFailed': {
    en: 'Could not create the workspace. Try again.',
    ko: '워크스페이스를 만들지 못했습니다. 다시 시도하세요.',
    ja: 'ワークスペースを作成できませんでした。もう一度お試しください。',
  },
  'settings.workspacesErrExists': {
    en: 'A workspace named {name} already exists.',
    ko: '{name}(이)라는 워크스페이스가 이미 있습니다.',
    ja: '{name} というワークスペースは既に存在します。',
  },
  'settings.workspacesErrInvalidName': {
    en: 'That name cannot be used for a workspace.',
    ko: '그 이름은 워크스페이스 이름으로 쓸 수 없습니다.',
    ja: 'その名前はワークスペース名に使えません。',
  },
  'settings.workspacesRemote': {
    en: 'Workspace management is available from a local browser only.',
    ko: '워크스페이스 관리는 로컬 브라우저에서만 할 수 있습니다.',
    ja: 'ワークスペースの管理はローカルブラウザからのみ行えます。',
  },
  'settings.workspacesRemoveTitle': {
    en: 'Remove workspace',
    ko: '워크스페이스 삭제',
    ja: 'ワークスペースを削除',
  },
  'settings.workspacesDestroyLabel': {
    en: 'Also destroy the tracker data file (the only copy of this tracker)',
    ko: '트래커 데이터 파일도 함께 파기 (이 트래커의 유일한 사본)',
    ja: 'トラッカーのデータファイルも破棄する（このトラッカーの唯一のコピー）',
  },
  'settings.workspacesDestroyHint': {
    en: 'Copy the tracker data file out first if you want to keep its data.',
    ko: '데이터를 남기려면 먼저 트래커 데이터 파일을 복사해 두세요.',
    ja: 'データを残す場合は、先にトラッカーのデータファイルをコピーしてください。',
  },
  'settings.workspacesRemoveFailed': {
    en: 'Could not remove the workspace. Try again.',
    ko: '워크스페이스를 삭제하지 못했습니다. 다시 시도하세요.',
    ja: 'ワークスペースを削除できませんでした。もう一度お試しください。',
  },
  'settings.workspacesAdvisories': {
    en: 'After this removal:',
    ko: '삭제 후 안내:',
    ja: '削除後の注記:',
  },
  'settings.groupLabels': {
    en: 'Group labels & colors',
    ko: '그룹 라벨·색상',
    ja: 'グループのラベルと色',
  },
  'settings.groupKey': {
    en: 'Group key',
    ko: '그룹 키',
    ja: 'グループキー',
  },
  'settings.label': {
    en: 'Label',
    ko: '라벨',
    ja: 'ラベル',
  },
  'settings.color': {
    en: 'Color',
    ko: '색상',
    ja: '色',
  },
  'settings.addRow': {
    en: '+ Add row',
    ko: '+ 행 추가',
    ja: '+ 行を追加',
  },
  'settings.deleteRow': {
    en: 'Delete row',
    ko: '행 삭제',
    ja: '行を削除',
  },
  'settings.groupsEmpty': {
    en: 'No groups yet.',
    ko: '아직 그룹이 없습니다.',
    ja: 'まだグループがありません。',
  },
  'settings.groupToProduct': {
    en: 'Group → product',
    ko: '그룹 → 제품',
    ja: 'グループ → 製品',
  },
  'settings.productKey': {
    en: 'Product key',
    ko: '제품 키',
    ja: '製品キー',
  },
  'settings.productLabel': {
    en: 'Product label',
    ko: '제품 라벨',
    ja: '製品ラベル',
  },
  'settings.productsEmpty': {
    en: 'No products yet.',
    ko: '아직 제품이 없습니다.',
    ja: 'まだ製品がありません。',
  },
  'settings.groupRules': {
    en: 'Group matching rules',
    ko: '그룹 판정 규칙',
    ja: 'グループ照合ルール',
  },
  'settings.rulesEmpty': {
    en: 'No rules yet.',
    ko: '아직 규칙이 없습니다.',
    ja: 'まだルールがありません。',
  },
  'settings.rulesTopDown': {
    en: 'Top to bottom',
    ko: '위에서 아래로',
    ja: '上から下へ',
  },
  'settings.rulesFirstWins': {
    en: 'first match wins',
    ko: '첫 매치가 이깁니다',
    ja: '最初の一致が勝ちます',
  },
  'settings.rulesDetail': {
    en: '. Conditions in a row are AND; values within a list are OR; empty conditions always match.',
    ko: '. 한 행의 조건들은 AND, 각 목록 안은 OR, 빈 조건은 항상 참입니다.',
    ja: '。行内の条件は AND、リスト内の値は OR、空の条件は常に一致します。',
  },
  'settings.groupQuery': {
    en: 'Classification SQL',
    ko: '분류 SQL',
    ja: '分類 SQL',
  },
  'settings.groupQueryHint': {
    en: "Optional. One SELECT or WITH returning (issue key, group). Empty group = unclassified. NULL or a missing key falls through to the rules above, then the assignee's member group. Runs when the list is rebuilt, not on each keystroke. Site-specific logic belongs here, not in the binary.",
    ko: '선택. (이슈 키, 그룹)을 돌려주는 SELECT/WITH 하나. 빈 그룹은 미분류, NULL이거나 빠진 키는 위 규칙 → 담당자 멤버 그룹으로 넘어갑니다. 목록을 다시 만들 때만 실행되며 키 입력마다 돌지 않습니다. 사이트 고유 로직은 바이너리가 아니라 여기에 둡니다.',
    ja: '任意。 (課題キー, グループ) を返す SELECT または WITH ひとつ。空のグループは未分類。NULL や欠けたキーは上の規則、その次に担当者のメンバーグループへ落ちます。一覧を再構築するときに走り、キー入力のたびに走りません。サイト固有のロジックはバイナリではなくここに置きます。',
  },
  'settings.projectsCol': {
    en: 'Projects',
    ko: '프로젝트',
    ja: 'プロジェクト',
  },
  'settings.componentsCol': {
    en: 'Components',
    ko: '컴포넌트',
    ja: 'コンポーネント',
  },
  'settings.memberEmail': {
    en: 'Email',
    ko: '이메일',
    ja: 'メール',
  },
  'settings.memberName': {
    en: 'Name',
    ko: '이름',
    ja: '名前',
  },
  'settings.memberAccountId': {
    en: 'Jira accountId',
    ko: 'Jira accountId',
    ja: 'Jira accountId',
  },
  'settings.displayName': {
    en: 'Display name',
    ko: '표시 이름',
    ja: '表示名',
  },
  'settings.department': {
    en: 'Department',
    ko: '부서',
    ja: '部署',
  },
  'settings.jobTitle': {
    en: 'Title',
    ko: '직무',
    ja: '役職',
  },
  'settings.avatarUrl': {
    en: 'Avatar URL',
    ko: '아바타 URL',
    ja: 'アバター URL',
  },
  'settings.addMember': {
    en: '+ Add member',
    ko: '+ 멤버 추가',
    ja: '+ メンバーを追加',
  },
  'settings.membersEmpty': {
    en: 'No members yet — add one to seed the workspace.',
    ko: '아직 멤버가 없습니다 — 워크스페이스에 첫 멤버를 추가하세요.',
    ja: 'まだメンバーがいません — 最初のメンバーを追加してください。',
  },
  'settings.discoveredFields': {
    en: 'Discovered fields',
    ko: '발견된 필드',
    ja: '検出されたフィールド',
  },
  'settings.discoveredFieldsHint': {
    en: 'Auto-detected from your Jira on the first full sync. Edits here are pinned and survive re-discovery; `gadak fields --apply` re-runs detection.',
    ko: '첫 전체 동기화에서 Jira로부터 자동 탐지됩니다. 여기서 수정하면 고정되어 재탐지에도 유지되고, `gadak fields --apply`로 재탐지할 수 있습니다.',
    ja: '最初の全同期で Jira から自動検出されます。ここの編集はピンされ、再検出後も残ります。`gadak fields --apply` で検出を再実行します。',
  },
  'settings.noDiscoveredFields': {
    en: 'Nothing discovered yet — run a full sync first.',
    ko: '아직 발견된 필드가 없습니다 — 전체 동기화를 먼저 실행하세요.',
    ja: 'まだ検出がありません — 先に全同期を実行してください。',
  },
  'settings.pinned': {
    en: 'pinned',
    ko: '고정됨',
    ja: 'ピン済み',
  },
  'settings.roleFacet': {
    en: 'chips',
    ko: '칩',
    ja: 'チップ',
  },
  'settings.roleBody': {
    en: 'document',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'settings.roleUser': {
    en: 'person',
    ko: '사람',
    ja: '人',
  },
  'settings.rolePlain': {
    en: 'text',
    ko: '텍스트',
    ja: 'テキスト',
  },
  'settings.kindNone': {
    en: 'read-only',
    ko: '읽기 전용',
    ja: '読み取り専用',
  },
  'settings.removeField': {
    en: 'remove',
    ko: '제거',
    ja: '削除',
  },
  'settings.adfSearchFields': {
    en: 'ADF custom field ids to include in body search (comma-separated)',
    ko: '본문 검색에 합칠 ADF 커스텀필드 id (콤마 구분)',
    ja: '本文検索に含める ADF カスタムフィールド id（カンマ区切り）',
  },
  'settings.advancedJson': {
    en: 'Advanced — edit JSON',
    ko: '고급 — JSON 직접 편집',
    ja: '上級 — JSON を編集',
  },
  'settings.jsonHint': {
    en: 'Last edit wins between form and JSON. Expanding refills from the current form.',
    ko: '폼과 JSON은 마지막 수정이 이깁니다. 펼칠 때마다 현재 폼 값으로 다시 채워집니다.',
    ja: 'フォームと JSON は最後の編集が勝ちます。展開するたびに現在のフォームから埋め直します。',
  },
  'settings.locale': {
    en: 'Language',
    ko: '언어',
    ja: '言語',
  },
  'settings.localeEn': {
    en: 'English',
    ko: 'English',
    ja: 'English',
  },
  'settings.localeKo': {
    en: '한국어',
    ko: '한국어',
    ja: '한국어',
  },
  'settings.localeJa': {
    en: '日本語',
    ko: '日本語',
    ja: '日本語',
  },
  'settings.localOriginCommandHint': {
    en: '<name> is the workspace name you choose.',
    ko: '<name>은 직접 정하는 워크스페이스 이름입니다.',
    ja: '<name> は自分で決めるワークスペース名です。',
  },
  /* ── Theme (per-browser; settings + palette) ── */
  'theme.label': {
    en: 'Theme',
    ko: '테마',
    ja: 'テーマ',
  },
  'theme.system': {
    en: 'System',
    ko: '시스템',
    ja: 'システム',
  },
  'theme.light': {
    en: 'Light',
    ko: '라이트',
    ja: 'ライト',
  },
  'theme.dark': {
    en: 'Dark',
    ko: '다크',
    ja: 'ダーク',
  },
  'theme.ink': {
    en: 'Ink',
    ko: '잉크',
    ja: 'インク',
  },
  'theme.ember': {
    en: 'Ember',
    ko: '엠버',
    ja: 'エンバー',
  },
  'theme.savedLocally': {
    en: 'Saved on this device only. The server did not take the theme.',
    ko: '이 기기에만 저장됨. 서버에 테마를 쓰지 못했습니다.',
    ja: 'このデバイスにだけ保存されました。サーバーはテーマを受け取りませんでした。',
  },
  /* ── Command palette ── */
  'palette.title': {
    en: 'Command palette',
    ko: '커맨드 팔레트',
    ja: 'コマンドパレット',
  },
  'palette.placeholder': {
    en: 'Jump to an issue, or search everything…',
    ko: '이슈로 이동, 또는 전체 검색…',
    ja: '課題へジャンプ、またはすべてを検索…',
  },
  'palette.sectionUnified': {
    en: 'All search',
    ko: '전체 검색',
    ja: '全体検索',
  },
  'palette.matchBody': {
    en: 'Body match',
    ko: '본문 매치',
    ja: '本文一致',
  },
  'palette.matchComment': {
    en: 'Comment match',
    ko: '코멘트 매치',
    ja: 'コメント一致',
  },
  'palette.matchTitle': {
    en: 'Title match',
    ko: '제목 매치',
    ja: 'タイトル一致',
  },
  'palette.seeMore': {
    en: 'See all results in the list',
    ko: '목록에서 전체 결과 보기',
    ja: '一覧ですべての結果を見る',
  },
  'palette.entryLabel': {
    en: 'Search everything',
    ko: '전체 검색',
    ja: 'すべてを検索',
  },
  'palette.entryTitle': {
    en: 'Search everything ({shortcut})',
    ko: '전체 검색 ({shortcut})',
    ja: 'すべてを検索 ({shortcut})',
  },
  // Sub line of the session row the palette puts under a matched issue
  // (GDK-1196). The label is the issue key; this says what the row does.
  'palette.openShell': {
    en: 'Open this issue’s shell',
    ko: '이 이슈의 셸 열기',
    ja: 'この課題のシェルを開く',
  },
  'palette.sectionIssues': {
    en: 'Issues',
    ko: '이슈',
    ja: '課題',
  },
  // Truncation, spelled out. "4 / 7" is reserved for the document screens'
  // filter fraction; the palette says "4 of 7" so the glyph keeps one meaning.
  'palette.docCount': {
    en: '{shown} of {total}',
    ko: '{total}개 중 {shown}개',
    ja: '{total}件中 {shown}件',
  },
  'palette.sectionDocs': {
    en: 'Documents',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'palette.sectionPeople': {
    en: 'People',
    ko: '사람',
    ja: '人',
  },
  'palette.sectionViews': {
    en: 'Views',
    ko: '저장된 뷰',
    ja: 'ビュー',
  },
  'palette.sectionActions': {
    en: 'Actions',
    ko: '액션',
    ja: 'アクション',
  },
  'palette.recent': {
    en: 'Recently viewed',
    ko: '최근 본 항목',
    ja: '最近閲覧',
  },
  'palette.updated': {
    en: 'Recently updated',
    ko: '최근 갱신',
    ja: '最近の更新',
  },
  'palette.empty': {
    en: 'No matches',
    ko: '일치하는 항목 없음',
    ja: '一致なし',
  },
  'palette.hintNav': {
    en: '↑↓ move · ↵ run · Esc close',
    ko: '↑↓ 이동 · ↵ 실행 · Esc 닫기',
    ja: '↑↓ 移動 · ↵ 実行 · Esc 閉じる',
  },
  'palette.hintHelp': {
    en: 'shortcuts',
    ko: '단축키',
    ja: 'ショートカット',
  },
  'palette.viewBuiltin': {
    en: 'Built-in view',
    ko: '기본 뷰',
    ja: '組み込みビュー',
  },
  // GDK-437: one kind of saved view, so one label. The palette used to split
  // "My view" from "Team view" by which store the row came from, and the
  // second half of that was never true — a server view is yours too.
  'palette.viewSaved': {
    en: 'Saved view',
    ko: '저장 뷰',
    ja: '保存ビュー',
  },
  'palette.viewSource': {
    en: 'Jira filter',
    ko: 'Jira 필터',
    ja: 'Jira フィルター',
  },
  // What a saved view opens, from its own config (GDK-191). One filter is the
  // common case for a saved view, so these two carry a singular of their own
  // rather than shipping "1 filters" on a row people read every day. Projects
  // only ever collapse to a count at three or more.
  'palette.viewProjects': {
    en: '{n} projects',
    ko: '프로젝트 {n}개',
    ja: '{n}件のプロジェクト',
  },
  'palette.viewKeyOne': {
    en: '1 key',
    ko: '키 1개',
    ja: 'キー 1件',
  },
  'palette.viewKeys': {
    en: '{n} keys',
    ko: '키 {n}개',
    ja: '{n}件のキー',
  },
  'palette.viewFilterOne': {
    en: '1 filter',
    ko: '필터 1개',
    ja: 'フィルター 1件',
  },
  'palette.viewFilters': {
    en: '{n} filters',
    ko: '필터 {n}개',
    ja: '{n}件のフィルター',
  },
  'palette.actionCreateIssue': {
    en: 'Create "{summary}"',
    ko: '"{summary}" 이슈 만들기',
    ja: '「{summary}」を作成',
  },
  'palette.actionSettings': {
    en: 'Open settings',
    ko: '설정 열기',
    ja: '設定を開く',
  },
  'palette.actionHistory': {
    en: 'Open history',
    ko: '히스토리 열기',
    ja: '履歴を開く',
  },
  'palette.actionDocs': {
    en: 'Open documents',
    ko: '문서 열기',
    ja: 'ドキュメントを開く',
  },
  // The terminal is a panel, not a column view, so this reads as a toggle
  // rather than "Open …" the way the view rows above it do.
  'palette.actionTerminal': {
    en: 'Terminal',
    ko: '터미널',
    ja: 'ターミナル',
  },
  'palette.actionFeed': {
    en: 'Open feed',
    ko: '피드 열기',
    ja: 'フィードを開く',
  },
  'palette.actionFavorite': {
    en: 'Favorite · {key}',
    ko: '즐겨찾기 · {key}',
    ja: 'お気に入り · {key}',
  },
  'palette.actionUnfavorite': {
    en: 'Unfavorite · {key}',
    ko: '즐겨찾기 해제 · {key}',
    ja: 'お気に入りを解除 · {key}',
  },
  'palette.actionWatch': {
    en: 'Watch · {key}',
    ko: '워치 · {key}',
    ja: 'ウォッチ · {key}',
  },
  'palette.actionUnwatch': {
    en: 'Unwatch · {key}',
    ko: '워치 해제 · {key}',
    ja: 'ウォッチを解除 · {key}',
  },
  'palette.actionToggleReopened': {
    en: 'Toggle reopened filter',
    ko: '재오픈 필터 토글',
    ja: '再オープンフィルターを切り替え',
  },
  'palette.actionToggleUnassigned': {
    en: 'Toggle unassigned filter',
    ko: '미할당 필터 토글',
    ja: '未割り当てフィルターを切り替え',
  },
  'palette.actionToggleStale': {
    en: 'Toggle stale filter',
    ko: '정체 필터 토글',
    ja: '滞留フィルターを切り替え',
  },
  'palette.actionLocale': {
    en: 'Switch language to {lang}',
    ko: '언어를 {lang}로 전환',
    ja: '言語を {lang} に切り替え',
  },
  'palette.actionTheme': {
    en: 'Switch theme to {mode}',
    ko: '테마를 {mode}로 전환',
    ja: 'テーマを {mode} に切り替え',
  },
  'palette.actionSyncStatus': {
    en: 'Show sync status',
    ko: '동기화 상태 보기',
    ja: '同期状態を表示',
  },
  'palette.actionSyncNow': {
    en: 'Sync now',
    ko: '지금 동기화',
    ja: '今すぐ同期',
  },
  'palette.syncToast': {
    en: '{overall} · synced {when}',
    ko: '{overall} · 동기화 {when}',
    ja: '{overall} · 同期 {when}',
  },
  'palette.triageSelected': {
    en: '{n} selected',
    ko: '{n}건 선택',
    ja: '{n}件選択',
  },
  'palette.actionTriageStatus': {
    en: 'Change status · {target}',
    ko: '상태 변경 · {target}',
    ja: 'ステータスを変更 · {target}',
  },
  'palette.actionTriageAssignee': {
    en: 'Change assignee · {target}',
    ko: '담당자 변경 · {target}',
    ja: '担当者を変更 · {target}',
  },
  'palette.actionTriageLabels': {
    en: 'Change labels · {target}',
    ko: '라벨 변경 · {target}',
    ja: 'ラベルを変更 · {target}',
  },
  'palette.actionTriageComment': {
    en: 'Comment on {key}',
    ko: '{key}에 코멘트',
    ja: '{key} にコメント',
  },
  'palette.actionTriageSelect': {
    en: 'Select {key}',
    ko: '{key} 선택',
    ja: '{key} を選択',
  },
  'palette.actionTriageDeselect': {
    en: 'Deselect {key}',
    ko: '{key} 선택 해제',
    ja: '{key} の選択を解除',
  },
  'palette.actionTriageClear': {
    en: 'Clear selection ({n})',
    ko: '선택 해제 ({n})',
    ja: '選択をクリア ({n})',
  },
  /* ── Sync now (shared) ── */
  'sync.starting': {
    en: 'Starting sync…',
    ko: '동기화 시작 중…',
    ja: '同期を開始しています…',
  },
  'sync.done': {
    en: 'Sync finished · fetched {n} · changed {changed}',
    ko: '동기화 완료 · 가져옴 {n} · 변경 {changed}',
    ja: '同期完了 · 取得 {n}件 · 変更 {changed}件',
  },
  'sync.failed': {
    en: 'Sync failed: {message}',
    ko: '동기화 실패: {message}',
    ja: '同期失敗: {message}',
  },
  // CLI frozenSyncError (cmd/gadak/sync.go): cause plus how to unfreeze.
  'sync.frozen': {
    en: 'This workspace is frozen — nothing goes to the tracker, syncs or writes. Unfreeze with `gadak config set frozen false`.',
    ko: '이 워크스페이스는 동결되어 트래커로 아무것도 나가지 않습니다 — 동기화도 쓰기도요. `gadak config set frozen false`로 해제합니다.',
    ja: 'このワークスペースは凍結されています — トラッカーへは同期も書き込みも行きません。`gadak config set frozen false` で解除します。',
  },
  // Jira landed, a second source did not. Naming it keeps a wiki permission
  // error from reading as "none of this worked".
  'sync.partial': {
    en: 'Issues synced. Documents failed: {message}',
    ko: '이슈는 동기화됨. 문서 실패: {message}',
    ja: '課題は同期しました。ドキュメントは失敗: {message}',
  },
  // One wording for a running sync, rendered by every surface that reports it.
  // The count is what separates a long pass from a stalled one.
  'sync.busy': {
    en: 'Syncing…',
    ko: '동기화 중…',
    ja: '同期中…',
  },
  'sync.busyIssues': {
    en: 'Syncing issues…',
    ko: '이슈 동기화 중…',
    ja: '課題を同期中…',
  },
  'sync.busyIssuesN': {
    en: 'Syncing issues · {n}',
    ko: '이슈 동기화 중 · {n}',
    ja: '課題を同期中 · {n}',
  },
  'sync.busyDocuments': {
    en: 'Fetching documents…',
    ko: '문서를 가져오는 중…',
    ja: 'ドキュメントを取得中…',
  },
  'sync.busyDocumentsN': {
    en: 'Fetching documents · {n}',
    ko: '문서를 가져오는 중 · {n}',
    ja: 'ドキュメントを取得中 · {n}',
  },
  // At rest: the verdict and the age together, so neither surface has to be
  // read alongside the other to mean anything.
  'sync.settledOk': {
    en: 'Synced {when}',
    ko: '{when} 동기화됨',
    ja: '{when} に同期',
  },
  'sync.settledDelayedWhen': {
    en: 'Sync delayed · {when}',
    ko: '동기화 지연 · {when}',
    ja: '同期遅延 · {when}',
  },
  'sync.settledFailedWhen': {
    en: 'Sync failed · {when}',
    ko: '동기화 실패 · {when}',
    ja: '同期失敗 · {when}',
  },
  'sync.settledFailed': {
    en: 'Sync failed',
    ko: '동기화 실패',
    ja: '同期失敗',
  },
  'sync.settledNever': {
    en: 'Never synced',
    ko: '동기화한 적 없음',
    ja: '未同期',
  },
  'sync.settledChecking': {
    en: 'Checking sync',
    ko: '동기화 확인 중',
    ja: '同期を確認中',
  },
} as const satisfies Record<string, Message>
