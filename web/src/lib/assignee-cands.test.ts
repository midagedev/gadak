import { describe, expect, test } from 'vitest'
import {
  ASSIGNEE_TYPED_LOCAL_CAP,
  candOfMember,
  groupedAssigneeCands,
  typedAssigneeCands,
} from './assignee-cands'
import type { JiraUser, Member } from './types'

function member(over: Partial<Member> & Pick<Member, 'email' | 'name'>): Member {
  return {
    display_name: over.display_name ?? over.name,
    profile_image: null,
    department: null,
    job_role: null,
    group: over.group ?? null,
    status: over.status ?? 'ACTIVE',
    jira_account_id: over.jira_account_id ?? null,
    ...over,
  }
}

const compare = (a: string, b: string) => a.localeCompare(b)

describe('groupedAssigneeCands', () => {
  const me = member({ email: 'me@ex.test', name: 'Me', jira_account_id: 'acc-me' })
  const reporter = member({
    email: 'rep@ex.test',
    name: 'Reporter',
    jira_account_id: 'acc-rep',
  })
  const teammate = member({
    email: 'team@ex.test',
    name: 'Teammate',
    jira_account_id: 'acc-team',
    group: 'alpha',
  })
  const other = member({
    email: 'other@ex.test',
    name: 'Other',
    jira_account_id: 'acc-other',
    group: 'beta',
  })
  const resigned = member({
    email: 'gone@ex.test',
    name: 'Gone',
    jira_account_id: 'acc-gone',
    status: 'RESIGN',
  })

  test('order is me, reporter, recent, same team, rest; resigned stay out of assignable', () => {
    const groups = groupedAssigneeCands({
      members: [other, teammate, resigned, reporter, me],
      me,
      context: { reporter, teamGroup: 'alpha' },
      recentAccountIds: ['acc-other'],
      assignToMeLabel: 'Assign to me',
      compare,
    })
    expect(groups.map((g) => g.map((c) => c.account_id))).toEqual([
      ['acc-me'],
      ['acc-rep'],
      ['acc-other'],
      ['acc-team'],
    ])
    expect(groups[0][0].label).toBe('Assign to me')
    expect(groups.flat().some((c) => c.account_id === 'acc-gone')).toBe(false)
  })

  test('dedupes by account_id so me is not repeated in rest', () => {
    const groups = groupedAssigneeCands({
      members: [me],
      me,
      context: { reporter: me, teamGroup: null },
      recentAccountIds: ['acc-me'],
      assignToMeLabel: 'Assign to me',
      compare,
    })
    expect(groups).toHaveLength(1)
    expect(groups[0].map((c) => c.account_id)).toEqual(['acc-me'])
  })
})

describe('typedAssigneeCands', () => {
  const local = member({
    email: 'dana@example.com',
    name: 'Dana',
    display_name: 'Dana Whitfield',
    jira_account_id: 'acc-dana',
  })
  const noId = member({
    email: 'lag@example.com',
    name: 'Lag',
    jira_account_id: null,
  })
  const outsider: JiraUser = {
    account_id: 'acc-out',
    display_name: 'WT Outsider',
    email: 'outsider@example.test',
    avatar_url: '',
    active: true,
  }

  test('local hits come first; server users whose email is new are appended', () => {
    const cands = typedAssigneeCands({
      query: 'da',
      members: [local, noId],
      serverUsers: [outsider],
    })
    expect(cands.map((c) => [c.origin, c.account_id])).toEqual([
      ['member', 'acc-dana'],
      ['server', 'acc-out'],
    ])
  })

  test('a member without account_id still appears locally (resolve later)', () => {
    const cands = typedAssigneeCands({
      query: 'lag',
      members: [noId],
      serverUsers: [],
    })
    expect(cands).toEqual([candOfMember(noId)])
    expect(cands[0].account_id).toBeNull()
  })

  test('empty or whitespace query is no typed list', () => {
    expect(
      typedAssigneeCands({ query: '  ', members: [local], serverUsers: [outsider] }),
    ).toEqual([])
  })

  test('local cap matches the picker bound', () => {
    expect(ASSIGNEE_TYPED_LOCAL_CAP).toBe(8)
    const many = Array.from({ length: 12 }, (_, i) =>
      member({
        email: `u${i}@ex.test`,
        name: `User ${i}`,
        jira_account_id: `acc-${i}`,
      }),
    )
    const cands = typedAssigneeCands({ query: 'user', members: many, serverUsers: [] })
    expect(cands).toHaveLength(8)
  })
})
