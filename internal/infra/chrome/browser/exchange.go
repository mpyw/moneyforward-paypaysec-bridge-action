package browser

// Exchange is one request and what came back.
//
// Flat and JSON, one per line, because the consumer is a person with jq and a
// question — "which call carries the number on the screen".
type Exchange struct {
	Time        string `json:"time"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	Type        string `json:"type"`
	Status      int64  `json:"status,omitempty"`
	MIME        string `json:"mime,omitempty"`
	RequestBody string `json:"requestBody,omitempty"`
	Body        string `json:"body,omitempty"`

	// Note carries what went wrong, when something did. A recorded exchange
	// with no body and no explanation is indistinguishable from an empty one.
	Note string `json:"note,omitempty"`

	// fetchPostData asks for the request body over CDP because the event did
	// not carry it. Not serialised; it is a note to this package.
	//declscope:package // the recorder's own bookkeeping rides on the exchange
	fetchPostData bool `json:"-"`
}
