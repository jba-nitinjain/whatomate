import { ref } from 'vue'
import { toast } from 'vue-sonner'

import { campaignsService } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'

type Translator = (key: string, params?: Record<string, unknown>) => string

interface CampaignReportTarget {
  id: string
  name: string
}

export function useCampaignReportExport(t: Translator) {
  const exportingCampaignId = ref<string | null>(null)

  async function exportCampaignReport(campaign: CampaignReportTarget) {
    exportingCampaignId.value = campaign.id
    try {
      const response = await campaignsService.exportReport(campaign.id)
      const blob = response.data instanceof Blob
        ? response.data
        : new Blob([response.data], { type: response.headers['content-type'] })
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = getCampaignReportFilename(response.headers['content-disposition'], campaign.name)
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      window.URL.revokeObjectURL(url)
      toast.success(t('importExport.exportSuccess', { count: 0 }))
    } catch (error) {
      toast.error(getErrorMessage(error, t('importExport.exportFailed')))
    } finally {
      exportingCampaignId.value = null
    }
  }

  return {
    exportingCampaignId,
    exportCampaignReport,
  }
}

function getCampaignReportFilename(contentDisposition: string | undefined, campaignName: string): string {
  const match = contentDisposition?.match(/filename="?([^"]+)"?/i)
  if (match?.[1]) {
    return match[1]
  }

  const fallbackName = campaignName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '_')
    .replace(/^_+|_+$/g, '')

  return `${fallbackName || 'campaign_report'}.xlsx`
}
