package shelves

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const (
	// maxPhotoURLBytes limits the decoded size of data URI photos.
	maxPhotoURLBytes = 5 * 1024 * 1024 // 5MB
	// maxPhotoURLLength limits the character length of non-data-URI URLs.
	maxPhotoURLLength = 4096
)

// allowedImageMIMETypes lists permitted MIME types for data URI images.
var allowedImageMIMETypes = map[string]bool{
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
}

// sanitizePhotoURL validates and normalizes a photo URL or data URI.
func sanitizePhotoURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	if strings.HasPrefix(trimmed, "data:") {
		parts := strings.SplitN(trimmed, ",", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("%w: photoUrl data URI is invalid", ErrValidation)
		}

		// Extract and validate MIME type from the data URI header (e.g., "data:image/png;base64")
		header := parts[0]
		mimeType := strings.TrimPrefix(header, "data:")
		mimeType = strings.TrimSuffix(mimeType, ";base64")
		mimeType = strings.ToLower(mimeType)
		if !allowedImageMIMETypes[mimeType] {
			return "", fmt.Errorf("%w: photoUrl must be a valid image type (JPEG, PNG, GIF, WebP, or SVG)", ErrValidation)
		}

		if _, err := base64.StdEncoding.DecodeString(parts[1]); err != nil {
			return "", fmt.Errorf("%w: photoUrl must contain valid base64 image data", ErrValidation)
		}

		estimatedBytes := len(parts[1]) * 3 / 4
		if estimatedBytes > maxPhotoURLBytes {
			return "", fmt.Errorf("%w: photoUrl must be smaller than %dMB", ErrValidation, maxPhotoURLBytes/(1024*1024))
		}

		return trimmed, nil
	}

	if len(trimmed) > maxPhotoURLLength {
		return "", fmt.Errorf("%w: photoUrl must be shorter than %d characters", ErrValidation, maxPhotoURLLength)
	}

	// Validate external URL: must be valid URL with https scheme
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: photoUrl must be a valid URL", ErrValidation)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: photoUrl must use HTTPS", ErrValidation)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%w: photoUrl must have a valid host", ErrValidation)
	}

	return trimmed, nil
}
