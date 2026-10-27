import type { Role } from '@/services/api'

/**
 * Role pre-selected when adding a member: the organisation's default role,
 * then the system "agent" role (least privilege), then the first role.
 */
export function pickDefaultRoleId(roles: Role[]): string {
  const byDefault = roles.find(r => r.is_default)
  if (byDefault) return byDefault.id
  const agent = roles.find(r => r.is_system && r.name.toLowerCase() === 'agent')
  if (agent) return agent.id
  return roles[0]?.id ?? ''
}
