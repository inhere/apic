package model

import "bytes"

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// StripBOM drops a leading UTF-8 byte order mark. Windows PowerShell 5
// (Set-Content -Encoding utf8) and older Notepad write one; left in place it
// becomes part of the first YAML key, so e.g. "sites" is silently ignored.
func StripBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}
