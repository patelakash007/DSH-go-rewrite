package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/tools"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const maxScheduleHistory = 200

type scheduleDelivery struct {
	ScheduledAt     string `json:"scheduledAt,omitempty"`
	DeliveredAt     string `json:"deliveredAt"`
	Prompt          string `json:"prompt,omitempty"`
	TargetSessionID string `json:"targetSessionId,omitempty"`
}

type scheduleTask struct {
	ID                 string             `json:"id"`
	Kind               string             `json:"kind"`
	Title              string             `json:"title"`
	Prompt             string             `json:"prompt"`
	Status             string             `json:"status"`
	Target             map[string]any     `json:"target"`
	ScheduledAt        string             `json:"scheduledAt,omitempty"`
	AfterSeconds       *int               `json:"afterSeconds,omitempty"`
	EverySeconds       *int               `json:"everySeconds,omitempty"`
	Time               string             `json:"time,omitempty"`
	TimeZone           string             `json:"timeZone,omitempty"`
	Weekdays           []int              `json:"weekdays,omitempty"`
	Expression         string             `json:"expression,omitempty"`
	CreatedAt          string             `json:"createdAt"`
	UpdatedAt          string             `json:"updatedAt"`
	LastDelivery       *scheduleDelivery  `json:"lastDelivery,omitempty"`
	InvolvedSessionIDs []string           `json:"involvedSessionIds,omitempty"`
	History            []scheduleDelivery `json:"history,omitempty"`
}

type scheduleStore struct {
	path  string
	mu    sync.Mutex
	tasks map[string]*scheduleTask
}

func newScheduleStore(path string) (*scheduleStore, error) {
	store := &scheduleStore{path: path, tasks: map[string]*scheduleTask{}}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(raw) > 0 {
		var tasks []*scheduleTask
		if err := json.Unmarshal(raw, &tasks); err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if task != nil && task.ID != "" {
				store.tasks[task.ID] = task
			}
		}
	}
	return store, nil
}

func (s *scheduleStore) persistLocked() error {
	tasks := make([]*scheduleTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt < tasks[j].CreatedAt })
	raw, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".schedules-*")
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

func (s *Server) startScheduleLoop() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.fireDueSchedules()
		}
	}()
}

func (s *Server) fireDueSchedules() {
	now := time.Now()
	s.schedules.mu.Lock()
	var due []*scheduleTask
	for _, task := range s.schedules.tasks {
		if task.Status != "active" || task.ScheduledAt == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, task.ScheduledAt)
		if err == nil && !at.After(now) {
			copy := *task
			due = append(due, &copy)
		}
	}
	s.schedules.mu.Unlock()
	for _, task := range due {
		s.deliverSchedule(task, true)
	}
}

func (s *Server) deliverSchedule(task *scheduleTask, scheduled bool) {
	targetSession := ""
	target, _ := task.Target["kind"].(string)
	switch target {
	case "session":
		targetSession, _ = task.Target["sessionId"].(string)
	case "new_once", "new":
		rec := s.getOrCreateSession(uuid.MustV4(), "")
		targetSession = rec.ID.Raw()
	}
	if targetSession == "" {
		s.recordScheduleDelivery(task, "", "no target session")
		return
	}
	rec := s.sessionByID(targetSession)
	if rec == nil {
		rec = s.getOrCreateSession(targetSession, "")
	}
	runtimeAgent := s.ensureAgent(rec)
	rec.mu.Lock()
	rec.Cancel = func() { runtimeAgent.Cancel(agent.CancelUser) }
	rec.mu.Unlock()
	var err error
	if runtimeAgent.Status() == agent.StatusRunning {
		err = runtimeAgent.FollowupRequest(task.Prompt, uuid.MustV4())
	} else {
		err = runtimeAgent.RunRequest(task.Prompt, uuid.MustV4())
	}
	if err != nil {
		s.recordScheduleDelivery(task, targetSession, err.Error())
		return
	}
	s.recordScheduleDelivery(task, targetSession, "")
}

