package v201710

import "encoding/xml"

type ConversionTrackingSettings struct {
	EffectiveConversionTrackingId      int64 `xml:effectiveConversionTrackingId`
	UsesCrossAccountConversionTracking bool  `xml:usesCrossAccountConversionTracking`
}

type ConversionTrackerService struct {
	Auth
}

type ConversionTracker struct {
	Id                           int64   `xml:"id"`
	Name                         string  `xml:"name"`
	Status                       string  `xml:"status"` // Status: "ENABLED", "PAUSED", "REMOVED"
	Category                     string  `xml:"category"`
	ViewthroughLookbackWindow    int64   `xml:"viewthroughLookbackWindow"`
	CtcLookbackWindow            int64   `xml:"ctcLookbackWindow"`
	CountingType                 string  `xml:"countingType"`
	DefaultRevenueValue          float64 `xml:"defaultRevenueValue"`
	DefaultRevenueCurrencyCode   string  `xml:"defaultRevenueCurrencyCode"`
	AlwaysUseDefaultRevenueValue bool    `xml:"alwaysUseDefaultRevenueValue"`
	ExcludeFromBidding           bool    `xml:"excludeFromBidding"`
	AttributionModelType         string  `xml:"attributionModelType"`
	ConversionTrackerType        string  `xml:"conversionTrackerType"`
	Snippet                      string  `xml:"snippet"`
	TextFormat                   string  `xml:"textFormat"`             // "UNKNOWN", "SEARCH", "DISPLAY", "SHOPPING"
	ConversionPageLanguage       string  `xml:"conversionPageLanguage"` // "UNKNOWN", "SEARCH_MOBILE_APP", "DISPLAY_MOBILE_APP", "SEARCH_EXPRESS", "DISPLAY_EXPRESS"
	BackgroundColor              string  `xml:"backgroundColor"`
	TrackingCodeType             string  `xml:"trackingCodeType"`
	XSIType                      *string `xml:"xsiType,omitempty"`
	Errors                       []error `xml:"-"`
}

func NewConversionTrackerService(auth *Auth) *ConversionTrackerService {
	return &ConversionTrackerService{Auth: *auth}
}

func (s *ConversionTrackerService) Get(selector Selector) (conversionTrackers []ConversionTracker, totalCount int64, err error) {
	// The default namespace, "", will break in 1.5 with the addition of
	// custom namespace support.  Hence, we have to ensure that the baseUrl is
	// set again as the proper namespace for the service/serviceSelector element
	selector.XMLName = xml.Name{baseUrl, "serviceSelector"}

	respBody, err := s.Auth.request(
		conversionTrackerServiceUrl,
		"get",
		struct {
			XMLName xml.Name
			Sel     Selector
		}{
			XMLName: xml.Name{
				Space: baseUrl,
				Local: "get",
			},
			Sel: selector,
		},
	)
	if err != nil {
		return conversionTrackers, totalCount, err
	}
	getResp := struct {
		Size               int64               `xml:"rval>totalNumEntries"`
		ConversionTrackers []ConversionTracker `xml:"rval>entries"`
	}{}
	err = xml.Unmarshal([]byte(respBody), &getResp)
	if err != nil {
		return conversionTrackers, totalCount, err
	}
	return getResp.ConversionTrackers, getResp.Size, err
}
