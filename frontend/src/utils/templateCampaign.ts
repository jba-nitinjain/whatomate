import type { Router } from 'vue-router'

interface CampaignSourceTemplate {
  id: string
  name: string
  display_name?: string
  whatsapp_account?: string
}

/**
 * Opens the Campaigns view with its "create campaign" dialog pre-filled from a
 * template (CampaignsView reads these query params on mount).
 */
export function openCampaignFromTemplate(router: Router, template: CampaignSourceTemplate) {
  const displayName = template.display_name || template.name
  return router.push({
    name: 'campaigns',
    query: {
      createFromTemplate: '1',
      templateId: template.id,
      account: template.whatsapp_account,
      campaignName: `${displayName} Campaign`,
    },
  })
}