func (s *Server) recordScheduleDelivery(task *scheduleTask, targetSession, failure string) {
	now := time.Now()
	s.schedules.mu.Lock()
	stored := s.schedules.tasks[task.ID]
	if stored == nil {
		s.schedules.mu.Unlock()
		return
	}
	delivery := scheduleDelivery{
		ScheduledAt: stored.ScheduledAt, DeliveredAt: now.UTC().Format(time.RFC3339Nano),
		Prompt: stored.Prompt, TargetSessionID: targetSession,
	}
	if failure != "" {
		delivery.Prompt = failure
	}
	stored.LastDelivery = &delivery
	stored.History = append(stored.History, delivery)
	if len(stored.History) > maxScheduleHistory {
		stored.History = stored.History[len(stored.History)-maxScheduleHistory:]
	}
	if stored.Kind == "at" || stored.Kind == "after" {
		stored.Status = "inactive"
		stored.ScheduledAt = ""
	} else {
		next, err := nextScheduleTime(stored, now)
		if err != nil {
			stored.Status = "inactive"
			stored.ScheduledAt = ""
		} else {
			stored.ScheduledAt = next.UTC().Format(time.RFC3339Nano)
		}
	}
	stored.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	_ = s.schedules.persistLocked()
	s.schedules.mu.Unlock()
	s.emitHost(map[string]any{"type": "emit", "event": "schedule/changed", "args": []any{stored.ID}})
}

