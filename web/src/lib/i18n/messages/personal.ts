/*
 * Personal, feed, notifications.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const personal = {
  'personal.myIssues': {
    en: 'My issues',
    ko: '내 이슈',
    ja: '自分の課題',
  },

  /* ── Personal ── */
  'personal.favorites': {
    en: 'Favorites',
    ko: '즐겨찾기',
    ja: 'お気に入り',
  },
  'personal.recent': {
    en: 'Recently viewed',
    ko: '최근 본 항목',
    ja: '最近閲覧',
  },
  'personal.recentHistory': {
    en: 'Browse history',
    ko: '이전 조회 기록',
    ja: '履歴を見る',
  },
  'personal.myAssignee': {
    en: 'Assigned to me',
    ko: '내 담당',
    ja: '自分の担当',
  },
  'personal.myReporter': {
    en: 'Reported by me',
    ko: '내가 보고',
    ja: '自分が報告',
  },
  'personal.feedHint': {
    en: 'Changes on my issues + comments that mention me',
    ko: '내 이슈 변화 + 나를 멘션한 코멘트',
    ja: '自分の課題の変化 + 自分宛メンションのコメント',
  },
  'personal.needCredentials': {
    en: 'Set credentials to see assigned, reported, and mentions →',
    ko: '자격증명을 설정하면 내 담당·보고·멘션이 여기 모입니다 →',
    ja: '資格情報を設定すると、担当・報告・メンションがここに集まります →',
  },
  'personal.demoNoIdentity': {
    en: 'Personal views need a Jira identity — not available in the demo',
    ko: '개인 뷰는 Jira 신원이 필요해 데모에서는 표시되지 않습니다',
    ja: '個人ビューには Jira の身元が必要で、デモでは利用できません',
  },
  'personal.favoriteAria': {
    en: 'Favorite {key}',
    ko: '{key} 즐겨찾기',
    ja: '{key} をお気に入り',
  },
  'personal.unfavoriteAria': {
    en: 'Unfavorite {key}',
    ko: '{key} 즐겨찾기 해제',
    ja: '{key} のお気に入りを解除',
  },
  'personal.watchOn': {
    en: 'Watching — status/comment/reopen alerts on',
    ko: '워치 중 — 상태 변경/코멘트/재오픈 알림',
    ja: 'ウォッチ中 — ステータス / コメント / 再オープンの通知オン',
  },
  'personal.watchOff': {
    en: 'Watch — status/comment/reopen alerts',
    ko: '워치 — 상태 변경/코멘트/재오픈 알림',
    ja: 'ウォッチ — ステータス / コメント / 再オープンの通知',
  },
  'personal.watchNeedCredentials': {
    en: 'Set credentials to watch',
    ko: '자격증명을 설정하면 워치할 수 있습니다',
    ja: 'ウォッチするには資格情報を設定してください',
  },
  /* ── Feed ── */
  'feed.title': {
    en: 'Feed',
    ko: '피드',
    ja: 'フィード',
  },
  'feed.markAllRead': {
    en: 'Mark all read',
    ko: '모두 읽음',
    ja: 'すべて既読',
  },
  'feed.backToList': {
    en: 'Back to list',
    ko: '목록으로 돌아가기',
    ja: '一覧に戻る',
  },
  'feed.needCredentials': {
    en: 'Set your Jira credentials first',
    ko: '먼저 Jira 자격증명을 설정하세요',
    ja: '先に Jira 資格情報を設定してください',
  },
  'feed.loading': {
    en: 'Loading feed…',
    ko: '피드 불러오는 중…',
    ja: 'フィードを読み込み中…',
  },
  'feed.empty': {
    en: 'No new activity',
    ko: '새 활동이 없습니다',
    ja: '新しいアクティビティはありません',
  },
  'feed.filterAll': {
    en: 'All',
    ko: '전체',
    ja: 'すべて',
  },
  'feed.filterAssignee': {
    en: 'Assigned',
    ko: '담당',
    ja: '担当',
  },
  'feed.filterReporter': {
    en: 'Reported',
    ko: '보고',
    ja: '報告',
  },
  'feed.filterMention': {
    en: 'Mentions',
    ko: '멘션',
    ja: 'メンション',
  },
  'feed.kindCreated': {
    en: 'New issue',
    ko: '새 이슈',
    ja: '新しい課題',
  },
  'feed.kindStatus': {
    en: 'Status change',
    ko: '상태 변경',
    ja: 'ステータス変更',
  },
  'feed.kindReopen': {
    en: 'Reopened',
    ko: '재오픈',
    ja: '再オープン',
  },
  'feed.kindAssignee': {
    en: 'Assignee change',
    ko: '담당자 변경',
    ja: '担当者変更',
  },
  'feed.kindComment': {
    en: 'New comment',
    ko: '새 코멘트',
    ja: '新しいコメント',
  },
  'feed.kindAttachment': {
    en: 'New attachment',
    ko: '새 첨부',
    ja: '新しい添付',
  },
  'feed.kindField': {
    en: 'Field change',
    ko: '필드 변경',
    ja: 'フィールド変更',
  },
  'feed.whyAssignee': {
    en: 'Assigned',
    ko: '담당',
    ja: '担当',
  },
  'feed.whyNewAssignee': {
    en: 'Newly assigned',
    ko: '새 담당',
    ja: '新規担当',
  },
  'feed.whyReporter': {
    en: 'Reported',
    ko: '보고',
    ja: '報告',
  },
  'feed.whyWatch': {
    en: 'Watching',
    ko: '워치',
    ja: 'ウォッチ',
  },
  'feed.whyMention': {
    en: 'Mentioned',
    ko: '멘션',
    ja: 'メンション',
  },
  'feed.notifyCreated': {
    en: 'created',
    ko: '생성',
    ja: '作成',
  },
  'feed.notifyStatus': {
    en: 'status',
    ko: '상태',
    ja: 'ステータス',
  },
  'feed.notifyReopened': {
    en: 'reopened',
    ko: '재오픈',
    ja: '再オープン',
  },
  'feed.notifyAssigned': {
    en: 'assigned',
    ko: '담당',
    ja: '担当',
  },
  'feed.notifyComment': {
    en: 'comment',
    ko: '코멘트',
    ja: 'コメント',
  },
  'feed.notifyAttachment': {
    en: 'attachment',
    ko: '첨부',
    ja: '添付',
  },
  'feed.notifyFields': {
    en: 'fields',
    ko: '필드',
    ja: 'フィールド',
  },
  'feed.notifyTitle': {
    en: '{key} {kind} by {actor}',
    ko: '{key} {kind} · {actor}',
    ja: '{key} {kind} · {actor}',
  },
  'feed.notifyTitleNoActor': {
    en: '{key} {kind}',
    ko: '{key} {kind}',
    ja: '{key} {kind}',
  },
  /* ── Notifications ── */
  'notif.title': {
    en: 'Notification settings',
    ko: '알림 설정',
    ja: '通知設定',
  },
  'notif.webPush': {
    en: 'Web notifications',
    ko: '웹 알림',
    ja: 'ウェブ通知',
  },
  'notif.unsupported': {
    en: 'This browser does not support web notifications.',
    ko: '이 브라우저는 웹 알림을 지원하지 않습니다.',
    ja: 'このブラウザはウェブ通知に対応していません。',
  },
  'notif.serverNotReady': {
    en: 'Server notification setup is not ready.',
    ko: '서버 알림 설정이 준비되지 않았습니다.',
    ja: 'サーバーの通知設定はまだ準備できていません。',
  },
  'notif.blocked': {
    en: 'Notifications are blocked in the browser.',
    ko: '브라우저에서 알림이 차단되었습니다.',
    ja: 'ブラウザで通知がブロックされています。',
  },
  'notif.mention': {
    en: 'Mentions',
    ko: '멘션',
    ja: 'メンション',
  },
  'notif.newAssignee': {
    en: 'New assigned issues',
    ko: '새 담당 이슈',
    ja: '新規担当の課題',
  },
  'notif.watchChange': {
    en: 'Watched issue changes',
    ko: '워치 이슈 변경',
    ja: 'ウォッチ中の課題の変化',
  },
  'notif.lockScreen': {
    en: 'Show content on lock screen',
    ko: '잠금 화면에 내용 표시',
    ja: 'ロック画面に内容を表示',
  },
  'notif.quietHours': {
    en: 'Quiet hours (hold notifications in this window)',
    ko: '조용 시간 (이 시간대엔 알림 보류)',
    ja: 'サイレント時間（この時間帯は通知を保留）',
  },
  'notif.quietStart': {
    en: 'Quiet hours start',
    ko: '조용 시간 시작',
    ja: 'サイレント開始',
  },
  'notif.quietEnd': {
    en: 'Quiet hours end',
    ko: '조용 시간 종료',
    ja: 'サイレント終了',
  },
  'notif.quietHint': {
    en: 'Local time · may span midnight',
    ko: '로컬 시간 · 자정 걸침 가능',
    ja: 'ローカル時刻 · 深夜をまたげます',
  },
  'notif.enabledHere': {
    en: 'On in this browser',
    ko: '이 브라우저에서 켜짐',
    ja: 'このブラウザでオン',
  },
  'notif.enableHere': {
    en: 'Enable in this browser',
    ko: '이 브라우저에서 켜기',
    ja: 'このブラウザで有効にする',
  },

  /* ── Me / notifications errors ── */
  'me.noCryptoKey': {
    en: 'Missing subscription encryption key.',
    ko: '구독 암호화 키가 없습니다.',
    ja: '購読の暗号化キーがありません。',
  },
  'me.enableNotifFailed': {
    en: 'Could not enable notifications in this browser.',
    ko: '이 브라우저에서 알림을 켜지 못했습니다.',
    ja: 'このブラウザで通知を有効にできませんでした。',
  },
  'me.disableNotifFailed': {
    en: 'Could not turn off notifications.',
    ko: '알림을 끄지 못했습니다.',
    ja: '通知をオフにできませんでした。',
  },
} as const satisfies Record<string, Message>
