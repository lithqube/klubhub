package einvoice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// ErrUnavailable means the generator could not be reached or is down: a
// condition to retry later, never a problem with the invoice.
var ErrUnavailable = errors.New("e-invoice generator unavailable")

// GenerationError means the generator refused the document (HTTP 4xx): our
// mapping or the data broke its schema. It is not an outage.
type GenerationError struct {
	Status  int
	Message string
	Details []string
}

func (e *GenerationError) Error() string {
	s := fmt.Sprintf("e-invoice generator rejected the document (HTTP %d): %s", e.Status, e.Message)
	if len(e.Details) > 0 {
		s += ": " + strings.Join(e.Details, "; ")
	}
	return s
}

// Generator produces an e-invoice from the sidecar JSON. name is a sidecar
// format name (Format.SidecarName / ValidationName); pdf is the visual PDF to
// embed, only for the hybrid format.
type Generator interface {
	Generate(ctx context.Context, name string, invoiceJSON []byte, pdf []byte) ([]byte, error)
}

// maxResponseBytes bounds what the sidecar may send back: an invoice PDF with
// the XML attached is a few hundred kilobytes.
const maxResponseBytes = 20 << 20

// SidecarClient talks to the e-invoice-eu REST server
// (POST /api/invoice/create/{format}). The server has no authentication and
// must only be reachable on the internal network.
type SidecarClient struct {
	base string
	hc   *http.Client
}

// NewSidecarClient returns a client for the server at baseURL. A nil client
// gets a 60-second timeout.
func NewSidecarClient(baseURL string, hc *http.Client) *SidecarClient {
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	return &SidecarClient{base: strings.TrimRight(baseURL, "/"), hc: hc}
}

// Generate implements Generator.
func (c *SidecarClient) Generate(ctx context.Context, name string, invoiceJSON []byte, pdf []byte) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	// The invoice must be a FILE part: the server answers 400 to a text field.
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="invoice"; filename="invoice.json"`)
	h.Set("Content-Type", "application/json")
	part, err := mw.CreatePart(h)
	if err != nil {
		return nil, err
	}
	_, _ = part.Write(invoiceJSON)
	if len(pdf) > 0 {
		fw, err := mw.CreateFormFile("pdf", "invoice.pdf")
		if err != nil {
			return nil, err
		}
		_, _ = fw.Write(pdf)
		_ = mw.WriteField("embedPDF", "true")
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/invoice/create/"+url.PathEscape(name), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: reading the response: %v", ErrUnavailable, err)
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("e-invoice generator response exceeds %d MB", maxResponseBytes>>20)
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return data, nil
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	default:
		return nil, generationError(resp.StatusCode, data)
	}
}

// generationError reads the server's {message, details: {errors: [...]}} body.
func generationError(status int, body []byte) *GenerationError {
	ge := &GenerationError{Status: status, Message: strings.TrimSpace(string(body))}
	var parsed struct {
		Message string `json:"message"`
		Details struct {
			Errors []struct {
				InstancePath string `json:"instancePath"`
				Message      string `json:"message"`
			} `json:"errors"`
		} `json:"details"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Message != "" {
		ge.Message = parsed.Message
		for _, e := range parsed.Details.Errors {
			ge.Details = append(ge.Details, strings.TrimSpace(e.InstancePath+" "+e.Message))
		}
	}
	return ge
}
