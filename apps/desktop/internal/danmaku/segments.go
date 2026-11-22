package danmaku

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ParseSegments reads the document the segment endpoint answers with.
//
// The reply is protobuf, not the XML the list endpoint used to send: a repeated
// field 1 of comments, each carrying its moment, its mode, its size, its colour
// and its words, beside bookkeeping the player has no use for. The endpoint does
// not say how many segments an upload has: a segment past the end simply arrives
// with no comment in it.
func ParseSegments(body []byte) ([]comment, error) {
	var comments []comment
	for len(body) > 0 {
		field, wire, _, payload, rest, err := nextField(body)
		if err != nil {
			return nil, err
		}
		body = rest
		if field != 1 || wire != 2 {
			continue
		}
		one, err := readComment(payload)
		if err != nil {
			return nil, err
		}
		comments = append(comments, one)
	}
	return comments, nil
}

// comment is one comment as the segment endpoint sends it.
type comment struct {
	progress int32
	mode     int32
	size     int32
	color    uint32
	text     string
}

// readComment reads one comment out of its protobuf message.
//
// A comment carries the moment it was sent at, its mode, the size it is drawn
// at and its colour, and the words are field 7. The numbers are taken by the
// tag the site gives them rather than by the order they arrive in: the endpoint
// numbers them its own way, and reading them by position puts the id of whoever
// sent the comment where the moment belongs.
func readComment(body []byte) (comment, error) {
	var out comment
	for len(body) > 0 {
		field, wire, number, payload, rest, err := nextField(body)
		if err != nil {
			return comment{}, err
		}
		body = rest
		if wire != 0 {
			if field == 7 && wire == 2 {
				out.text = string(payload)
			}
			continue
		}
		switch field {
		case 2:
			out.progress = int32(number)
		case 3:
			out.mode = int32(number)
		case 4:
			out.size = int32(number)
		case 5:
			out.color = uint32(number)
		}
	}
	return out, nil
}

// nextField reads one field out of body and returns its value and the rest.
func nextField(body []byte) (field, wire int, number uint64, payload, rest []byte, err error) {
	tag, n := binary.Uvarint(body)
	if n <= 0 {
		return 0, 0, 0, nil, nil, errors.New("segments: bad tag")
	}
	body = body[n:]
	field, wire = int(tag>>3), int(tag&7)
	switch wire {
	case 0:
		got, n := binary.Uvarint(body)
		if n <= 0 {
			return 0, 0, 0, nil, nil, errors.New("segments: bad varint")
		}
		return field, wire, got, nil, body[n:], nil
	case 1:
		if len(body) < 8 {
			return 0, 0, 0, nil, nil, fmt.Errorf("segments: field %d cut short", field)
		}
		return field, wire, binary.LittleEndian.Uint64(body), nil, body[8:], nil
	case 2:
		length, n := binary.Uvarint(body)
		if n <= 0 {
			return 0, 0, 0, nil, nil, fmt.Errorf("segments: field %d has no length", field)
		}
		body = body[n:]
		if uint64(len(body)) < length {
			return 0, 0, 0, nil, nil, fmt.Errorf("segments: field %d cut short", field)
		}
		return field, wire, 0, body[:length], body[length:], nil
	case 5:
		if len(body) < 4 {
			return 0, 0, 0, nil, nil, fmt.Errorf("segments: field %d cut short", field)
		}
		return field, wire, uint64(binary.LittleEndian.Uint32(body)), nil, body[4:], nil
	}
	return 0, 0, 0, nil, nil, fmt.Errorf("segments: field %d has an unknown wire type %d", field, wire)
}
