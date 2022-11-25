package root

import (
	root "bantu-monitor/root/models"
)

// GetRootInfo returns root information on this bantupayapi instance.
// notably the organization and other relevant pieces of informations for general consumption
func GetRootInfo() root.RootInfo {
	var rootDefault root.RootInfo
	rootDefault.CreatedBy = "Ric Richards (github: @richardsric7)"
	return rootDefault
}
