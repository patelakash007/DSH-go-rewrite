package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type settingNamespace struct {
	Revision int            `json:"revision"`
	Value    map[string]any `json:"value"`
}

type nativeSettings struct {
	path       string
	mu         sync.Mutex
	namespaces map[string]*settingNamespace
}

func newNativeSettings(path string) (*nativeSettings, error) {
	settings := &nativeSettings{path: path, namespaces: map[string]*settingNamespace{}}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings.namespaces); err != nil {
			return nil, err
		}
	}
	settings.ensureDefaultsLocked()
	if err := settings.persistLocked(); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *nativeSettings) ensureDefaultsLocked() {
	if s.namespaces["agent-preset-registry"] == nil {
		s.namespaces["agent-preset-registry"] = &settingNamespace{Value: map[string]any{"selectedDefault": "native"}}
	}
	if s.namespaces["native-engine"] == nil {
		s.namespaces["native-engine"] = &settingNamespace{Value: map[string]any{
			"defaultPermission": "auto", "defaultProvider": "deepseek", "defaultModel": "deepseek-chat",
		}}
	}
	if s.namespaces["native-engine"].Value["defaultProvider"] == nil {
		s.namespaces["native-engine"].Value["defaultProvider"] = "deepseek"
	}
	if s.namespaces["native-engine"].Value["defaultModel"] == nil {
		s.namespaces["native-engine"].Value["defaultModel"] = "deepseek-chat"
	}
	if s.namespaces[piAiSettingsNamespace] == nil {
		s.namespaces[piAiSettingsNamespace] = &settingNamespace{Value: map[string]any{"providers": map[string]any{}}}
	}
	for _, namespace := range s.namespaces {
		if namespace.Value == nil {
			namespace.Value = map[string]any{}
		}
	}
}

func (s *nativeSettings) getString(namespace, key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	section := s.namespaces[namespace]
	if section == nil {
		return "", false
	}
	value, ok := section.Value[key].(string)
	return value, ok
}

func (s *Server) defaultPermission() string {
	value, ok := s.settings.getString("native-engine", "defaultPermission")
	if !ok || value == "" {
		return "auto"
	}
	return value
}

func (s *nativeSettings) describe() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.namespaces))
	for key := range s.namespaces {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, s.viewLocked(key))
	}
	return map[string]any{"writable": true, "hasDocument": true, "namespaces": rows}
}

func (s *nativeSettings) viewLocked(namespace string) map[string]any {
	section := s.namespaces[namespace]
	value := cloneJSONMap(section.Value)
	return map[string]any{
		"ns": namespace, "schema": nativeSettingSchema(namespace),
		"value": value, "base": map[string]any{}, "user": value,
		"revision": section.Revision, "autoGenerate": true, "secrets": []any{},
	}
}

func (s *nativeSettings) mutate(namespace string, ops []any, expected *int) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	section := s.namespaces[namespace]
	if section == nil {
		return nil, fmt.Errorf("settings namespace %q not found", namespace)
	}
	if expected != nil && section.Revision != *expected {
		return nil, fmt.Errorf("settings revision conflict")
	}
	value := cloneJSONMap(section.Value)
	for _, raw := range ops {
		op, _ := raw.(map[string]any)
		pathRaw, _ := op["path"].([]any)
		path := make([]string, 0, len(pathRaw))
		for _, segment := range pathRaw {
			if text, ok := segment.(string); ok && text != "" {
				path = append(path, text)
			}
		}
		if len(path) == 0 {
			return nil, fmt.Errorf("settings operation path is empty")
		}
		switch op["op"] {
		case "set":
			setJSONPath(value, path, op["value"])
		case "unset":
			unsetJSONPath(value, path)
		default:
			return nil, fmt.Errorf("unknown settings operation")
		}
	}
	if err := validateNativeSettings(namespace, value); err != nil {
		return nil, err
	}
	section.Value = value
	section.Revision++
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return s.viewLocked(namespace), nil
}

