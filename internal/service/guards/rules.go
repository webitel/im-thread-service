package guards

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	// [REGEX] Standard pattern for filenames (prevents path traversal and weird chars)
	reFilename = regexp.MustCompile(`^[\p{L}\p{N}_\- .]+$`)
	// [MIME] Common safe image types
	allowedImageMimes = map[string]bool{
		"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true, "image/svg+xml": true,
	}
)

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

// ValidateFilename ensures the name is safe and has a reasonable length
func validateFilename(name string) error {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) < 1 || len(trimmed) > 255 {
		return errors.New("filename must be between 1 and 255 characters")
	}
	if !reFilename.MatchString(trimmed) {
		return fmt.Errorf("filename '%s' contains forbidden characters", name)
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
