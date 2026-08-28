/*
 * Filter/form fields, columns, categories, deploy, groups.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const fields = {
  /* ── Fields (filter / form labels) ── */
  // GDK-831: 분류 is the team-grouping word (settings.featureTeams); the
  // status axis is 카테고리, matching en Category / ja カテゴリ.
  'field.status_category': {
    en: 'Category',
    ko: '카테고리',
    ja: 'カテゴリ',
  },
  'field.status': {
    en: 'Status',
    ko: '상태',
    ja: 'ステータス',
  },
  // The changelog's assignee axis is the plain id "assignee" (GDK-1065);
  // assignee_email below is the filter/form axis. Wording is common.assignee's.
  'field.assignee': {
    en: 'Assignee',
    ko: '담당자',
    ja: '担当者',
  },
  'field.assignee_email': {
    en: 'Assignee',
    ko: '담당자',
    ja: '担当者',
  },
  'field.reporter_email': {
    en: 'Reporter',
    ko: '보고자',
    ja: '報告者',
  },
  'field.actor': {
    en: 'Actor',
    ko: '작업자',
    ja: '実行者',
  },
  'field.team_group': {
    en: 'Team',
    ko: '팀',
    ja: 'チーム',
  },
  'field.labels': {
    en: 'Labels',
    ko: '라벨',
    ja: 'ラベル',
  },
  'field.priority': {
    en: 'Priority',
    ko: '우선순위',
    ja: '優先度',
  },
  'field.severity': {
    en: 'Severity',
    ko: '심각도',
    ja: '重大度',
  },
  'field.issue_type': {
    en: 'Type',
    ko: '유형',
    ja: '課題タイプ',
  },
  'field.components': {
    en: 'Components',
    ko: '컴포넌트',
    ja: 'コンポーネント',
  },
  'field.fix_versions': {
    en: 'Fix versions',
    ko: '수정 버전',
    ja: '修正バージョン',
  },
  'field.environment': {
    en: 'Environment',
    ko: '발생 환경',
    ja: '環境',
  },
  'field.browser': {
    en: 'Browser',
    ko: '브라우저',
    ja: 'ブラウザ',
  },
  'field.dev_project_number': {
    en: 'Project number',
    ko: '발생 프로젝트 번호',
    ja: 'プロジェクト番号',
  },
  'field.found_version': {
    en: 'Found in version',
    ko: '발생 버전',
    ja: '発見バージョン',
  },
  'field.occurrence': {
    en: 'Frequency',
    ko: '발생 빈도',
    ja: '発生頻度',
  },
  'field.solution': {
    en: 'Solution',
    ko: '솔루션',
    ja: 'ソリューション',
  },
  'field.critical_phenomenon': {
    en: 'Critical phenomenon',
    ko: '크리티컬 현상',
    ja: '重大現象',
  },
  'field.development_area': {
    en: 'Dev area',
    ko: '개발 영역',
    ja: '開発領域',
  },
  'field.development_test_assignee_email': {
    en: 'Dev test assignee',
    ko: '개발 테스트 담당자',
    ja: '開発テスト担当者',
  },
  'field.development_test_result': {
    en: 'Dev test result',
    ko: '개발 테스트 결과',
    ja: '開発テスト結果',
  },
  'field.qa_run': {
    en: 'QA run',
    ko: 'QA 차수',
    ja: 'QA ラン',
  },
  'field.qa_suite': {
    en: 'QA suite',
    ko: 'QA 영역',
    ja: 'QA スイート',
  },
  'field.qa_impact': {
    en: 'QA impact',
    ko: 'QA 영향',
    ja: 'QA 影響',
  },
  'field.deploy_state': {
    en: 'Deploy',
    ko: '배포',
    ja: 'デプロイ',
  },
  'field.cs': {
    en: 'CS',
    ko: 'CS',
    ja: 'CS',
  },
  'field.jira_project': {
    en: 'Project',
    ko: '프로젝트',
    ja: 'プロジェクト',
  },
  'field.source_project': {
    en: 'Source project',
    ko: '복제 원본 프로젝트',
    ja: 'ソースプロジェクト',
  },
  'field.created': {
    en: 'Created',
    ko: '생성',
    ja: '作成',
  },
  'field.updated': {
    en: 'Updated',
    ko: '갱신',
    ja: '更新',
  },
  'field.due': {
    en: 'Due date',
    ko: '기한',
    ja: '期限',
  },
  'field.parent': {
    en: 'Parent',
    ko: '상위 항목',
    ja: '親',
  },
  // The changelog's machine id for parent links (GDK-1055); field.parent
  // above is the form axis over the same concept.
  'field.issueparentassociation': {
    en: 'Parent',
    ko: '상위 항목',
    ja: '親課題',
  },
  // The most frequent changelog id the feed ships (726 rows measured,
  // GDK-1055) — mapped so ko/ja readers do not get an English word.
  'field.resolution': {
    en: 'Resolution',
    ko: '해결',
    ja: '解決状況',
  },
  'field.development_opinion': {
    en: 'Dev notes',
    ko: '개발 의견',
    ja: '開発メモ',
  },
  /* ── Columns ── */
  'column.assignee': {
    en: 'Assignee',
    ko: '담당자',
    ja: '担当者',
  },
  'column.updated': {
    en: 'Updated',
    ko: '갱신 시간',
    ja: '更新',
  },
  'column.labels': {
    en: 'Labels',
    ko: '라벨',
    ja: 'ラベル',
  },
  'column.reopen': {
    en: 'Reopened',
    ko: '재오픈',
    ja: '再オープン',
  },
  'column.stale': {
    en: 'Stale (age)',
    ko: '정체(경과)',
    ja: '滞留（経過）',
  },
  'column.qa_impact': {
    en: 'QA impact',
    ko: 'QA 영향',
    ja: 'QA 影響',
  },
  'column.deploy': {
    en: 'Deploy stage',
    ko: '배포 단계',
    ja: 'デプロイ段階',
  },
  'column.severity': {
    en: 'Severity',
    ko: '심각도',
    ja: '重大度',
  },
  'column.issue_type': {
    en: 'Type',
    ko: '유형',
    ja: '課題タイプ',
  },
  'column.status': {
    en: 'Status',
    ko: '상태',
    ja: 'ステータス',
  },
  'column.reporter': {
    en: 'Reporter',
    ko: '보고자',
    ja: '報告者',
  },
  'column.comment_count': {
    en: 'Comments',
    ko: '코멘트 수',
    ja: 'コメント',
  },
  'column.fix_versions': {
    en: 'Fix versions',
    ko: '수정 버전',
    ja: '修正バージョン',
  },
  'column.components': {
    en: 'Components',
    ko: '컴포넌트',
    ja: 'コンポーネント',
  },
  'column.created': {
    en: 'Created',
    ko: '생성 시간',
    ja: '作成',
  },
  'column.due': {
    en: 'Due date',
    ko: '기한',
    ja: '期限',
  },
  'column.environment': {
    en: 'Environment',
    ko: '환경',
    ja: '環境',
  },
  'column.team_group': {
    en: 'Team',
    ko: '팀',
    ja: 'チーム',
  },
  'column.dev_test_result': {
    en: 'Dev test result',
    ko: '개발 테스트 결과',
    ja: '開発テスト結果',
  },
  /* ── Status categories ── */
  'category.new': {
    en: 'New',
    ko: '신규',
    ja: '未着手',
  },
  'category.inprogress': {
    en: 'In progress',
    ko: '진행 중',
    ja: '進行中',
  },
  'category.done': {
    en: 'Done',
    ko: '완료',
    ja: '完了',
  },
  /* ── Deploy states ── */
  'deploy.none': {
    en: 'Not in a release',
    ko: '릴리즈 미포함',
    ja: 'リリース未含',
  },
  'deploy.merged': {
    en: 'Merged',
    ko: '머지됨',
    ja: 'マージ済み',
  },
  'deploy.dev': {
    en: 'dev release',
    ko: 'dev 릴리즈',
    ja: 'dev リリース',
  },
  'deploy.qa_preview': {
    en: 'QA pending (pre-swap)',
    ko: 'QA 대기(스왑 전)',
    ja: 'QA 待ち（スワップ前）',
  },
  'deploy.qa': {
    en: 'QA ready',
    ko: 'QA 확인 가능',
    ja: 'QA 準備完了',
  },
  'deploy.prod': {
    en: 'prod deployed',
    ko: 'prod 배포',
    ja: 'prod デプロイ済み',
  },
  'deploy.notDeployed': {
    en: 'Not deployed',
    ko: '미배포',
    ja: '未デプロイ',
  },
  'deploy.stageTitle': {
    en: 'Deploy stage: {label}',
    ko: '배포 단계: {label}',
    ja: 'デプロイ段階: {label}',
  },
  'deploy.mergedNoRelease': {
    en: 'Merged · not in a release',
    ko: '머지됨 · 릴리즈 미포함',
    ja: 'マージ済み · リリース未含',
  },
  'deploy.unmerged': {
    en: 'Not merged',
    ko: '미머지',
    ja: '未マージ',
  },
  'deploy.merge': {
    en: 'Merge',
    ko: '머지',
    ja: 'マージ',
  },
  'deploy.qaRelease': {
    en: 'qa release',
    ko: 'qa 릴리즈',
    ja: 'qa リリース',
  },
  'deploy.qaSwapReady': {
    en: 'qa swap · QA ready',
    ko: 'qa 스왑 · QA 확인 가능',
    ja: 'qa スワップ · QA 準備完了',
  },
  'deploy.prMergedFrac': {
    en: '{a}/{b} PRs merged',
    ko: '{a}/{b} PR 머지',
    ja: '{a}/{b} PR マージ済み',
  },
  'deploy.prMergedCount': {
    en: '{n} PRs merged',
    ko: '{n} PR 머지',
    ja: '{n} PR マージ済み',
  },
  'deploy.includedIn': {
    en: 'In: {tag}',
    ko: '포함: {tag}',
    ja: '含有: {tag}',
  },
  'deploy.resolvedNoRelease': {
    en: 'Resolved but not yet included in any release (merged state)',
    ko: '해결됨이지만 아직 어느 릴리즈에도 포함되지 않음(머지 상태)',
    ja: '解決済みですが、まだどのリリースにも含まれていません（マージ状態）',
  },
  'deploy.qaSwapDone': {
    en: 'qa swap done — QA ready',
    ko: 'qa 스왑 완료 — QA 확인 가능',
    ja: 'qa スワップ完了 — QA 準備完了',
  },
  'deploy.byPr': {
    en: 'Inclusion by PR',
    ko: 'PR별 포함 여부',
    ja: 'PR ごとの含有',
  },
  /* ── Group / empty labels ── */
  'group.noStatus': {
    en: '(no status)',
    ko: '(상태 없음)',
    ja: '(ステータスなし)',
  },
  'group.noPriority': {
    en: 'No priority',
    ko: '우선순위 없음',
    ja: '優先度なし',
  },
  'group.noSeverity': {
    en: 'No severity',
    ko: '심각도 없음',
    ja: '重大度なし',
  },
  'group.noProduct': {
    en: 'No product',
    ko: '제품 없음',
    ja: '製品なし',
  },
  'group.noType': {
    en: 'No type',
    ko: '유형 없음',
    ja: 'タイプなし',
  },
  'group.noProject': {
    en: 'No project',
    ko: '프로젝트 없음',
    ja: 'プロジェクトなし',
  },
  'group.noEpic': {
    en: 'No epic',
    ko: '에픽 없음',
    ja: 'エピックなし',
  },
  'group.qaIrrelevant': {
    en: 'Not in current run',
    ko: '현재 차수 무관',
    ja: '今ラン対象外',
  },
  'group.none': {
    en: 'None',
    ko: '없음',
    ja: 'なし',
  },
  'group.sectionNone': {
    en: 'No sections',
    ko: '섹션 없음',
    ja: 'セクションなし',
  },
  'group.byStatusCategory': {
    en: 'Progress',
    ko: '진행 단계',
    ja: '進捗',
  },
  'group.byProduct': {
    en: 'Product',
    ko: '제품',
    ja: '製品',
  },
  'group.byTeam': {
    en: 'Team',
    ko: '팀',
    ja: 'チーム',
  },
  'group.byAssignee': {
    en: 'Assignee',
    ko: '담당자',
    ja: '担当者',
  },
  'group.byActor': {
    en: 'Actor',
    ko: '작업자',
    ja: '実行者',
  },
  'group.noActor': {
    en: 'No actors',
    ko: '작업자 없음',
    ja: '実行者なし',
  },
  'group.byPriority': {
    en: 'Priority',
    ko: '우선순위',
    ja: '優先度',
  },
  'group.bySeverity': {
    en: 'Severity',
    ko: '심각도',
    ja: '重大度',
  },
  'group.byType': {
    en: 'Type',
    ko: '유형',
    ja: 'タイプ',
  },
  'group.byDevTestResult': {
    en: 'Dev test result',
    ko: '개발 테스트 결과',
    ja: '開発テスト結果',
  },
  'group.byQaImpact': {
    en: 'QA impact',
    ko: 'QA 영향',
    ja: 'QA 影響',
  },
  'group.bySourceProject': {
    en: 'Source project',
    ko: '복제 원본',
    ja: 'ソースプロジェクト',
  },
  'group.byEpic': {
    en: 'Epic',
    ko: '에픽',
    ja: 'エピック',
  },
  'group.byStatus': {
    en: 'Jira status',
    ko: 'Jira 상태',
    ja: 'Jira ステータス',
  },
  'group.breakdown': {
    en: 'Breakdown',
    ko: '구분',
    ja: '内訳',
  },
  'group.openEpic': {
    en: 'Open epic issue',
    ko: '에픽 이슈 열기',
    ja: 'エピック課題を開く',
  },
} as const satisfies Record<string, Message>