func (s *Server) dispatchScheduleRPC(endpoint string, args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	switch endpoint {
	case "schedule/catalog":
		s.schedules.mu.Lock()
		tasks := make([]*scheduleTask, 0, len(s.schedules.tasks))
		for _, task := range s.schedules.tasks {
			copy := *task
			copy.History = nil
			tasks = append(tasks, &copy)
		}
		s.schedules.mu.Unlock()
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt < tasks[j].CreatedAt })
		return tasks, nil
	case "schedule/edit":
		id, _ := request["id"].(string)
		action, _ := request["action"].(string)
		expected, _ := request["expectedUpdatedAt"].(string)
		s.schedules.mu.Lock()
		task := s.schedules.tasks[id]
		if task == nil {
			s.schedules.mu.Unlock()
			return map[string]any{"code": "schedule_not_found"}, nil
		}
		if expected != "" && expected != task.UpdatedAt {
			s.schedules.mu.Unlock()
			return map[string]any{"code": "schedule_conflict"}, nil
		}
		if action == "delete" {
			delete(s.schedules.tasks, id)
			_ = s.schedules.persistLocked()
			s.schedules.mu.Unlock()
			s.emitHost(map[string]any{"type": "emit", "event": "schedule/changed", "args": []any{id}})
			return map[string]any{"deleted": true, "id": id}, nil
		}
		patch, _ := request["patch"].(map[string]any)
		if title, ok := patch["title"].(string); ok {
			task.Title = strings.TrimSpace(title)
		}
		if prompt, ok := patch["prompt"].(string); ok {
			task.Prompt = prompt
		}
		if timing, ok := patch["timing"].(map[string]any); ok {
			if err := applyScheduleTiming(task, timing, time.Now()); err != nil {
				s.schedules.mu.Unlock()
				return map[string]any{"code": "invalid_rule", "message": err.Error()}, nil
			}
		}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.schedules.persistLocked()
		copy := *task
		copy.History = nil
		s.schedules.mu.Unlock()
		s.emitHost(map[string]any{"type": "emit", "event": "schedule/changed", "args": []any{id}})
		return copy, nil
	case "schedule/run":
		id, _ := request["id"].(string)
		s.schedules.mu.Lock()
		task := s.schedules.tasks[id]
		if task == nil {
			s.schedules.mu.Unlock()
			return map[string]any{"code": "schedule_not_found"}, nil
		}
		copy := *task
		s.schedules.mu.Unlock()
		s.deliverSchedule(&copy, false)
		return map[string]any{"id": id}, nil
	case "schedule/history":
		id, _ := request["id"].(string)
		limit := numberArg(request, "limit", 20)
		if limit < 1 {
			limit = 1
		}
		if limit > 100 {
			limit = 100
		}
		s.schedules.mu.Lock()
		task := s.schedules.tasks[id]
		if task == nil {
			s.schedules.mu.Unlock()
			return map[string]any{"code": "schedule_not_found"}, nil
		}
		history := append([]scheduleDelivery(nil), task.History...)
		s.schedules.mu.Unlock()
		sort.Slice(history, func(i, j int) bool { return history[i].DeliveredAt > history[j].DeliveredAt })
		before, _ := request["before"].(string)
		if before != "" {
			filtered := history[:0]
			for _, run := range history {
				if run.DeliveredAt < before {
					filtered = append(filtered, run)
				}
			}
			history = filtered
		}
		nextBefore := ""
		if len(history) > limit {
			nextBefore = history[limit-1].DeliveredAt
			history = history[:limit]
		}
		return map[string]any{
			"records": history, "nextBefore": nextBefore,
			"retention": map[string]any{"records": maxScheduleHistory},
		}, nil
	case "schedule/update":
		id, _ := request["id"].(string)
		s.schedules.mu.Lock()
		task := s.schedules.tasks[id]
		if task == nil {
			s.schedules.mu.Unlock()
			return map[string]any{"code": "schedule_not_found"}, nil
		}
		if title, ok := request["title"].(string); ok {
			task.Title = strings.TrimSpace(title)
		}
		if prompt, ok := request["prompt"].(string); ok {
			task.Prompt = prompt
		}
		if change, ok := request["change"].(map[string]any); ok {
			if err := applyScheduleTiming(task, change, time.Now()); err != nil {
				s.schedules.mu.Unlock()
				return map[string]any{"code": "invalid_rule", "message": err.Error()}, nil
			}
		}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.schedules.persistLocked()
		copy := *task
		copy.History = nil
		s.schedules.mu.Unlock()
		s.emitHost(map[string]any{"type": "emit", "event": "schedule/changed", "args": []any{id}})
		return map[string]any{"updated": true, "record": copy}, nil
	case "schedule/delete":
		id, _ := request["id"].(string)
		s.schedules.mu.Lock()
		_, exists := s.schedules.tasks[id]
		delete(s.schedules.tasks, id)
		_ = s.schedules.persistLocked()
		s.schedules.mu.Unlock()
		if exists {
			s.emitHost(map[string]any{"type": "emit", "event": "schedule/changed", "args": []any{id}})
		}
		return map[string]any{"deleted": exists}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown schedule method"}
	}
}

