package investapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// post sends one multipart form and decodes the reply.
//
// Multipart because that is what the page sends: its transport builds a FormData,
// whatever the axios default header says. A form-urlencoded body was never tried —
// matching the client the server already serves is one fewer thing to be wrong
// about, and this endpoint has been wrong about enough.
func (c *Client) post(ctx context.Context, path string, fields map[string]string, out checked) error {
	body, contentType, err := multipartBody(fields)
	if err != nil {
		return fmt.Errorf("build %s body: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+path, body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", contentType)
	// Not optional. See [pagePath] for what its absence costs.
	req.Header.Set("Referer", origin+pagePath)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("post %s: %s", path, res.Status)
	}

	// Read whole rather than streamed into the decoder, so that Trace has
	// something to show when the decode is what failed.
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if c.Trace != nil {
		c.Trace(path, fields, payload)
	}

	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode %s: %w — an HTML body here means the session is "+
			"not authenticated", path, err)
	}
	return out.check(path)
}

// multipartBody encodes the fields and reports the content type that describes
// them, boundary included.
func multipartBody(fields map[string]string) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := form.WriteField(name, value); err != nil {
			return nil, "", err
		}
	}
	if err := form.Close(); err != nil {
		return nil, "", err
	}
	return &body, form.FormDataContentType(), nil
}
