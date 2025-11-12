// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package fleetsql

import (
	"database/sql"
	"image"
	_ "image/jpeg" // Register JPEG format
	_ "image/png"  // Register PNG format
	"os"
	"path/filepath"
)

// ImageMetadata contains metadata extracted from an image file.
type ImageMetadata struct {
	Width    int
	Height   int
	ByteSize int64
}

// ReadImageMetadata reads the width, height, and file size of an image.
// It supports JPEG and PNG formats. Returns zero values if the file cannot
// be read or is not a supported image format.
func ReadImageMetadata(basePath, relativePath string) ImageMetadata {
	if relativePath == "" {
		return ImageMetadata{}
	}

	// Construct full path
	fullPath := filepath.Join(basePath, relativePath)

	// Get file size
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return ImageMetadata{}
	}

	// Open and decode image
	f, err := os.Open(fullPath)
	if err != nil {
		return ImageMetadata{}
	}
	defer f.Close()

	// DecodeConfig is faster than Decode as it only reads the header
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return ImageMetadata{}
	}

	return ImageMetadata{
		Width:    config.Width,
		Height:   config.Height,
		ByteSize: fileInfo.Size(),
	}
}

// sqlNullInt64FromInt converts an int to sql.NullInt64, treating 0 as NULL.
func sqlNullInt64FromInt(i int) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{
		Int64: int64(i),
		Valid: true,
	}
}

// sqlNullInt64FromInt64 converts an int64 to sql.NullInt64, treating 0 as NULL.
func sqlNullInt64FromInt64(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{
		Int64: i,
		Valid: true,
	}
}
