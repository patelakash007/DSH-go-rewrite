package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

const MaxPublicNameLength = 64

const hashLength = 12

var invalidNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func PublicToolName(serverName, rawName string) string {
	joined := "mcp__" + serverName + "__" + rawName
	normalized := invalidNameChars.ReplaceAllString(joined, "_")
	if normalized == joined && len(normalized) <= MaxPublicNameLength {
		return normalized
	}
	sum := sha256.Sum256([]byte(serverName + "\x00" + rawName))
	hash := hex.EncodeToString(sum[:])[:hashLength]
	keep := MaxPublicNameLength - hashLength - 1
	if keep > len(normalized) {
		keep = len(normalized)
	}
	return normalized[:keep] + "_" + hash
}
