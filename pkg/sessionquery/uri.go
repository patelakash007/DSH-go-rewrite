package sessionquery

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

const Scheme = "dsh-session:"

var enc = base64.RawURLEncoding

var payloadShape = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var ErrInvalidURI = errors.New("sessionquery: invalid session reference URI")

func EncodeURI(sessionID string) string {
	raw, _ := json.Marshal(sessionID)
	return Scheme + enc.EncodeToString(raw)
}

func DecodeURI(uri string) (string, error) {
	if !strings.HasPrefix(uri, Scheme) {
		return "", ErrInvalidURI
	}
	payload := uri[len(Scheme):]
	if !payloadShape.MatchString(payload) {
		return "", ErrInvalidURI
	}
	raw, err := enc.DecodeString(payload)
	if err != nil {
		return "", ErrInvalidURI
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return "", ErrInvalidURI
	}
	if EncodeURI(id) != uri {
		return "", ErrInvalidURI
	}
	return id, nil
}

var escapeLabel = regexp.MustCompile(`[\\\]]`)

func FormatMention(sessionID, label string) string {
	if label == "" {
		label = sessionID
	}
	label = escapeLabel.ReplaceAllStringFunc(label, func(m string) string { return `\` + m })
	return "@[" + label + "](" + EncodeURI(sessionID) + ")"
}
