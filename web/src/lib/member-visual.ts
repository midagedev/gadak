import { config } from './config'
import type { Member } from './types'

/* 그룹(파트/팀) 라벨·색은 테넌트마다 다르므로 런타임 config 에서 온다. 미설정이면 부서 해시 색으로 폴백. */
const DEPARTMENT_COLORS = ['#22d3ee', '#4ade80', '#facc15', '#f472b6', '#818cf8', '#fb923c']

function hashIndex(value: string, length: number): number {
  let hash = 0
  for (let index = 0; index < value.length; index++) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0
  }
  return hash % length
}

export function memberOrgColor(member: Member | undefined): string | null {
  if (!member) return null
  const group = member.group?.toLowerCase() ?? ''
  const groupColor = config().groupColors[group]
  if (groupColor) return groupColor
  const seed = member.department?.trim()
  return seed ? DEPARTMENT_COLORS[hashIndex(seed, DEPARTMENT_COLORS.length)] : null
}

export function memberTooltip(member: Member | undefined, fallback: string): string {
  if (!member) return fallback
  const groupLabel = member.group ? config().groupLabels[member.group.toLowerCase()] : undefined
  return [...new Set([member.display_name || member.name || fallback, member.job_role, member.department, groupLabel].filter(Boolean))].join(' · ')
}
