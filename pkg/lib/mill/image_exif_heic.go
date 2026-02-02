//go:build cgo && !noheic

package mill

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/adrium/goheif/heif"
)

// isHEICFormat checks if the data starts with HEIC magic bytes.
// HEIC files start with ftyp box followed by a brand like "heic", "heix", "mif1", etc.
func isHEICFormat(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	// Check for "ftyp" at offset 4
	if string(data[4:8]) != "ftyp" {
		return false
	}
	// Check for HEIC-related brands
	brand := string(data[8:12])
	switch brand {
	case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
		return true
	}
	return false
}

// heicConfig holds HEIC image configuration extracted from metadata.
type heicConfig struct {
	Width     int
	Height    int
	Rotations int
}

// getHEICConfig extracts image dimensions and rotation from HEIC file.
// It handles files where mdat box comes before meta box by reordering.
func getHEICConfig(data []byte) (heicConfig, error) {
	ra := bytes.NewReader(data)
	hf := heif.Open(ra)

	it, err := hf.PrimaryItem()
	if err != nil {
		// Try reordering boxes if standard parsing fails
		reordered, reorderErr := reorderHEICForMetadata(data)
		if reorderErr != nil {
			return heicConfig{}, fmt.Errorf("get primary item: %w (reorder failed: %v)", err, reorderErr)
		}
		ra = bytes.NewReader(reordered)
		hf = heif.Open(ra)
		it, err = hf.PrimaryItem()
		if err != nil {
			return heicConfig{}, fmt.Errorf("get primary item after reorder: %w", err)
		}
	}

	width, height, ok := it.SpatialExtents()
	if !ok {
		return heicConfig{}, fmt.Errorf("no spatial extents found")
	}

	return heicConfig{
		Width:     width,
		Height:    height,
		Rotations: it.Rotations(),
	}, nil
}

type heicBoxInfo struct {
	offset int64
	size   int64
	typ    string
}

// parseHEICBoxes parses the top-level boxes in a HEIC file.
func parseHEICBoxes(data []byte) ([]heicBoxInfo, error) {
	var boxes []heicBoxInfo
	offset := int64(0)

	for offset < int64(len(data))-8 {
		size := int64(binary.BigEndian.Uint32(data[offset : offset+4]))
		boxType := string(data[offset+4 : offset+8])

		if size == 0 {
			size = int64(len(data)) - offset
		}

		actualSize := size
		if size == 1 {
			if offset+16 > int64(len(data)) {
				break
			}
			actualSize = int64(binary.BigEndian.Uint64(data[offset+8 : offset+16]))
		}

		boxes = append(boxes, heicBoxInfo{offset: offset, size: actualSize, typ: boxType})
		offset += actualSize
	}

	return boxes, nil
}

// reorderHEICForMetadata creates a byte slice with ftyp + meta boxes,
// skipping mdat and other data boxes. This allows parsing metadata
// from files where mdat comes before meta.
func reorderHEICForMetadata(data []byte) ([]byte, error) {
	boxes, err := parseHEICBoxes(data)
	if err != nil {
		return nil, err
	}

	var ftyp, meta *heicBoxInfo
	for i := range boxes {
		switch boxes[i].typ {
		case "ftyp":
			ftyp = &boxes[i]
		case "meta":
			meta = &boxes[i]
		}
	}

	if ftyp == nil {
		return nil, fmt.Errorf("ftyp box not found")
	}
	if meta == nil {
		return nil, fmt.Errorf("meta box not found")
	}

	// Create reordered data: ftyp + meta
	result := make([]byte, 0, ftyp.size+meta.size)
	result = append(result, data[ftyp.offset:ftyp.offset+ftyp.size]...)
	result = append(result, data[meta.offset:meta.offset+meta.size]...)

	return result, nil
}

// reorderHEICFull reorders HEIC boxes to put meta before mdat and updates iloc offsets.
// This is needed for full image decoding, not just metadata parsing.
func reorderHEICFull(data []byte) ([]byte, error) {
	boxes, err := parseHEICBoxes(data)
	if err != nil {
		return nil, err
	}

	var ftyp, mdat, meta *heicBoxInfo
	var others []*heicBoxInfo

	for i := range boxes {
		switch boxes[i].typ {
		case "ftyp":
			ftyp = &boxes[i]
		case "mdat":
			mdat = &boxes[i]
		case "meta":
			meta = &boxes[i]
		default:
			others = append(others, &boxes[i])
		}
	}

	if ftyp == nil || meta == nil {
		return nil, fmt.Errorf("required boxes not found")
	}

	// Check if already in correct order (meta before mdat or no mdat)
	if mdat == nil || meta.offset < mdat.offset {
		return data, nil
	}

	// Calculate offset adjustment for iloc
	// New layout: ftyp, meta, mdat, others
	// mdat moves from offset=ftyp.size to offset=ftyp.size+meta.size
	offsetAdjustment := meta.size

	// Create reordered data
	newData := make([]byte, len(data))
	newOffset := int64(0)

	// Copy ftyp
	copy(newData[newOffset:], data[ftyp.offset:ftyp.offset+ftyp.size])
	newOffset += ftyp.size

	// Copy meta with updated iloc offsets
	metaData := make([]byte, meta.size)
	copy(metaData, data[meta.offset:meta.offset+meta.size])
	updateIlocOffsets(metaData, offsetAdjustment)
	copy(newData[newOffset:], metaData)
	newOffset += meta.size

	// Copy mdat
	copy(newData[newOffset:], data[mdat.offset:mdat.offset+mdat.size])
	newOffset += mdat.size

	// Copy other boxes
	for _, box := range others {
		copy(newData[newOffset:], data[box.offset:box.offset+box.size])
		newOffset += box.size
	}

	return newData[:newOffset], nil
}

