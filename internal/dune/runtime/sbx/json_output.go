package sbx

import "bytes"

func trimToJSONPayload(output []byte) []byte {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil
	}

	object := bytes.IndexByte(trimmed, '{')
	array := bytes.IndexByte(trimmed, '[')
	switch {
	case object == -1 && array == -1:
		return nil
	case object == -1:
		return bytes.TrimSpace(trimmed[array:])
	case array == -1:
		return bytes.TrimSpace(trimmed[object:])
	case object < array:
		return bytes.TrimSpace(trimmed[object:])
	default:
		return bytes.TrimSpace(trimmed[array:])
	}
}
