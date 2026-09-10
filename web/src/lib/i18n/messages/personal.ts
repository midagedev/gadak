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
    en: 'What you open appears here',
    ko: '열어본 항목이 여기에 표시됩니다',
    ja: '開いたものがここに並びます',
  },
  'personal.feedHint': {
    en: 'Changes on my issues + comments that mention me',
    ko: '내 이슈 변화 + 나를 멘션한 코멘트',
    ja: '自分の課題の変化 + 自分宛メンションのコメント',
  },
  'personal.needCredentials': {
    en: 'Set credentials to see your feed and reported issues →',
    ko: '자격증명을 설정하면 피드와 내가 보고한 이슈가 여기 모입니다 →',
    ja: '資格情報を設定すると、フィードと自分が報告した課題がここに集まります →',
  },
  'personal.demoNoIdentity': {
    // GDK-1189, same reordering: the demo has no Jira account by design.
    en: 'The demo runs without a Jira account, so there is no personal feed.',
    ko: '데모는 Jira 계정 없이 돌아갑니다. 그래서 개인 피드가 없습니다.',
    ja: 'デモは Jira アカウントなしで動きます。そのため個人フィードはありません。',
  },
  /* GDK-1122: built-in has no credential to offer a dialog for, so this
     note replaces the needCredentials CTA there. */
  'personal.builtInNoIdentity': {
    // GDK-1189: the property first, the trade after. The old order ("needs
    // an identity — runs without an account") made the thing this workspace
    // is for read as the thing it is missing; three blind reviewers of the
    // 0.19 film called it out. ko and ja carry the same order.
    en: 'This workspace runs without an account, so there is no personal feed.',
    ko: '이 워크스페이스는 계정 없이 돌아갑니다. 그래서 개인 피드가 없습니다.',
    ja: 'このワークスペースはアカウントなしで動きます。そのため個人フィードはありません。'
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
    ko: '지켜보는 중 — 상태 변경/코멘트/재오픈 알림',
    ja: 'ウォッチ中 — ステータス / コメント / 再オープンの通知オン',
  },
  'personal.watchOff': {
    en: 'Watch — status/comment/reopen alerts',
    ko: '지켜보기 — 상태 변경/코멘트/재오픈 알림',
    ja: 'ウォッチ — ステータス / コメント / 再オープンの通知',
  },
  'personal.watchNeedCredentials': {
    en: 'Set credentials to watch',
    ko: '자격증명을 설정하면 지켜볼 수 있습니다',
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
  // GDK-1590: the feed day header's total (title beside the bare number).
  'feed.dayTotal': {
    en: '{n} events',
    ko: '활동 {n}건',
    ja: '活動 {n}件',
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
    ko: '지켜보기',
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