// updateIlocOffsets updates the iloc box offsets inside the meta box.
func updateIlocOffsets(metaData []byte, adjustment int64) {
	// Meta structure: size(4) + type(4) + version/flags(4) + children
	offset := int64(12)
	for offset < int64(len(metaData))-8 {
		size := int64(binary.BigEndian.Uint32(metaData[offset : offset+4]))
		boxType := string(metaData[offset+4 : offset+8])

		if size == 0 {
			break
		}

		if boxType == "iloc" {
			updateIloc(metaData[offset:offset+size], adjustment)
			return
		}

		offset += size
	}
}

// updateIloc updates offsets in the iloc box.
func updateIloc(iloc []byte, adjustment int64) {
	if len(iloc) < 14 {
		return
	}

	version := iloc[8]
	offsetSizeField := int((iloc[12] >> 4) & 0x0F)
	lengthSizeField := int(iloc[12] & 0x0F)
	baseOffsetSizeField := int((iloc[13] >> 4) & 0x0F)
	indexSizeField := int(iloc[13] & 0x0F)

	pos := 14

	var itemCount int
	if version < 2 {
		itemCount = int(binary.BigEndian.Uint16(iloc[pos : pos+2]))
		pos += 2
	} else {
		itemCount = int(binary.BigEndian.Uint32(iloc[pos : pos+4]))
		pos += 4
	}

	for i := 0; i < itemCount && pos < len(iloc); i++ {
		// item_id
		if version < 2 {
			pos += 2
		} else {
			pos += 4
		}

		// construction_method (version 1+)
		var constructionMethod uint8
		if version >= 1 {
			constructionMethod = iloc[pos+1] & 0x0F
			pos += 2
		}

		// data_reference_index
		pos += 2

		// base_offset - only adjust if construction_method is 0
		if baseOffsetSizeField > 0 {
			if constructionMethod == 0 {
				baseOffset := readSizedInt(iloc[pos:], baseOffsetSizeField)
				writeSizedInt(iloc[pos:], baseOffsetSizeField, baseOffset+adjustment)
			}
			pos += baseOffsetSizeField
		}

		// extent_count
		extentCount := int(binary.BigEndian.Uint16(iloc[pos : pos+2]))
		pos += 2

		// extents
		for j := 0; j < extentCount && pos < len(iloc); j++ {
			// extent_index (version 1+ if indexSizeField > 0)
			if version >= 1 && indexSizeField > 0 {
				pos += indexSizeField
			}

			// extent_offset - only adjust if construction_method is 0
			if offsetSizeField > 0 {
				if constructionMethod == 0 {
					extentOffset := readSizedInt(iloc[pos:], offsetSizeField)
					writeSizedInt(iloc[pos:], offsetSizeField, extentOffset+adjustment)
				}
				pos += offsetSizeField
			}

			// extent_length
			pos += lengthSizeField
		}
	}
}

func readSizedInt(data []byte, size int) int64 {
	switch size {
	case 2:
		return int64(binary.BigEndian.Uint16(data[:2]))
	case 4:
		return int64(binary.BigEndian.Uint32(data[:4]))
	case 8:
		return int64(binary.BigEndian.Uint64(data[:8]))
	}
	return 0
}

func writeSizedInt(data []byte, size int, value int64) {
	switch size {
	case 2:
		binary.BigEndian.PutUint16(data[:2], uint16(value))
	case 4:
		binary.BigEndian.PutUint32(data[:4], uint32(value))
	case 8:
		binary.BigEndian.PutUint64(data[:8], uint64(value))
	}
}

// decodeHEICConfig tries to decode HEIC image configuration.
// Returns the config, format string, and error.
func decodeHEICConfig(r io.ReadSeeker) (width, height int, format string, err error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, 0, "", fmt.Errorf("read heic data: %w", err)
	}

	if !isHEICFormat(data) {
		return 0, 0, "", fmt.Errorf("not a HEIC file")
	}

	cfg, err := getHEICConfig(data)
	if err != nil {
		return 0, 0, "", err
	}

	// Reset reader position
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, 0, "", fmt.Errorf("seek: %w", err)
	}

	return cfg.Width, cfg.Height, "heic", nil
}
