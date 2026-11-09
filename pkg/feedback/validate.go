package feedback

import (
	"errors"
	"strings"
)

var ErrInvalidRating = errors.New("feedback: rating must be thumbs up or down")

var ErrEmptyNote = errors.New("feedback: note must contain a non-whitespace character")

func ValidateRating(r Rating) error {
	if r != RatingThumbsUp && r != RatingThumbsDown {
		return ErrInvalidRating
	}
	return nil
}

func ValidateNote(note string) error {
	if note == "" {
		return nil
	}
	if strings.TrimSpace(note) == "" {
		return ErrEmptyNote
	}
	return nil
}