func applyScheduleTiming(task *scheduleTask, timing map[string]any, now time.Time) error {
	kind, _ := timing["kind"].(string)
	if kind == "" {
		kind = task.Kind
	}
	switch kind {
	case "at":
		selector, _ := timing["at"].(map[string]any)
		if selector == nil {
			selector = timing
		}
		date, _ := selector["date"].(string)
		clock, _ := selector["time"].(string)
		zone, _ := selector["time_zone"].(string)
		location, err := loadScheduleLocation(zone)
		if err != nil {
			return err
		}
		parsed, err := time.ParseInLocation("2006-01-02 15:04:05", date+" "+clock, location)
		if err != nil {
			parsed, err = time.ParseInLocation("2006-01-02 15:04", date+" "+clock, location)
		}
		if err != nil {
			return fmt.Errorf("invalid at date/time")
		}
		if !parsed.After(now) {
			return fmt.Errorf("scheduled time is not in the future")
		}
		task.Kind = "at"
		task.ScheduledAt = parsed.UTC().Format(time.RFC3339Nano)
		task.Time = clock
		task.TimeZone = zone
		task.AfterSeconds = nil
		task.EverySeconds = nil
		task.Weekdays = nil
		task.Expression = ""
	case "every":
		seconds := numberArg(timing, "every_seconds", numberArg(timing, "everySeconds", 0))
		if seconds < 60 {
			return fmt.Errorf("every interval must be at least 60 seconds")
		}
		task.Kind = "every"
		task.EverySeconds = &seconds
		task.ScheduledAt = now.Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339Nano)
		task.Time = ""
		task.Weekdays = nil
		task.Expression = ""
	case "daily":
		selector, _ := timing["daily"].(map[string]any)
		if selector == nil {
			selector = timing
		}
		clock, _ := selector["time"].(string)
		zone, _ := selector["time_zone"].(string)
		location, err := loadScheduleLocation(zone)
		if err != nil {
			return err
		}
		next, err := nextDaily(now, clock, location)
		if err != nil {
			return err
		}
		task.Kind = "daily"
		task.Time = clock
		task.TimeZone = zone
		task.ScheduledAt = next.UTC().Format(time.RFC3339Nano)
		task.EverySeconds = nil
		task.Weekdays = nil
		task.Expression = ""
	case "weekly":
		selector, _ := timing["weekly"].(map[string]any)
		if selector == nil {
			selector = timing
		}
		clock, _ := selector["time"].(string)
		zone, _ := selector["time_zone"].(string)
		weekdays := intList(selector["weekdays"])
		if len(weekdays) == 0 {
			return fmt.Errorf("weekly requires at least one weekday")
		}
		location, err := loadScheduleLocation(zone)
		if err != nil {
			return err
		}
		next, err := nextWeekly(now, clock, weekdays, location)
		if err != nil {
			return err
		}
		task.Kind = "weekly"
		task.Time = clock
		task.TimeZone = zone
		task.Weekdays = weekdays
		task.ScheduledAt = next.UTC().Format(time.RFC3339Nano)
		task.EverySeconds = nil
		task.Expression = ""
	case "cron":
		selector, _ := timing["cron"].(map[string]any)
		if selector == nil {
			selector = timing
		}
		expression, _ := selector["expression"].(string)
		zone, _ := selector["time_zone"].(string)
		location, err := loadScheduleLocation(zone)
		if err != nil {
			return err
		}
		next, err := nextCron(now, expression, location)
		if err != nil {
			return err
		}
		task.Kind = "cron"
		task.Expression = expression
		task.TimeZone = zone
		task.ScheduledAt = next.UTC().Format(time.RFC3339Nano)
		task.EverySeconds = nil
		task.Weekdays = nil
		task.Time = ""
	default:
		return fmt.Errorf("unknown schedule kind %q", kind)
	}
	task.Status = "active"
	return nil
}

func nextScheduleTime(task *scheduleTask, now time.Time) (time.Time, error) {
	switch task.Kind {
	case "every":
		if task.EverySeconds == nil || *task.EverySeconds < 60 {
			return time.Time{}, fmt.Errorf("invalid every interval")
		}
		return now.Add(time.Duration(*task.EverySeconds) * time.Second), nil
	case "daily":
		location, err := loadScheduleLocation(task.TimeZone)
		if err != nil {
			return time.Time{}, err
		}
		return nextDaily(now, task.Time, location)
	case "weekly":
		location, err := loadScheduleLocation(task.TimeZone)
		if err != nil {
			return time.Time{}, err
		}
		return nextWeekly(now, task.Time, task.Weekdays, location)
	case "cron":
		location, err := loadScheduleLocation(task.TimeZone)
		if err != nil {
			return time.Time{}, err
		}
		return nextCron(now, task.Expression, location)
	default:
		return time.Time{}, fmt.Errorf("one-shot schedule has no next time")
	}
}

func loadScheduleLocation(zone string) (*time.Location, error) {
	if zone == "" {
		return time.Local, nil
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q", zone)
	}
	return location, nil
}

