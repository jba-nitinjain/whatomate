// Helpers shared by the catalog list, detail and product dialogs.
// Prices travel to/from the API in minor units (cents, paise) because that is
// what Meta's catalog API stores; the UI always shows and edits major units.

export const CATALOG_CURRENCIES = ['INR', 'USD', 'EUR', 'GBP', 'AED', 'SAR', 'SGD', 'AUD', 'CAD', 'BRL', 'MXN', 'IDR'] as const

export const DEFAULT_CATALOG_CURRENCY = 'INR'

export function toMinorUnits(amount: number | string): number {
  const value = typeof amount === 'string' ? parseFloat(amount) : amount
  if (!Number.isFinite(value)) return 0
  return Math.round(value * 100)
}

export function fromMinorUnits(minor: number): string {
  return (minor / 100).toFixed(2)
}

export function formatCatalogPrice(minor: number, currency: string): string {
  const amount = minor / 100
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency: currency || 'USD' }).format(amount)
  } catch {
    return `${amount.toFixed(2)} ${currency}`
  }
}

export function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export interface CatalogAccountOption {
  id: string
  name: string
}

/** Extracts the account list from the `/accounts` envelope. */
export function extractAccounts(payload: any): CatalogAccountOption[] {
  const data = payload?.data ?? payload
  const list = data?.accounts ?? []
  return Array.isArray(list) ? list.map((a: any) => ({ id: String(a.id), name: String(a.name) })) : []
}
