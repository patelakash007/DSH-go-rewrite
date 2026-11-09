package attachment

import (
	"encoding/base64"
	"errors"
)

var ErrInvalidBase64 = errors.New("attachment: image upload is not canonical base64")

func DecodeCanonicalBase64(data string) ([]byte, error) {
	if data == "" {
		return nil, ErrInvalidBase64
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, ErrInvalidBase64
	}
	if base64.StdEncoding.EncodeToString(raw) != data {
		return nil, ErrInvalidBase64
	}
	return raw, nil
}