func parseClock(clock string) (int, int, int, error) {
	parts := strings.Split(clock, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, fmt.Errorf("invalid time %q", clock)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, 0, fmt.Errorf("invalid hour")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, 0, fmt.Errorf("invalid minute")
	}
	second := 0
	if len(parts) == 3 {
		second, err = strconv.Atoi(strings.Split(parts[2], ".")[0])
		if err != nil || second < 0 || second > 59 {
			return 0, 0, 0, fmt.Errorf("invalid second")
		}
	}
	return hour, minute, second, nil
}

func nextDaily(now time.Time, clock string, location *time.Location) (time.Time, error) {
	hour, minute, second, err := parseClock(clock)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(location)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, second, 0, location)
	if !candidate.After(local) {
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate, nil
}

func nextWeekly(now time.Time, clock string, weekdays []int, location *time.Location) (time.Time, error) {
	hour, minute, second, err := parseClock(clock)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(location)
	for offset := 0; offset <= 8; offset++ {
		day := local.AddDate(0, 0, offset)
		iso := int(day.Weekday())
		if iso == 0 {
			iso = 7
		}
		if !containsInt(weekdays, iso) {
			continue
		}
		candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, second, 0, location)
		if candidate.After(local) {
			return candidate, nil
		}
	}
	return time.Time{}, fmt.Errorf("no weekly occurrence")
}

func nextCron(now time.Time, expression string, location *time.Location) (time.Time, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron expression must have five fields")
	}
	minutes, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, err
	}
	hours, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, err
	}
	days, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, err
	}
	months, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, err
	}
	weekdays, err := parseCronField(fields[4], 0, 7)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(location).Truncate(time.Minute).Add(time.Minute)
	limit := local.AddDate(1, 0, 0)
	for !local.After(limit) {
		weekday := int(local.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if containsInt(minutes, local.Minute()) && containsInt(hours, local.Hour()) &&
			containsInt(days, local.Day()) && containsInt(months, int(local.Month())) &&
			(containsInt(weekdays, weekday) || (weekday == 7 && containsInt(weekdays, 0))) {
			return local, nil
		}
		local = local.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron expression has no occurrence within a year")
}

func parseCronField(field string, minimum, maximum int) ([]int, error) {
	values := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		step := 1
		if slash := strings.Index(part, "/"); slash >= 0 {
			parsed, err := strconv.Atoi(part[slash+1:])
			if err != nil || parsed < 1 {
				return nil, fmt.Errorf("invalid cron step")
			}
			step = parsed
			part = part[:slash]
		}
		start, end := minimum, maximum
		if part != "*" {
			if dash := strings.Index(part, "-"); dash >= 0 {
				parsedStart, err := strconv.Atoi(part[:dash])
				if err != nil {
					return nil, fmt.Errorf("invalid cron range")
				}
				parsedEnd, err := strconv.Atoi(part[dash+1:])
				if err != nil {
					return nil, fmt.Errorf("invalid cron range")
				}
				start, end = parsedStart, parsedEnd
			} else {
				parsed, err := strconv.Atoi(part)
				if err != nil {
					return nil, fmt.Errorf("invalid cron value")
				}
				start, end = parsed, parsed
			}
		}
		if start < minimum || end > maximum || start > end {
			return nil, fmt.Errorf("cron value out of range")
		}
		for value := start; value <= end; value += step {
			values[value] = true
		}
	}
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Ints(result)
	return result, nil
}

