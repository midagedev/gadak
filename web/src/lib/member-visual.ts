import { config } from './config'
import type { Member } from './types'

/* Group (part/team) labels & colors are per-tenant via runtime config. Fallback: department hash color. */
const DEPARTMENT_COLORS = ['#3a6a72', '#4a6e50', '#8a6418', '#8a4a62', '#3d4f6f', '#9a5a32']

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
