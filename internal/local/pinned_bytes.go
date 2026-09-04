package local

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// A Run snapshot carries the bytes it was pinned to - every definition, every
// context resource, every dispatched envelope - and it carries them again in
// every version it ever had and in every event that records a state. Those
// strings never change, so they are stored once by digest and referenced from
// each snapshot.
//
// The rewrite works on the raw document: only long string values are replaced,
// everything else is copied byte for byte, so unpacking reproduces the exact
// original and its digest still describes it.
const pinnedByteThreshold = 512

// pinnedMarker names a replaced string. A document that already contains this
// key is stored unpacked, so a reference can never be confused with data.
const pinnedMarker = `"$pinned"`

func packPinnedBytes(ctx context.Context, conn *sql.Conn, data json.RawMessage) (json.RawMessage, bool, error) {
	if len(data) < pinnedByteThreshold || bytes.Contains(data, []byte(pinnedMarker)) {
		return data, false, nil
	}
	packed := make([]byte, 0, len(data))
	pinned := map[string][]byte{}
	var walk func(i, depth int) (int, error)
	walk = func(i, depth int) (int, error) {
		end, err := scanValue(data, i)
		if err != nil {
			return 0, err
		}
		value := data[i:end]
		// A pinned definition, a context resource or a dispatched envelope is a
		// whole value that never changes. Above the threshold it is stored once
		// and referenced; the document itself is never replaced by a reference.
		if depth > 0 && end-i >= pinnedByteThreshold && (value[0] == '{' || value[0] == '[' || value[0] == '"') {
			digest := digestBytes(value)
			pinned[digest] = bytes.Clone(value)
			packed = append(packed, `{"$pinned":"`...)
			packed = append(packed, digest...)
			packed = append(packed, `"}`...)
			return end, nil
		}
		switch value[0] {
		case '{':
			packed = append(packed, '{')
			i++
			for i < end-1 {
				i = copySpace(data, i, &packed)
				if data[i] == ',' {
					packed = append(packed, ',')
					i++
					continue
				}
				keyEnd, err := scanValue(data, i)
				if err != nil {
					return 0, err
				}
				packed = append(packed, data[i:keyEnd]...)
				i = copySpace(data, keyEnd, &packed)
				if i >= end-1 || data[i] != ':' {
					return 0, errors.New("expected an object member")
				}
				packed = append(packed, ':')
				i = copySpace(data, i+1, &packed)
				if i, err = walk(i, depth+1); err != nil {
					return 0, err
				}
			}
			packed = append(packed, '}')
			return end, nil
		case '[':
			packed = append(packed, '[')
			i++
			for i < end-1 {
				i = copySpace(data, i, &packed)
				if data[i] == ',' {
					packed = append(packed, ',')
					i++
					continue
				}
				var err error
				if i, err = walk(i, depth+1); err != nil {
					return 0, err
				}
			}
			packed = append(packed, ']')
			return end, nil
		}
		packed = append(packed, value...)
		return end, nil
	}
	if _, err := walk(0, 0); err != nil {
		return data, false, nil // Not a document this rewrite understands.
	}
	if len(pinned) == 0 {
		return data, false, nil
	}
	insert, err := conn.PrepareContext(ctx, "INSERT INTO pinned_bytes(digest,bytes) VALUES(?,?) ON CONFLICT(digest) DO NOTHING")
	if err != nil {
		return nil, false, err
	}
	defer insert.Close()
	for digest, content := range pinned {
		if _, err := insert.ExecContext(ctx, digest, content); err != nil {
			return nil, false, err
		}
	}
	return packed, true, nil
}

// copySpace copies insignificant whitespace verbatim so that restoring a
// document reproduces its exact bytes.
func copySpace(data []byte, i int, out *[]byte) int {
	for i < len(data) {
		switch data[i] {
		case ' ', '\t', '\n', '\r':
			*out = append(*out, data[i])
			i++
		default:
			return i
		}
	}
	return i
}

// scanValue returns the index just past the JSON value that begins at i.
func scanValue(data []byte, i int) (int, error) {
	if i >= len(data) {
		return 0, errors.New("value expected")
	}
	switch data[i] {
	case '"':
		return endOfJSONString(data, i)
	case '{', '[':
		depth := 0
		for j := i; j < len(data); j++ {
			switch data[j] {
			case '"':
				end, err := endOfJSONString(data, j)
				if err != nil {
					return 0, err
				}
				j = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return j + 1, nil
				}
			}
		}
		return 0, errors.New("unterminated structure")
	}
	for j := i; j < len(data); j++ {
		switch data[j] {
		case ',', '}', ']', ' ', '\t', '\n', '\r':
			return j, nil
		}
	}
	return len(data), nil
}

// unpackPinnedBytes restores a stored document. A reference whose bytes are
// missing is an integrity failure: the document cannot be reproduced, and a
// partially restored state is not evidence of anything.
func unpackPinnedBytes(ctx context.Context, conn *sql.Conn, data json.RawMessage) (json.RawMessage, error) {
	marker := []byte(`{"$pinned":"`)
	if !bytes.Contains(data, marker) {
		return data, nil
	}
	restored := make([]byte, 0, len(data)*2)
	for i := 0; i < len(data); {
		if !bytes.HasPrefix(data[i:], marker) {
			restored = append(restored, data[i])
			i++
			continue
		}
		rest := data[i+len(marker):]
		end := bytes.Index(rest, []byte(`"}`))
		if end < 0 {
			return nil, fmt.Errorf("%w: unterminated pinned reference", ErrIntegrity)
		}
		digest := string(rest[:end])
		var content []byte
		err := conn.QueryRowContext(ctx, "SELECT bytes FROM pinned_bytes WHERE digest=?", digest).Scan(&content)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: pinned bytes %s are missing", ErrIntegrity, digest)
		}
		if err != nil {
			return nil, err
		}
		if digestBytes(content) != digest {
			return nil, ErrIntegrity
		}
		restored = append(restored, content...)
		i += len(marker) + end + 2
	}
	return restored, nil
}

// endOfJSONString returns the index just past the string literal starting at i.
func endOfJSONString(data []byte, i int) (int, error) {
	for j := i + 1; j < len(data); j++ {
		switch data[j] {
		case '\\':
			j++
		case '"':
			return j + 1, nil
		}
	}
	return 0, errors.New("unterminated string")
}

// restore returns the exact recorded document. A packed row is rebuilt from the
// shared byte store and checked against the digest recorded with it, so a
// restored document is proven identical to what was written.
func restore(ctx context.Context, conn *sql.Conn, data json.RawMessage, packed bool, digest string) (json.RawMessage, error) {
	if !packed {
		return data, nil
	}
	restored, err := unpackPinnedBytes(ctx, conn, data)
	if err != nil {
		return nil, err
	}
	if digest != "" && digestBytes(restored) != digest {
		return nil, ErrIntegrity
	}
	return restored, nil
}