func (s *nativeSettings) update(namespace string, patch map[string]any, expected *int) (map[string]any, error) {
	ops := make([]any, 0, len(patch))
	for key, value := range patch {
		ops = append(ops, map[string]any{"op": "set", "path": []any{key}, "value": value})
	}
	return s.mutate(namespace, ops, expected)
}

func (s *nativeSettings) persistLocked() error {
	raw, err := json.MarshalIndent(s.namespaces, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".native-settings-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

func (s *Server) dispatchSettingsRPC(endpoint string, args map[string]any) (any, *RPCError) {
	if endpoint == "settings/describe" {
		return s.settings.describe(), nil
	}
	namespace, _ := args["ns"].(string)
	var expected *int
	if _, present := args["expectedRevision"]; present {
		value := numberArg(args, "expectedRevision", -1)
		expected = &value
	}
	var (
		view map[string]any
		err  error
	)
	if endpoint == "settings/update" {
		patch, _ := args["patch"].(map[string]any)
		view, err = s.settings.update(namespace, patch, expected)
	} else {
		ops, _ := args["ops"].([]any)
		view, err = s.settings.mutate(namespace, ops, expected)
	}
	if err != nil {
		code := "settings/rejected"
		if err.Error() == "settings revision conflict" {
			code = "settings/conflict"
		}
		return nil, &RPCError{Code: code, Message: err.Error()}
	}
	revision := numberArg(view, "revision", 0)
	s.emitHost(map[string]any{"type": "emit", "event": "settings/document-updated", "args": []any{namespace, revision}})
	return view, nil
}

func nativeSettingSchema(namespace string) map[string]any {
	fields := map[string]any{}
	switch namespace {
	case "agent-preset-registry":
		fields["selectedDefault"] = map[string]any{
			"type": "string", "meta": map[string]any{"description": "Default native agent preset", "required": true},
		}
	case "native-engine":
		fields["defaultPermission"] = map[string]any{
			"type": "string", "meta": map[string]any{"description": "Default native tool permission preset", "required": true},
		}
		fields["defaultProvider"] = map[string]any{
			"type": "string", "meta": map[string]any{"description": "Default provider for new sessions", "required": true},
		}
		fields["defaultModel"] = map[string]any{
			"type": "string", "meta": map[string]any{"description": "Default model for new sessions", "required": true},
		}
	case piAiSettingsNamespace:
		return providerSettingsSchema()
	}
	return map[string]any{"type": "object", "dict": fields, "meta": map[string]any{}}
}

func validateNativeSettings(namespace string, value map[string]any) error {
	switch namespace {
	case "agent-preset-registry":
		if selected, _ := value["selectedDefault"].(string); selected != "native" {
			return fmt.Errorf("only the native agent preset is installed")
		}
	case "native-engine":
		permission, _ := value["defaultPermission"].(string)
		if permission != "read-only" && permission != "auto" && permission != "danger-full-access" {
			return fmt.Errorf("invalid native permission preset")
		}
		provider, _ := value["defaultProvider"].(string)
		if provider == "" {
			provider = "deepseek"
		}
		model, _ := value["defaultModel"].(string)
		if model == "" {
			model = "deepseek-chat"
		}
		if provider != "deepseek" && provider != "deepseek-account" {
			return fmt.Errorf("only the built-in DeepSeek providers can be the default")
		}
	case piAiSettingsNamespace:
		return validateProviderSettings(value)
	default:
		return fmt.Errorf("settings namespace %q not found", namespace)
	}
	return nil
}

func cloneJSONMap(source map[string]any) map[string]any {
	raw, _ := json.Marshal(source)
	result := map[string]any{}
	_ = json.Unmarshal(raw, &result)
	return result
}

func setJSONPath(root map[string]any, path []string, value any) {
	current := root
	for _, key := range path[:len(path)-1] {
		next, _ := current[key].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

func unsetJSONPath(root map[string]any, path []string) {
	current := root
	for _, key := range path[:len(path)-1] {
		next, _ := current[key].(map[string]any)
		if next == nil {
			return
		}
		current = next
	}
	delete(current, path[len(path)-1])
}
