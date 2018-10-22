package v201806

// https://developers.google.com/adwords/api/docs/reference/v201806/AdGroupExtensionSettingService.Page
// Contains the results from a get call.
type Page struct {
	TotalNumEntries int    `xml:"https://adwords.google.com/api/adwords/cm/v201806 totalNumEntries,omitempty"`
	PageType        string `xml:"https://adwords.google.com/api/adwords/cm/v201806 Page.Type,omitempty"`
}
