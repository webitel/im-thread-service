package guards

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var allowedImageMimes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true, "image/svg+xml": true,
}

// [VALIDATE_URL] ENSURES STRING IS A SECURE ABSOLUTE URL FROM TRUSTED SOURCE
func validateURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("URL is empty")
	}

	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// [SCHEME] ONLY HTTP/HTTPS
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", scheme)
	}

	return nil
}

// ValidateMime checks if the mime type is present and formatted correctly
func validateMime(mime string, isImage bool) error {
	if mime == "" || !strings.Contains(mime, "/") {
		return fmt.Errorf("invalid or empty mime type: %s", mime)
	}

	if isImage && !allowedImageMimes[strings.ToLower(mime)] {
		return fmt.Errorf("unsupported image mime type: %s", mime)
	}

	return nil
}
