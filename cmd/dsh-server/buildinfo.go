package main

import (
	"encoding/json"
	"net/http"
	"runtime"
)

// Overridden by reproducible release builds with -ldflags -X. Development
// builds remain explicit instead of pretending to have release provenance.
var (
	engineVersion    = "dev"
	protocolVersion  = "native-v1"
	sourceRepository = "https://github.com/patelakash007/DSH-go-rewrite"
	sourceRevision   = "unknown"
	buildABI         = "" // Android ABI (arm64-v8a/x86_64), supplied by CI.
	sourceDirty      = "true"
)

type BuildInfo struct {
	EngineVersion    string `json:"engineVersion"`
	ProtocolVersion  string `json:"protocolVersion"`
	SourceRepository string `json:"sourceRepository"`
	SourceRevision   string `json:"sourceRevision"`
	SourceDirty      bool   `json:"sourceDirty"`
	ABI              string `json:"abi"`
	GOOS             string `json:"goos"`
	GOARCH           string `json:"goarch"`
	GoVersion        string `json:"goVersion"`
}

func currentBuildInfo() BuildInfo {
	abi := buildABI
	if abi == "" {
		abi = runtime.GOARCH
	}
	return BuildInfo{
		EngineVersion: engineVersion, ProtocolVersion: protocolVersion,
		SourceRepository: sourceRepository, SourceRevision: sourceRevision,
		SourceDirty: sourceDirty != "false", ABI: abi,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version(),
	}
}

func (s *Server) handleBuildInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isCookieAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(currentBuildInfo())
}
