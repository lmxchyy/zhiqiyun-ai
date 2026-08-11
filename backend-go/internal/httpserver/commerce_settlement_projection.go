package httpserver

import "strings"

func shouldWriteLegacyCommissionProjection(order adminOrder) bool {
	return !strings.EqualFold(stringValue(order.PriceSnapshot["settlementEngine"]), string(settlementEngineV132))
}
