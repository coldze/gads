package v201710

type Ad struct {
	AdGroupId int64  `xml:"-"`
	Id        int64  `xml:"id,omitempty"`
	Status    string `xml:"-"`
}
