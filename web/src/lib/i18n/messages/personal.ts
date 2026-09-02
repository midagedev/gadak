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
    en: 'Recent',
    ko: '최근',
    ja: '最近',
  },
  'personal.recentEmpty': {
    en: 'Issues and documents you open appear here',
    ko: '연 이슈와 문서가 여기에 쌓입니다',
    ja: '開いた課題やドキュメントがここに並びます',
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
  /* GDK-1122: local-origin has no credential to offer a dialog for, so this
     note replaces the needCredentials CTA there. */
  'personal.localOriginNoIdentity': {
    en: 'Personal views need an identity — this workspace runs without an account',
    ko: '개인 뷰는 신원이 필요해 계정 없이 쓰는 워크스페이스에서는 표시되지 않습니다',
    ja: '個人ビューには身元が必要で、アカウントなしで使うワークスペースでは利用できません',
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
  /* GDK-1066: the feed request failed — not "no new activity". */
  'feed.loadFailed': {
    en: 'Could not load the feed.',
    ko: '피드를 불러오지 못했습니다.',
    ja: 'フィードを読み込めませんでした。',
  },
  'feed.unreadCount': {
    en: '{n} unread',
    ko: '안 읽은 활동 {n}건',
    ja: '未読 {n}件',
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
} as const satisfies Record<string, Message>
