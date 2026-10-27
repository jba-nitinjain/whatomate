package whatsapp

// GraphError is the typed error returned by ParseMetaAPIError. Its Error()
// text is identical to the message the function has always produced, so
// existing callers that only log or compare the string are unaffected, while
// callers that need the HTTP status or Meta error code can use errors.As.
type GraphError struct {
	StatusCode int // HTTP status returned by the Graph API
	Code       int // Meta error code (0 when the body was not a Meta error)
	Subcode    int // Meta error_subcode
	msg        string
}

func (e *GraphError) Error() string { return e.msg }
