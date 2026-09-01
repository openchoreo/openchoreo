// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagentrouter

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// maxClientHelloRecord bounds how many bytes we read looking for the SNI. A TLS
// ClientHello fits in a single record well under this.
const maxClientHelloRecord = 16 * 1024

var errNoSNI = errors.New("remoteagentrouter: no SNI in ClientHello")

// readClientHelloSNI reads the first TLS record from r (the ClientHello), extracts
// the SNI host, and returns both the SNI and the exact bytes consumed so the caller
// can replay them to the backend (this is a passthrough proxy — it never decrypts).
func readClientHelloSNI(r io.Reader) (sni string, raw []byte, err error) {
	// TLS record header: type(1) + version(2) + length(2). Fixed-size array so the
	// indexing below is statically bounded.
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return "", nil, fmt.Errorf("remoteagentrouter: read record header: %w", err)
	}
	recordType := hdr[0]
	if recordType != 0x16 { // handshake
		return "", nil, fmt.Errorf("remoteagentrouter: not a TLS handshake record (type %d)", recordType)
	}
	recLen := int(binary.BigEndian.Uint16(hdr[3:5]))
	if recLen == 0 || recLen > maxClientHelloRecord {
		return "", nil, fmt.Errorf("remoteagentrouter: bad ClientHello record length %d", recLen)
	}
	// Read the header + body into one buffer so the caller can replay the exact bytes.
	raw = make([]byte, len(hdr)+recLen)
	copy(raw, hdr[:])
	body := raw[len(hdr):]
	if _, err := io.ReadFull(r, body); err != nil {
		return "", nil, fmt.Errorf("remoteagentrouter: read record body: %w", err)
	}

	host, err := parseClientHelloSNI(body)
	if err != nil {
		return "", raw, err
	}
	return host, raw, nil
}

// parseClientHelloSNI extracts the server_name (host_name) from a TLS ClientHello
// handshake message body. Returns errNoSNI when absent.
func parseClientHelloSNI(b []byte) (string, error) {
	// Handshake header: msg_type(1) + length(3).
	if len(b) < 4 || b[0] != 0x01 { // ClientHello
		return "", errors.New("remoteagentrouter: not a ClientHello")
	}
	b = b[4:]

	// legacy_version(2) + random(32).
	if len(b) < 34 {
		return "", errNoSNI
	}
	b = b[34:]

	// session_id.
	b, ok := skipVec8(b)
	if !ok {
		return "", errNoSNI
	}
	// cipher_suites (uint16 length).
	b, ok = skipVec16(b)
	if !ok {
		return "", errNoSNI
	}
	// compression_methods (uint8 length).
	b, ok = skipVec8(b)
	if !ok {
		return "", errNoSNI
	}
	// extensions (uint16 length).
	if len(b) < 2 {
		return "", errNoSNI
	}
	extLen := int(binary.BigEndian.Uint16(b[:2]))
	b = b[2:]
	if extLen > len(b) {
		return "", errNoSNI
	}
	ext := b[:extLen]

	for len(ext) >= 4 {
		extType := binary.BigEndian.Uint16(ext[:2])
		l := int(binary.BigEndian.Uint16(ext[2:4]))
		ext = ext[4:]
		if l > len(ext) {
			return "", errNoSNI
		}
		data := ext[:l]
		ext = ext[l:]
		if extType != 0x0000 { // server_name
			continue
		}
		return parseSNIExtension(data)
	}
	return "", errNoSNI
}

// parseSNIExtension pulls the first host_name entry out of a server_name extension.
func parseSNIExtension(data []byte) (string, error) {
	// ServerNameList: list_length(2) then entries.
	if len(data) < 2 {
		return "", errNoSNI
	}
	listLen := int(binary.BigEndian.Uint16(data[:2]))
	data = data[2:]
	if listLen > len(data) {
		return "", errNoSNI
	}
	data = data[:listLen]
	for len(data) >= 3 {
		nameType := data[0]
		nameLen := int(binary.BigEndian.Uint16(data[1:3]))
		data = data[3:]
		if nameLen > len(data) {
			return "", errNoSNI
		}
		name := data[:nameLen]
		data = data[nameLen:]
		if nameType == 0x00 { // host_name
			return string(name), nil
		}
	}
	return "", errNoSNI
}

// skipVec8 skips a vector prefixed by a uint8 length.
func skipVec8(b []byte) ([]byte, bool) {
	if len(b) < 1 {
		return nil, false
	}
	n := int(b[0])
	if 1+n > len(b) {
		return nil, false
	}
	return b[1+n:], true
}

// skipVec16 skips a vector prefixed by a uint16 length.
func skipVec16(b []byte) ([]byte, bool) {
	if len(b) < 2 {
		return nil, false
	}
	n := int(binary.BigEndian.Uint16(b[:2]))
	if 2+n > len(b) {
		return nil, false
	}
	return b[2+n:], true
}
