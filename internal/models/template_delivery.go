package models

import "strings"

type TemplateDeliveryRoute string

const (
	TemplateDeliveryRouteCloudAPI              TemplateDeliveryRoute = "cloud_api"
	TemplateDeliveryRouteMarketingMessagesLite TemplateDeliveryRoute = "marketing_messages_lite"
)

func ResolveTemplateDeliveryRoute(account *WhatsAppAccount, template *Template) TemplateDeliveryRoute {
	if account == nil || template == nil {
		return TemplateDeliveryRouteCloudAPI
	}

	if strings.EqualFold(template.Category, "MARKETING") &&
		account.MarketingMessagesLiteOnboarded &&
		account.MarketingMessagesLiteEnabled {
		return TemplateDeliveryRouteMarketingMessagesLite
	}

	return TemplateDeliveryRouteCloudAPI
}
