/*
 * Writes, onboarding, Jira credentials.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const write = {
  /* ── Write / new issue ── */
  'write.newIssue': {
    en: 'New issue',
    ko: '새 이슈',
    ja: '新しい課題',
  },
  'write.issueTitle': {
    en: 'Issue title',
    ko: '이슈 제목',
    ja: '課題のタイトル',
  },
  'write.descriptionPlain': {
    en: 'Plain text (line breaks kept)',
    ko: '평문 (줄바꿈 유지)',
    ja: 'プレーンテキスト（改行を保持）',
  },
  'write.searchPersonOptional': {
    en: 'Search name/email (optional)',
    ko: '이름/이메일 검색 (선택)',
    ja: '名前/メールで検索（任意）',
  },
  'write.addLabelOptional': {
    en: 'Add labels (optional)',
    ko: '라벨 추가 (선택)',
    ja: 'ラベルを追加（任意）',
  },
  'write.requiredFields': {
    en: 'Project, type, and title are required.',
    ko: '프로젝트·유형·제목은 필수입니다.',
    ja: 'プロジェクト、タイプ、タイトルは必須です。',
  },
  'write.createRequiresMore': {
    en: 'Also required here: {names}. This dialog cannot set them, so Jira may reject the issue.',
    ko: '이 프로젝트·유형은 {names}도 요구합니다. 이 다이얼로그로는 설정할 수 없어 Jira가 거절할 수 있습니다.',
    ja: 'ここでも必須です: {names}。このダイアログでは設定できないため、Jira が課題を拒否することがあります。',
  },
  'write.projectRequired': {
    en: 'Pick a project.',
    ko: '프로젝트를 고르세요.',
    ja: 'プロジェクトを選んでください。',
  },
  'write.issueTypeRequired': {
    en: 'Pick an issue type.',
    ko: '유형을 고르세요.',
    ja: '課題タイプを選んでください。',
  },
  'write.priorityRequired': {
    en: 'Pick a priority.',
    ko: '우선순위를 고르세요.',
    ja: '優先度を選んでください。',
  },
  'write.createFailed': {
    en: 'Could not create issue. Try again.',
    ko: '이슈 생성에 실패했습니다. 다시 시도하세요.',
    ja: '課題を作成できませんでした。再試行してください。',
  },
  'write.metaFailed': {
    en: 'Could not load create metadata.',
    ko: '생성 메타를 불러오지 못했습니다.',
    ja: '作成メタデータを読み込めませんでした。',
  },
  'write.issueCreated': {
    en: 'Created {key}.',
    ko: '{key} 이슈를 생성했습니다.',
    ja: '{key} を作成しました。',
  },
  'write.issueCreatedFilled': {
    en: 'Created {key} · {type} · {project}',
    ko: '{key} · {type} · {project} 이슈를 만들었습니다.',
    ja: '{key} · {type} · {project} を作成しました',
  },
  'write.needToken': {
    en: 'Set your personal Jira credentials first.',
    ko: '먼저 개인 Jira 자격증명을 설정하세요.',
    ja: '先に個人の Jira 資格情報を設定してください。',
  },
  'write.tokenRejected': {
    en: 'Your Jira API token was rejected — replace it with a new personal token.',
    ko: 'Jira API 토큰이 거부되었습니다 — 새 개인 토큰으로 교체하세요.',
    ja: 'Jira API token が拒否されました — 新しい個人トークンに差し替えてください。',
  },
  // write.go fail() / failJira codes — sentences verified against those call sites.
  'write.jiraUnavailable': {
    en: 'Could not reach Jira. Check the connection and try again.',
    ko: 'Jira에 연결하지 못했습니다. 연결을 확인한 뒤 다시 시도하세요.',
    ja: 'Jira に到達できませんでした。接続を確認して再試行してください。',
  },
  'write.workspaceBusy': {
    en: 'Another process is using this workspace. Write through its serve, or close it and retry.',
    ko: '다른 프로세스가 이 워크스페이스를 사용 중입니다. 그 serve를 통해 쓰거나, 닫은 뒤 다시 시도하세요.',
    ja: '別のプロセスがこのワークスペースを使っています。その serve 経由で書くか、閉じて再試行してください。',
  },
  'write.mirrorStale': {
    en: 'The change was saved in Jira, but the local copy could not be refreshed. Do not retry.',
    ko: '변경은 Jira에 저장됐지만 로컬 사본을 갱신하지 못했습니다. 다시 시도하지 마세요.',
    ja: '変更は Jira に保存されましたが、ローカルコピーを更新できませんでした。再試行しないでください。',
  },
  'write.notFound': {
    en: 'That issue was not found.',
    ko: '이슈를 찾을 수 없습니다.',
    ja: 'その課題は見つかりませんでした。',
  },
  'write.summaryTooLong': {
    en: 'Title cannot be longer than 255 characters.',
    ko: '제목은 255자를 넘을 수 없습니다.',
    ja: 'タイトルは 255 文字を超えられません。',
  },
  'write.projectNotMirrored': {
    en: 'That project is not in this mirror.',
    ko: '이 미러에 없는 프로젝트입니다.',
    ja: 'そのプロジェクトはこのミラーにありません。',
  },
  'write.fieldNotEditable': {
    en: 'That field cannot be edited.',
    ko: '그 필드는 편집할 수 없습니다.',
    ja: 'そのフィールドは編集できません。',
  },
  'write.siteRequired': {
    en: 'Set the Jira site in settings first.',
    ko: '먼저 설정에서 Jira 사이트를 지정하세요.',
    ja: '先に設定で Jira サイトを指定してください。',
  },
  'write.transitionFailed': {
    en: 'Could not transition status. Try again.',
    ko: '상태 전환에 실패했습니다. 다시 시도하세요.',
    ja: 'ステータスをトランジションできませんでした。再試行してください。',
  },
  'write.assignFailed': {
    en: 'Could not change assignee. Try again.',
    ko: '담당자 변경에 실패했습니다. 다시 시도하세요.',
    ja: '担当者を変更できませんでした。再試行してください。',
  },
  'write.priorityFailed': {
    en: 'Could not change priority.',
    ko: '우선순위 변경에 실패했습니다.',
    ja: '優先度を変更できませんでした。',
  },
  'write.prioritiesFailed': {
    en: 'Could not load priorities.',
    ko: '우선순위를 불러오지 못했습니다.',
    ja: '優先度を読み込めませんでした。',
  },
  'write.changePriority': {
    en: 'Change priority',
    ko: '우선순위 변경',
    ja: '優先度を変更',
  },
  'write.noPriorities': {
    en: 'No priorities on this site.',
    ko: '이 사이트에 우선순위가 없습니다.',
    ja: 'このサイトに優先度はありません。',
  },
  'write.summaryFailed': {
    en: 'Could not rename issue.',
    ko: '제목 변경에 실패했습니다.',
    ja: '課題のタイトルを変更できませんでした。',
  },
  'write.editTitle': {
    en: 'Edit title',
    ko: '제목 수정',
    ja: 'タイトルを編集',
  },
  'write.editDescription': {
    en: 'Edit description',
    ko: '설명 수정',
    ja: '説明を編集',
  },
  'write.descriptionFailed': {
    en: 'Could not update description.',
    ko: '설명 변경에 실패했습니다.',
    ja: '説明を更新できませんでした。',
  },
  'write.descriptionFormatWarn': {
    en: 'Saving will remove formatting and embeds.',
    ko: '저장하면 서식·임베드가 제거됩니다.',
    ja: '保存すると書式と埋め込みが取り除かれます。',
  },
  'write.saveAsPlain': {
    en: 'Save as plain text',
    ko: '평문으로 저장',
    ja: 'プレーンテキストとして保存',
  },
  'write.titleRequired': {
    en: 'Title cannot be empty.',
    ko: '제목은 비울 수 없습니다.',
    ja: 'タイトルを空にはできません。',
  },
  'write.labelsFailed': {
    en: 'Could not update labels.',
    ko: '라벨 변경에 실패했습니다.',
    ja: 'ラベルを更新できませんでした。',
  },
  'write.addLabel': {
    en: 'Add a label',
    ko: '라벨 추가',
    ja: 'ラベルを追加',
  },
  'write.addLabelNamed': {
    en: 'Add “{label}”',
    ko: '‘{label}’ 추가',
    ja: '「{label}」を追加',
  },
  'write.removeLabel': {
    en: 'Remove {label}',
    ko: '{label} 제거',
    ja: '{label} を削除',
  },
  'write.editMetaFailed': {
    en: 'Could not load editable fields.',
    ko: '편집 항목을 불러오지 못했습니다.',
    ja: '編集可能なフィールドを読み込めませんでした。',
  },
  'write.fieldFailed': {
    en: 'Could not update field.',
    ko: '필드 변경에 실패했습니다.',
    ja: 'フィールドを更新できませんでした。',
  },
  'write.commentFailed': {
    en: 'Could not post comment. Try again.',
    ko: '코멘트 등록에 실패했습니다. 다시 시도하세요.',
    ja: 'コメントを投稿できませんでした。再試行してください。',
  },
  'write.commentPosted': {
    en: 'Posted comment on {key}.',
    ko: '{key}에 코멘트를 등록했습니다.',
    ja: '{key} にコメントを投稿しました。',
  },
  'write.attachFailed': {
    en: 'Attachment upload failed: {name}',
    ko: '첨부 업로드 실패: {name}',
    ja: '添付のアップロードに失敗しました: {name}',
  },
  'write.credSaved': {
    en: 'Jira credentials saved.',
    ko: 'Jira 자격증명을 저장했습니다.',
    ja: 'Jira 資格情報を保存しました。',
  },
  'write.credSaveFailed': {
    en: 'Could not save credentials.',
    ko: '자격증명 저장에 실패했습니다.',
    ja: '資格情報を保存できませんでした。',
  },
  'write.credDeleted': {
    en: 'Jira credentials deleted.',
    ko: 'Jira 자격증명을 삭제했습니다.',
    ja: 'Jira 資格情報を削除しました。',
  },
  'write.credDeleteFailed': {
    en: 'Could not delete credentials.',
    ko: '자격증명 삭제에 실패했습니다.',
    ja: '資格情報を削除できませんでした。',
  },
  'write.commentPlaceholder': {
    en: 'Add a comment…',
    ko: '코멘트 추가…',
    ja: 'コメントを追加…',
  },
  'write.commentShortcut': {
    en: '{mod} ↵',
    ko: '{mod} ↵',
    ja: '{mod} ↵',
  },
  'write.commentNeedCredentials': {
    en: 'Set credentials to leave a comment',
    ko: '코멘트를 남기려면 자격증명을 설정하세요',
    ja: 'コメントするには資格情報を設定してください',
  },
  'write.commentPosting': {
    en: 'Posting…',
    ko: '등록 중…',
    ja: '投稿中…',
  },
  'write.commentButton': {
    en: 'Comment',
    ko: '코멘트',
    ja: 'コメント',
  },
  'write.removeAttachment': {
    en: 'Remove attachment',
    ko: '첨부 제거',
    ja: '添付を削除',
  },
  'write.attachFile': {
    en: 'Attach file',
    ko: '파일 첨부',
    ja: 'ファイルを添付',
  },
  'write.attachLabel': {
    en: 'Attach',
    ko: '첨부',
    ja: '添付',
  },
  'write.uploading': {
    en: 'Uploading… ({n})',
    ko: '업로드 중… ({n})',
    ja: 'アップロード中… ({n})',
  },
  'write.changeStatus': {
    en: 'Change status',
    ko: '상태 변경',
    ja: 'ステータスを変更',
  },
  'write.noTransitions': {
    en: 'No transitions available.',
    ko: '가능한 전환이 없습니다.',
    ja: '利用できるトランジションはありません。',
  },
  'write.transitionsFailed': {
    en: 'Could not load transitions.',
    ko: '전환 목록을 불러오지 못했습니다.',
    ja: 'トランジションを読み込めませんでした。',
  },
  'write.assignToMe': {
    en: 'Assign to me',
    ko: '나에게 할당',
    ja: '自分に割り当て',
  },
  'write.changeAssignee': {
    en: 'Change assignee',
    ko: '담당자 변경',
    ja: '担当者を変更',
  },
  'write.pickAssignee': {
    en: 'Choose assignee',
    ko: '담당자 선택',
    ja: '担当者を選ぶ',
  },
  'write.searchNameEmail': {
    en: 'Search name or email',
    ko: '이름 또는 이메일 검색',
    ja: '名前またはメールで検索',
  },
  'write.typeToSearch': {
    en: 'Type a name to search',
    ko: '이름을 입력해 검색하세요',
    ja: '名前を入力して検索',
  },
  'write.userNotFound': {
    en: 'No Jira user found.',
    ko: 'Jira 사용자를 찾지 못했습니다.',
    ja: 'Jira ユーザーが見つかりませんでした。',
  },
  'write.assignSpecifyFailed': {
    en: 'Could not set assignee.',
    ko: '담당자 지정에 실패했습니다.',
    ja: '担当者を設定できませんでした。',
  },
  'write.assigneeLabel': {
    en: 'Assignee',
    ko: '담당자',
    ja: '担当者',
  },
  /* ── Jira key settings ── */
  'jiraSettings.title': {
    en: 'Jira credentials',
    ko: 'Jira 자격증명 설정',
    ja: 'Jira 資格情報',
  },
  'jiraSettings.heading': {
    en: 'Personal Jira API token',
    ko: '개인 Jira API 토큰',
    ja: '個人の Jira API token',
  },
  'jiraSettings.intro1': {
    en: 'Status transitions, comments, and creating issues run as',
    ko: '상태 전환·코멘트·이슈 생성은',
    ja: 'ステータスのトランジション、コメント、課題の作成は',
  },
  'jiraSettings.intro2': {
    en: 'your Jira account',
    ko: '본인 Jira 계정',
    ja: 'あなたの Jira アカウント',
  },
  'jiraSettings.intro3': {
    en: '. Issue an Atlassian',
    ko: '으로 수행됩니다. Atlassian',
    ja: 'として実行されます。Atlassian の',
  },
  'jiraSettings.intro4': {
    en: 'API token',
    ko: 'API 토큰',
    ja: 'API token',
  },
  // Leading space on purpose: this follows the link element directly. Korean
  // attaches a particle instead and must not have one.
  'jiraSettings.intro5': {
    en: ' and register it here.',
    ko: '을 발급해 등록하세요.',
    ja: 'を発行して、ここに登録してください。',
  },
  'jiraSettings.email': {
    en: 'Jira email',
    ko: 'Jira 이메일',
    ja: 'Jira メール',
  },
  'jiraSettings.tokenReplace': {
    en: '(only when replacing)',
    ko: '(교체 시에만 입력)',
    ja: '(差し替えるときだけ)',
  },
  'jiraSettings.replaceToken': {
    en: 'Replace token',
    ko: '토큰 교체',
    ja: 'トークンを差し替え',
  },
  'jiraSettings.connected': {
    en: 'Connected',
    ko: '연결됨',
    ja: '接続済み',
  },
  'jiraSettings.tokenDots': {
    en: 'Token {hint}',
    ko: '토큰 {hint}',
    ja: 'トークン {hint}',
  },
  'jiraSettings.deleteConfirm': {
    en: 'Click again to delete',
    ko: '한 번 더 누르면 삭제됩니다',
    ja: 'もう一度クリックすると削除します',
  },
  'jiraSettings.verified': {
    en: 'Verified {when}',
    ko: '검증 {when}',
    ja: '検証 {when}',
  },
  'jiraSettings.tokenExpires': {
    en: "Expires (from Atlassian's create dialog)",
    ko: '만료일 (Atlassian 발급 화면의 날짜)',
    ja: '有効期限（Atlassian の作成ダイアログの日付）',
  },
  'jiraSettings.tokenExpiresHint': {
    en: 'Optional. Leave blank to assume the default one-year lifetime.',
    ko: '선택. 비우면 기본 수명(1년)으로 가정합니다.',
    ja: '任意です。空欄なら既定の1年寿命とみなします。',
  },

  /* ── Onboarding (first run) ── */
  'onboarding.title': {
    en: 'Set up your mirror',
    ko: '미러 설정하기',
    ja: 'ミラーをセットアップ',
  },
  'onboarding.intro': {
    en: 'Setup happens here, no terminal needed. The last step is optional.',
    ko: '설정은 여기서 끝납니다. 터미널이 필요 없고, 마지막 단계는 선택입니다.',
    ja: 'セットアップはここで完了します。ターミナルは不要で、最後のステップは任意です。',
  },
  'onboarding.stepOf': {
    en: 'Step {n} of 4',
    ko: '{n}/4 단계',
    ja: '{n} / 4 ステップ',
  },
  'onboarding.stepCredential': {
    en: 'Connect',
    ko: '연결',
    ja: '接続',
  },
  'onboarding.stepProjects': {
    en: 'Projects',
    ko: '프로젝트',
    ja: 'プロジェクト',
  },
  'onboarding.stepSync': {
    en: 'First sync',
    ko: '첫 동기화',
    ja: '最初の同期',
  },
  'onboarding.site': {
    en: 'Jira site',
    ko: 'Jira 사이트',
    ja: 'Jira サイト',
  },
  'onboarding.sitePlaceholder': {
    en: 'https://your-team.atlassian.net',
    ko: 'https://your-team.atlassian.net',
    ja: 'https://your-team.atlassian.net',
  },
  'onboarding.email': {
    en: 'Jira account email',
    ko: 'Jira 계정 이메일',
    ja: 'Jira アカウントのメール',
  },
  'onboarding.token': {
    en: 'API token',
    ko: 'API 토큰',
    ja: 'API token',
  },
  // Atlassian's token page offers three things that look like one, and two of
  // them 401 here: a *scoped* token (which its page recommends first) and an
  // org key from admin.atlassian.com. Naming all three next to the link that
  // leads there is the only place the failure can still be prevented — after
  // the 401 there is nothing left to do but explain it (GDK-98).
  // tools/doc-checks.sh pins this against the `gadak init` prompt, which says
  // the same thing to the same person on the other surface.
  'onboarding.tokenHint': {
    en: 'Stored locally in ~/.gadak/config.json and sent only to your site. Use "Create API token" with no scopes — a user token (ATATT…). A scoped token, or an org key from admin.atlassian.com (ATCTT…), cannot sign in to a site URL.',
    ko: '~/.gadak/config.json에만 저장되고, 당신의 사이트로만 전송됩니다. "Create API token"으로 스코프 없이 만든 사용자 토큰(ATATT…)이 필요합니다. 스코프 토큰이나 admin.atlassian.com의 조직 키(ATCTT…)는 사이트 URL로 로그인할 수 없습니다.',
    ja: '~/.gadak/config.json にローカル保存され、あなたのサイトにだけ送られます。「Create API token」でスコープなしのユーザートークン（ATATT…）を使ってください。スコープ付きトークンや admin.atlassian.com の組織キー（ATCTT…）ではサイト URL にサインインできません。',
  },
  'onboarding.tokenExpires': {
    en: "Expires (from Atlassian's create dialog)",
    ko: '만료일 (Atlassian 발급 화면의 날짜)',
    ja: '有効期限（Atlassian の作成ダイアログの日付）',
  },
  'onboarding.tokenExpiresHint': {
    en: 'Optional. Leave blank to assume the default one-year lifetime.',
    ko: '선택. 비우면 기본 수명(1년)으로 가정합니다.',
    ja: '任意です。空欄なら既定の1年寿命とみなします。',
  },
  'onboarding.errExpires': {
    en: 'That expiry date is not a date (YYYY-MM-DD).',
    ko: '만료일이 날짜가 아닙니다 (YYYY-MM-DD).',
    ja: 'その有効期限は日付ではありません（YYYY-MM-DD）。',
  },
  'onboarding.tokenLink': {
    en: 'Create an API token',
    ko: 'API 토큰 만들기',
    ja: 'API token を作成',
  },
  'onboarding.connect': {
    en: 'Connect',
    ko: '연결',
    ja: '接続',
  },
  'onboarding.connectedAs': {
    en: 'Connected as {name}',
    ko: '{name}으로 연결됨',
    ja: '{name} として接続',
  },
  'onboarding.errRejected': {
    en: 'Jira rejected that email and token. Check both and try again.',
    ko: 'Jira가 이메일/토큰을 거부했습니다. 둘 다 확인해 주세요.',
    ja: 'Jira がそのメールとトークンを拒否しました。両方を確認して再試行してください。',
  },
  // Only fires now when the pasted token actually carries the ATCTT prefix
  // (internal/server/onboarding.go rejectedCredentialCode), so this sentence
  // states a fact about the token in the box rather than a possibility.
  'onboarding.errRejectedOrgKey': {
    en: 'Org API keys (ATCTT from admin.atlassian.com) do not work — create a user API token (ATATT) instead.',
    ko: '조직 API 키(admin.atlassian.com의 ATCTT)는 동작하지 않습니다 — 사용자 API 토큰(ATATT)을 만드세요.',
    ja: '組織 API キー（admin.atlassian.com の ATCTT）は使えません — 代わりにユーザー API token（ATATT）を作成してください。',
  },
  // The other rejections are indistinguishable at the server: a scoped token
  // and a mistyped one both come back as a bare 401. So this sentence hands the
  // user the check they can run themselves — which list their token is in.
  'onboarding.errRejectedScoped': {
    en: 'Scoped tokens ("Create API token with scopes") are issued for Atlassian\'s cloud API, not for your site URL — open id.atlassian.com and, if the token you pasted is a scoped one, create one without scopes.',
    ko: '스코프 토큰("Create API token with scopes")은 Atlassian 클라우드 API 용이라 사이트 URL에는 쓸 수 없습니다 — id.atlassian.com에서 확인하고, 붙여넣은 토큰이 스코프 토큰이면 스코프 없는 토큰을 새로 만드세요.',
    ja: 'スコープ付きトークン（「Create API token with scopes」）は Atlassian のクラウド API 向けで、サイト URL には使えません — id.atlassian.com を開き、貼ったトークンがスコープ付きなら、スコープなしで作り直してください。',
  },
  'onboarding.errSite': {
    en: 'Enter your Jira site URL, for example https://your-team.atlassian.net.',
    ko: 'Jira 사이트 URL을 입력하세요. 예: https://your-team.atlassian.net',
    ja: 'Jira サイト URL を入力してください。例: https://your-team.atlassian.net',
  },
  'onboarding.errFields': {
    en: 'Email and API token are both required.',
    ko: '이메일과 API 토큰이 모두 필요합니다.',
    ja: 'メールと API token の両方が必要です。',
  },
  'onboarding.errConnect': {
    en: 'Could not reach Jira: {message}',
    ko: 'Jira에 연결할 수 없습니다: {message}',
    ja: 'Jira に到達できませんでした: {message}',
  },
  // Naming the empty case here: leaving the list alone is the choice that
  // keeps working as the site grows, so it must not read like an unfinished
  // form (GDK-99).
  'onboarding.projectsIntro': {
    en: 'Pick the projects to mirror, or pick none to mirror every project you can see — including ones created later. You can change this in settings.',
    ko: '미러할 프로젝트를 고르세요. 하나도 고르지 않으면 볼 수 있는 모든 프로젝트를 미러합니다 — 나중에 만들어지는 것까지. 설정에서 바꿀 수 있습니다.',
    ja: 'ミラーするプロジェクトを選ぶか、何も選ばずに見られるすべてのプロジェクトをミラーします — あとから作られるものも含みます。設定で変更できます。',
  },
  'onboarding.loadingProjects': {
    en: 'Loading projects…',
    ko: '프로젝트 불러오는 중…',
    ja: 'プロジェクトを読み込み中…',
  },
  'onboarding.errProjects': {
    en: 'Could not list projects: {message}',
    ko: '프로젝트 목록을 가져오지 못했습니다: {message}',
    ja: 'プロジェクトを一覧できませんでした: {message}',
  },
  'onboarding.projectsTruncated': {
    en: 'Showing the first {n} projects; add any others in settings.',
    ko: '앞쪽 {n}개만 표시했습니다. 나머지는 설정에서 추가하세요.',
    ja: '先頭 {n}件のプロジェクトを表示しています。残りは設定で追加してください。',
  },
  'onboarding.noProjects': {
    en: 'This account cannot browse any project on that site.',
    ko: '이 계정이 조회할 수 있는 프로젝트가 없습니다.',
    ja: 'このアカウントはそのサイトのプロジェクトを閲覧できません。',
  },
  'onboarding.noProjectsChecklist': {
    en: 'Check: the site URL is correct, the account has Browse Projects, and you are not using an org admin key.',
    ko: '확인: 사이트 URL이 맞고, Browse Projects 권한이 있으며, 조직 관리자 키가 아닌지.',
    ja: '確認: サイト URL が正しく、アカウントに Browse Projects があり、組織管理者キーを使っていないこと。',
  },
  'onboarding.noProjectsManual': {
    en: 'You can type project keys manually in settings.',
    ko: '설정에서 프로젝트 키를 직접 입력할 수 있습니다.',
    ja: '設定でプロジェクトキーを手入力できます。',
  },
  'onboarding.selectAll': {
    en: 'Select all',
    ko: '전체 선택',
    ja: 'すべて選択',
  },
  'onboarding.selectNone': {
    en: 'Select none',
    ko: '선택 해제',
    ja: '選択を解除',
  },
  'onboarding.selectedCount': {
    en: '{n} selected',
    ko: '{n}개 선택',
    ja: '{n}件選択',
  },
  'onboarding.startSync': {
    en: 'Start first sync',
    ko: '첫 동기화 시작',
    ja: '最初の同期を開始',
  },
  'onboarding.errSaveProjects': {
    en: 'Could not save the project list: {message}',
    ko: '프로젝트 목록을 저장하지 못했습니다: {message}',
    ja: 'プロジェクト一覧を保存できませんでした: {message}',
  },
  'onboarding.syncStarting': {
    en: 'Starting…',
    ko: '시작 중…',
    ja: '開始中…',
  },
  'onboarding.syncDone': {
    en: 'Mirrored {n} issues.',
    ko: '{n}건 미러 완료.',
    ja: '{n}件をミラーしました。',
  },
  'onboarding.syncServeHint': {
    en: 'For automatic updates later, run gadak serve (or use Sync now from the sidebar).',
    ko: '이후 자동 갱신은 gadak serve로 서버를 실행하거나 사이드바의 지금 동기화를 쓰세요.',
    ja: '以降の自動更新には gadak serve を実行するか、サイドバーの「今すぐ同期」を使ってください。',
  },
  /* Step 4 — optional. The mirror is full; this is where it gets a second reader. */
  'onboarding.stepAgent': {
    en: 'Connect an agent',
    ko: '에이전트 연결',
    ja: 'エージェントを接続',
  },
  'onboarding.stepOptional': {
    en: 'Optional',
    ko: '선택',
    ja: '任意',
  },
  'onboarding.agentIntro': {
    en: 'The mirror is filled. The last choice is what reads it.',
    ko: '미러가 채워졌습니다. 이제 이걸 무엇으로 읽을지만 고르면 됩니다.',
    ja: 'ミラーは埋まりました。最後の選択は、それを何で読むかです。',
  },
  'onboarding.agentWhy': {
    en: 'This app is one reader. The other is your coding agent — one command lets it query the mirror directly, without going back to Jira.',
    ko: '지금 보고 있는 앱이 리더 하나이고, 다른 하나는 당신의 코딩 에이전트입니다. 명령 한 줄이면 에이전트가 Jira를 다시 거치지 않고 이 미러에 직접 질의합니다.',
    ja: 'このアプリがリーダーのひとつです。もうひとつはコーディングエージェントです — コマンドひとつで、Jira に戻らずミラーへ直接問い合わせられます。',
  },
  'onboarding.agentCommandsLabel': {
    en: 'Register gadak with your agent',
    ko: '에이전트에 gadak 등록하기',
    ja: 'エージェントに gadak を登録',
  },
  'onboarding.agentCommandsHint': {
    en: 'Run it in a terminal — your agent is a CLI, so you already have one.',
    ko: '터미널에서 실행하세요 — 에이전트 자체가 CLI라 이미 하나 열려 있습니다.',
    ja: 'ターミナルで実行してください — エージェント自体が CLI なので、すでにひとつ開いています。',
  },
  'onboarding.agentSkillCaption': {
    en: 'Claude Code — installs a skill that teaches it the schema and the queries. No server; it loads only when a question needs it.',
    ko: 'Claude Code — 스키마와 쿼리 패턴을 알려 주는 스킬을 설치합니다. 서버가 필요 없고, 질문이 생길 때만 로드됩니다.',
    ja: 'Claude Code — スキーマとクエリを教えるスキルをインストールします。サーバーは不要で、質問が必要になったときだけ読み込みます。',
  },
  'onboarding.agentMcpCaption': {
    en: 'Or register an MCP server — the way in for a shell-less host (Claude Desktop). claude registers itself; cursor and codex print config to paste:',
    ko: '또는 MCP 서버로 등록 — 셸이 없는 호스트(Claude Desktop)를 위한 경로입니다. claude는 등록까지 대신 하고, cursor·codex는 붙여넣을 설정을 출력합니다:',
    ja: 'または MCP サーバーを登録 — シェルのないホスト（Claude Desktop）向けです。claude は自分で登録し、cursor と codex は貼り付ける設定を出力します:',
  },
  'onboarding.agentNoCli': {
    en: 'No gadak in your terminal? In the desktop app: Settings → Integrations.',
    ko: '터미널에 gadak가 없나요? 데스크톱 앱의 설정 → 연동에서 설치하세요.',
    ja: 'ターミナルに gadak がありませんか? デスクトップアプリでは 設定 → 連携 です。',
  },
  'onboarding.agentDocsSetup': {
    en: 'Agent setup',
    ko: '에이전트 설정',
    ja: 'エージェントのセットアップ',
  },
  'onboarding.agentDocsRecipes': {
    en: 'Query recipes',
    ko: '질의 레시피',
    ja: 'クエリのレシピ',
  },
  'onboarding.agentDone': {
    en: 'Open the app',
    ko: '앱 열기',
    ja: 'アプリを開く',
  },
  'onboarding.agentSkip': {
    en: 'Skip for now',
    ko: '나중에 하기',
    ja: '今はスキップ',
  },
  'onboarding.errSync': {
    en: 'Sync failed: {message}',
    ko: '동기화 실패: {message}',
    ja: '同期失敗: {message}',
  },
  'onboarding.back': {
    en: 'Back',
    ko: '뒤로',
    ja: '戻る',
  },
  'onboarding.switchAccount': {
    en: 'Use a different account',
    ko: '다른 계정으로 연결',
    ja: '別のアカウントを使う',
  },
  'onboarding.openSettings': {
    en: 'Open settings',
    ko: '설정 열기',
    ja: '設定を開く',
  },
  'onboarding.cliHint': {
    en: 'The same setup is available as gadak init in a terminal.',
    ko: '같은 설정을 터미널에서 gadak init으로도 할 수 있습니다.',
    ja: '同じセットアップはターミナルの gadak init でもできます。',
  },
  // GDK-247: PUT onboarding/connect/ 409 standalone_data_present. Facts match
  // cmd/gadak/init.go's ReplaceRefusedError sentence (via workspace.RefuseReplace).
  'onboarding.standaloneBlocked': {
    en: 'This workspace is standalone and holds {n} locally originated issues or documents. They exist only here — no Jira site has a copy. Converting this workspace deletes them from the mirror.',
    ko: '이 워크스페이스는 독립 워크스페이스이며 여기서만 존재하는 로컬 원본 이슈 또는 문서가 {n}개 있습니다. 어떤 Jira 사이트에도 사본이 없습니다. 이 워크스페이스를 전환하면 미러에서 그 이슈 또는 문서들이 즉시 삭제됩니다.',
    ja: 'このワークスペースはスタンドアロンで、ここでだけ存在する課題またはドキュメントが {n}件あります。どの Jira サイトにもコピーはありません。このワークスペースを変換すると、それらはミラーから削除されます。',
  },
  'onboarding.standalonePersist': {
    en: 'Origin persist file: {path}',
    ko: '원본 persist 파일: {path}',
    ja: 'origin persist ファイル: {path}',
  },
  'onboarding.standaloneOtherWorkspace': {
    en: 'Connect the site in a separate workspace: gadak --workspace <name> init (list workspaces with gadak workspaces).',
    ko: '사이트는 별도 워크스페이스에 연결하세요: gadak --workspace <name> init (워크스페이스 목록은 gadak workspaces).',
    ja: 'サイトは別のワークスペースで接続してください: gadak --workspace <name> init（一覧は gadak workspaces）。',
  },
  'onboarding.standaloneReplaceConfirm': {
    en: 'Replace this workspace anyway. Converting deletes these issues or documents from the mirror.',
    ko: '그래도 이 워크스페이스를 교체합니다. 전환하면 미러에서 이 이슈 또는 문서들이 즉시 삭제됩니다.',
    ja: 'それでもこのワークスペースを置き換えます。変換するとこれらの課題またはドキュメントはミラーから削除されます。',
  },
  'onboarding.standaloneReplace': {
    en: 'Replace and connect',
    ko: '교체하고 연결',
    ja: '置き換えて接続',
  },
} as const satisfies Record<string, Message>
