package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/skills"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func (s *Server) ensureSkillRegistryLocked(rec *SessionRecord) *skills.SkillRegistry {
	if rec.Skills != nil {
		rec.Skills.Scan()
		return rec.Skills
	}
	registry := skills.New(rec.Cwd)
	if agentsHome := os.Getenv("AGENTS_HOME"); agentsHome != "" {
		registry.AddLayer(skills.SkillLayer{Title: "native-agents", Dir: filepath.Join(agentsHome, "skills"), Rank: 2})
	}
	registry.AddLayer(skills.SkillLayer{Title: "native-home", Dir: filepath.Join(s.homeDir, "skills"), Rank: 3})
	registry.Scan()
	rec.Skills = registry
	return registry
}

func (s *Server) ensureSkillRegistry(rec *SessionRecord) *skills.SkillRegistry {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return s.ensureSkillRegistryLocked(rec)
}

func (s *Server) listSkills(args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	sessionID, _ := request["sessionId"].(string)
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	registry := s.ensureSkillRegistry(rec)
	result := make([]map[string]any, 0)
	for _, skill := range registry.List() {
		if !skill.UserInvocable {
			continue
		}
		result = append(result, map[string]any{
			"name": skill.Name, "description": skill.Description,
			"path": skill.Path, "modelInvocable": skill.ModelInvocable,
		})
	}
	return map[string]any{"skills": result}, nil
}

func nativeSkillTool(registry *skills.SkillRegistry) *tools.Tool {
	allow := false
	return &tools.Tool{
		Name: "load_skill", Description: "Load the instructions of one available native skill",
		Schema: &tools.JsonSchemaNode{
			Type: tools.TypeObject,
			Properties: map[string]*tools.JsonSchemaNode{
				"name": {Type: tools.TypeString, Description: "Skill name"},
			},
			Required: []string{"name"}, AdditionalProperties: &allow,
		},
		Execute: func(_ context.Context, input map[string]any) (any, error) {
			name, _ := input["name"].(string)
			return skills.NewSkillTool(registry).Resolve(strings.TrimSpace(name), true)
		},
	}
}