func intList(raw any) []int {
	list, _ := raw.([]any)
	result := make([]int, 0, len(list))
	for _, item := range list {
		switch value := item.(type) {
		case int:
			result = append(result, value)
		case float64:
			result = append(result, int(value))
		}
	}
	return result
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *Server) scheduleCreateTool(sessionID string) *tools.Tool {
	allow := false
	return &tools.Tool{
		Name:        "schedule_create",
		Description: "Create a durable reminder or recurring automation task that delivers a prompt to this conversation",
		Schema: &tools.JsonSchemaNode{
			Type: tools.TypeObject,
			Properties: map[string]*tools.JsonSchemaNode{
				"title":  {Type: tools.TypeString, Description: "Short task title"},
				"prompt": {Type: tools.TypeString, Description: "Prompt delivered when the task fires"},
				"at": {Type: tools.TypeObject, Description: "One-shot local date and time", Properties: map[string]*tools.JsonSchemaNode{
					"date": {Type: tools.TypeString}, "time": {Type: tools.TypeString}, "time_zone": {Type: tools.TypeString},
				}, Required: []string{"date", "time", "time_zone"}, AdditionalProperties: &allow},
				"every_seconds": {Type: tools.TypeInteger, Description: "Fixed interval in seconds (minimum 60)"},
				"daily": {Type: tools.TypeObject, Description: "Daily local time", Properties: map[string]*tools.JsonSchemaNode{
					"time": {Type: tools.TypeString}, "time_zone": {Type: tools.TypeString},
				}, Required: []string{"time", "time_zone"}, AdditionalProperties: &allow},
				"weekly": {Type: tools.TypeObject, Description: "Weekly local time", Properties: map[string]*tools.JsonSchemaNode{
					"time": {Type: tools.TypeString}, "time_zone": {Type: tools.TypeString},
					"weekdays": {Type: tools.TypeArray, Items: &tools.JsonSchemaNode{Type: tools.TypeInteger}},
				}, Required: []string{"time", "time_zone", "weekdays"}, AdditionalProperties: &allow},
				"cron": {Type: tools.TypeObject, Description: "Five-field cron expression", Properties: map[string]*tools.JsonSchemaNode{
					"expression": {Type: tools.TypeString}, "time_zone": {Type: tools.TypeString},
				}, Required: []string{"expression", "time_zone"}, AdditionalProperties: &allow},
			},
			Required: []string{"title", "prompt"}, AdditionalProperties: &allow,
		},
		Execute: func(_ context.Context, input map[string]any) (any, error) {
			return s.createScheduleFromTool(sessionID, input)
		},
	}
}

func (s *Server) createScheduleFromTool(sessionID string, input map[string]any) (any, error) {
	title, _ := input["title"].(string)
	prompt, _ := input["prompt"].(string)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("schedule_create requires title and prompt")
	}
	now := time.Now()
	task := &scheduleTask{
		ID: uuid.MustV4(), Title: strings.TrimSpace(title), Prompt: prompt, Status: "active",
		Target:    map[string]any{"kind": "session", "sessionId": sessionID},
		CreatedAt: now.UTC().Format(time.RFC3339Nano), UpdatedAt: now.UTC().Format(time.RFC3339Nano),
		InvolvedSessionIDs: []string{sessionID},
	}
	timing := map[string]any{}
	for _, key := range []string{"at", "every_seconds", "daily", "weekly", "cron"} {
		if value, ok := input[key]; ok {
			timing[key] = value
		}
	}
	if len(timing) == 0 {
		return nil, fmt.Errorf("schedule_create requires one timing selector")
	}
	switch {
	case timing["at"] != nil:
		timing["kind"] = "at"
	case timing["every_seconds"] != nil:
		timing["kind"] = "every"
	case timing["daily"] != nil:
		timing["kind"] = "daily"
	case timing["weekly"] != nil:
		timing["kind"] = "weekly"
	case timing["cron"] != nil:
		timing["kind"] = "cron"
	}
	if err := applyScheduleTiming(task, timing, now); err != nil {
		return nil, err
	}
	s.schedules.mu.Lock()
	s.schedules.tasks[task.ID] = task
	_ = s.schedules.persistLocked()
	s.schedules.mu.Unlock()
	s.emitHost(map[string]any{"type": "emit", "event": "schedule/changed", "args": []any{task.ID}})
	return map[string]any{"id": task.ID, "title": task.Title, "scheduledAt": task.ScheduledAt}, nil
}
