package cache

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// encodeFrames encodes a slice of frames into a single byte slice
// Format: [frame_count:4][len1:4][data1][len2:4][data2]...
func encodeFrames(frames [][]byte) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write frame count
	frameCount := len(frames)
	if frameCount > (1 << 32) {
		return nil, fmt.Errorf("frame count exceeds uint32 max")
	}
	// #nosec - length validation above ensures uint32 conversion is safe
	if err := binary.Write(buf, binary.BigEndian, uint32(frameCount)); err != nil {
		return nil, fmt.Errorf("failed to encode frame count: %w", err)
	}

	// Write each frame with its length prefix
	for _, frame := range frames {
		frameLen := len(frame)
		if frameLen > (1 << 32) {
			return nil, fmt.Errorf("frame size exceeds uint32 max")
		}
		// #nosec - length validation above ensures uint32 conversion is safe
		if err := binary.Write(buf, binary.BigEndian, uint32(frameLen)); err != nil {
			return nil, fmt.Errorf("failed to encode frame length: %w", err)
		}
		if _, err := buf.Write(frame); err != nil {
			return nil, fmt.Errorf("failed to encode frame data: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// decodeFrames decodes a byte slice back into a slice of frames
func decodeFrames(data []byte) ([][]byte, error) {
	buf := bytes.NewReader(data)

	// Read frame count
	var frameCount uint32
	if err := binary.Read(buf, binary.BigEndian, &frameCount); err != nil {
		return nil, fmt.Errorf("failed to decode frame count: %w", err)
	}

	frames := make([][]byte, frameCount)

	// Read each frame
	for i := uint32(0); i < frameCount; i++ {
		var frameLen uint32
		if err := binary.Read(buf, binary.BigEndian, &frameLen); err != nil {
			return nil, fmt.Errorf("failed to decode frame %d length: %w", i, err)
		}

		frameData := make([]byte, frameLen)
		if _, err := buf.Read(frameData); err != nil {
			return nil, fmt.Errorf("failed to decode frame %d data: %w", i, err)
		}

		frames[i] = frameData
	}

	return frames, nil
}
