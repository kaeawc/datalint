package lsp

import "unicode/utf8"

// applyContentChange returns buf with one LSP textDocument/didChange
// content-change entry applied. A nil Range is a full-document
// replacement (Sync kind 1 semantics, still permitted under kind 2).
// A non-nil Range is an incremental edit: the byte slice spanning
// [Range.Start, Range.End) — measured in UTF-16 code units per the
// LSP default — is replaced by Text.
func applyContentChange(buf []byte, ch contentChange) []byte {
	if ch.Range == nil {
		return []byte(ch.Text)
	}
	startByte := positionToByte(buf, ch.Range.Start)
	endByte := positionToByte(buf, ch.Range.End)
	if endByte < startByte {
		endByte = startByte
	}
	out := make([]byte, 0, len(buf)-(endByte-startByte)+len(ch.Text))
	out = append(out, buf[:startByte]...)
	out = append(out, ch.Text...)
	out = append(out, buf[endByte:]...)
	return out
}

// positionToByte converts an LSP Position (zero-based line + UTF-16
// character offset within that line) to a byte offset into buf. A
// position past EOF clamps to len(buf); a position past the end of
// its line clamps to that line's terminator (the '\n', or EOF for
// the last line).
//
// LSP Position counts characters in UTF-16 code units by default —
// BMP runes are 1 unit, supplementary-plane runes are 2 (surrogate
// pair). This function decodes UTF-8 in the buffer and accumulates
// the equivalent UTF-16 unit count. Invalid UTF-8 bytes count as
// one unit each so the function still progresses.
func positionToByte(buf []byte, pos lspPosition) int {
	lineStart := lineStartOffset(buf, pos.Line)
	if lineStart < 0 {
		return len(buf)
	}
	return walkUTF16Units(buf, lineStart, pos.Character)
}

// lineStartOffset returns the byte offset of the first byte of the
// given zero-based line in buf, or -1 if the line is past EOF. Line
// 0 starts at offset 0; subsequent lines start one past each '\n'.
func lineStartOffset(buf []byte, line int) int {
	if line == 0 {
		return 0
	}
	seen := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] != '\n' {
			continue
		}
		seen++
		if seen == line {
			return i + 1
		}
	}
	return -1
}

// walkUTF16Units advances from byte offset start in buf, consuming
// rune-by-rune until it has accumulated character UTF-16 code units
// or hit a '\n' or EOF. Returns the resulting byte offset.
func walkUTF16Units(buf []byte, start, character int) int {
	offset := start
	units := 0
	for offset < len(buf) && units < character {
		if buf[offset] == '\n' {
			break
		}
		r, size := utf8.DecodeRune(buf[offset:])
		if r == utf8.RuneError && size == 1 {
			units++
			offset++
			continue
		}
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
		offset += size
	}
	return offset
}
