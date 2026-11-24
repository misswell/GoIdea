package openidea

// HTTPRequestBlock is one request parsed from an IntelliJ-style .http file.
type HTTPRequestBlock struct {
	Index     uint32  `json:"index"`
	Name      *string `json:"name"`
	Method    string  `json:"method"`
	URL       string  `json:"url"`
	Line      uint32  `json:"line"`
	StartLine uint32  `json:"startLine"`
	EndLine   uint32  `json:"endLine"`
}

// HTTPPreparedRequest is a request after variables and file includes resolve.
type HTTPPreparedRequest struct {
	Name    *string     `json:"name"`
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers [][2]string `json:"headers"`
	Body    *string     `json:"body"`
}

// HTTPResponse is the captured response of one HTTP Client exchange.
type HTTPResponse struct {
	Status      uint16      `json:"status"`
	StatusText  string      `json:"statusText"`
	Version     string      `json:"version"`
	Headers     [][2]string `json:"headers"`
	ContentType *string     `json:"contentType"`
	Body        *string     `json:"body"`
	Size        float64     `json:"size"`
	Truncated   bool        `json:"truncated"`
	TTFBMs      float64     `json:"ttfbMs"`
	TotalMs     float64     `json:"totalMs"`
}

// HTTPHistoryEntry is one response in the project's HTTP Client history.
type HTTPHistoryEntry struct {
	ID      string   `json:"id"`
	TimeMs  float64  `json:"timeMs"`
	Name    *string  `json:"name"`
	Method  string   `json:"method"`
	URL     string   `json:"url"`
	Status  *uint16  `json:"status"`
	TotalMs *float64 `json:"totalMs"`
	File    *string  `json:"file"`
	Error   *string  `json:"error"`
}

// HTTPExchange is the prepared request and response stored in history.
type HTTPExchange struct {
	Entry    HTTPHistoryEntry    `json:"entry"`
	Request  HTTPPreparedRequest `json:"request"`
	Response *HTTPResponse       `json:"response"`
}

// RouteSource is one Java or Kotlin file from OpenIDEA's Spring mapping scan.
type RouteSource struct {
	Path string `json:"path"`
	Text string `json:"text"`
}
