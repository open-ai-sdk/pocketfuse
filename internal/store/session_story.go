package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SessionStory is one session read as a continuous conversation: turns in start
// order, with the plain requests between them gathered into activity items.
type SessionStory struct {
	Session
	Title       string      `json:"title,omitempty"`
	Totals      StoryTotals `json:"totals"`
	Items       []StoryItem `json:"items"`
	HiddenNoise int         `json:"hiddenNoise"`
	Omitted     int         `json:"omittedTraces,omitempty"`
}

// StoryTotals sums the session. Usage and cost stay nil when nothing reported them.
type StoryTotals struct {
	Turns       int        `json:"turns"`
	ModelCalls  int        `json:"modelCalls"`
	ToolCalls   int        `json:"toolCalls"`
	FailedTools int        `json:"failedTools"`
	Errors      int        `json:"errors"`
	Usage       TokenUsage `json:"usage"`
	Cost        *float64   `json:"cost,omitempty"`
	StartTime   string     `json:"startTime,omitempty"`
	EndTime     string     `json:"endTime,omitempty"`
	DurationMs  float64    `json:"durationMs"`
}

// StoryItem is either a turn or a run of plain requests.
type StoryItem struct {
	Type     string         `json:"type"`
	Turn     *Turn          `json:"turn,omitempty"`
	Activity *StoryActivity `json:"activity,omitempty"`
}

// StoryActivity gathers consecutive plain requests between two turns.
type StoryActivity struct {
	StartTime  string         `json:"startTime,omitempty"`
	EndTime    string         `json:"endTime,omitempty"`
	ErrorCount int            `json:"errorCount,omitempty"`
	Requests   []StoryRequest `json:"requests"`
}

// StoryRequest is one plain trace. Unlinked marks a time match: no identifier
// connects it to this session, so another session may own it.
type StoryRequest struct {
	TraceID    string  `json:"traceId"`
	Name       string  `json:"name,omitempty"`
	StartTime  string  `json:"startTime,omitempty"`
	DurationMs float64 `json:"durationMs"`
	Status     string  `json:"status,omitempty"`
	Unlinked   bool    `json:"unlinked,omitempty"`
}

// GetSessionStory builds a session's story, or returns sql.ErrNoRows.
func (s *Store) GetSessionStory(ctx context.Context, id string, hideNoise bool, options TurnOptions) (*SessionStory, error) {
	detail, err := s.GetSessionDetail(ctx, id, hideNoise)
	if err != nil {
		return nil, err
	}
	story := &SessionStory{Session: detail.Session, Items: []StoryItem{}, HiddenNoise: detail.HiddenNoise}
	if listed := len(detail.Traces) + detail.HiddenNoise; listed < detail.TraceCount {
		story.Omitted = detail.TraceCount - listed
	}
	type entry struct {
		start   time.Time
		turn    *Turn
		request *StoryRequest
	}
	entries := make([]entry, 0, len(detail.Traces)+len(detail.Unlinked))
	for position := range detail.Traces {
		trace := &detail.Traces[position]
		observations, err := s.listTraceObservations(ctx, trace.ID)
		if err != nil {
			return nil, err
		}
		applyEffectiveTiming(observations)
		annotateObservations(observations)
		start, _ := parseTimestamp(firstNonEmpty(trace.StartTime, trace.Timestamp))
		current := entry{start: start, turn: buildTurn(trace, observations, options)}
		if current.turn == nil {
			current.request = requestFrom(trace, false)
		}
		entries = append(entries, current)
	}
	for position := range detail.Unlinked {
		trace := &detail.Unlinked[position]
		start, _ := parseTimestamp(firstNonEmpty(trace.StartTime, trace.Timestamp))
		entries = append(entries, entry{start: start, request: requestFrom(trace, true)})
	}
	sort.SliceStable(entries, func(left, right int) bool { return entries[left].start.Before(entries[right].start) })

	var previousEnd time.Time
	var previous *Turn
	jobs := map[string]*Turn{}
	var turns []*Turn
	for position := range entries {
		current := &entries[position]
		if current.turn == nil {
			story.appendRequest(current.request)
			continue
		}
		turn := current.turn
		if previous != nil && current.start.Before(previousEnd) {
			turn.Overlaps = true
		}
		if end, ok := parseTimestamp(turn.EndTime); ok && end.After(previousEnd) {
			previousEnd = end
		}
		previous = turn
		turns = append(turns, turn)
		if turn.JobID != "" {
			jobs[turn.JobID] = turn
		}
		story.Items = append(story.Items, StoryItem{Type: "turn", Turn: turn})
	}
	pairAcrossTurns(turns, options)
	linkSpawnedJobs(turns, jobs)
	for _, turn := range turns {
		story.Totals.add(turn)
	}
	if len(turns) > 0 && turns[0].UserMessage != nil {
		story.Title = clipTitle(turns[0].UserMessage.Text)
	}
	return story, nil
}

func requestFrom(trace *Trace, unlinked bool) *StoryRequest {
	return &StoryRequest{TraceID: trace.ID, Name: trace.Name, StartTime: firstNonEmpty(trace.StartTime, trace.Timestamp),
		DurationMs: trace.Duration, Status: trace.Status, Unlinked: unlinked}
}

func (story *SessionStory) appendRequest(request *StoryRequest) {
	if count := len(story.Items); count == 0 || story.Items[count-1].Type != "activity" {
		story.Items = append(story.Items, StoryItem{Type: "activity", Activity: &StoryActivity{StartTime: request.StartTime, Requests: []StoryRequest{}}})
	}
	activity := story.Items[len(story.Items)-1].Activity
	activity.Requests = append(activity.Requests, *request)
	activity.EndTime = request.StartTime
	if request.Status == "error" {
		activity.ErrorCount++
	}
}

func (totals *StoryTotals) add(turn *Turn) {
	totals.Turns++
	totals.ModelCalls += turn.ModelCalls
	totals.ToolCalls += turn.ToolCallCount
	totals.FailedTools += turn.FailedTools
	totals.Errors += turn.ErrorCount
	totals.Usage.add(turn.Usage)
	if turn.Cost != nil {
		if totals.Cost == nil {
			zero := 0.0
			totals.Cost = &zero
		}
		*totals.Cost += *turn.Cost
	}
	if totals.StartTime == "" {
		totals.StartTime = turn.StartTime
	}
	if end, ok := parseTimestamp(turn.EndTime); ok {
		if known, hasKnown := parseTimestamp(totals.EndTime); !hasKnown || end.After(known) {
			totals.EndTime = turn.EndTime
		}
	}
	totals.DurationMs = durationMillis(totals.StartTime, totals.EndTime)
}

// linkSpawnedJobs connects a job to the tool call whose result named it, which
// is recorded evidence and not a guess from timing.
func linkSpawnedJobs(turns []*Turn, jobs map[string]*Turn) {
	if len(jobs) == 0 {
		return
	}
	for _, turn := range turns {
		for stepPosition := range turn.Steps {
			step := &turn.Steps[stepPosition]
			for _, call := range step.ToolCalls {
				if call.rawResult == "" {
					continue
				}
				for jobID, job := range jobs {
					if job != turn && job.SpawnedBy == nil && strings.Contains(call.rawResult, jobID) {
						job.SpawnedBy = &TurnLink{TraceID: turn.TraceID, ObservationID: step.ObservationID, Tool: call.Name}
					}
				}
			}
		}
	}
}

func clipTitle(text string) string {
	const titleChars = 140
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > titleChars {
		return string(runes[:titleChars])
	}
	return text
}

// sessionSummaries fills the fields of a session list row a reader scans for:
// what the conversation is about and whether anything failed.
func (s *Store) sessionSummaries(ctx context.Context, sessions []Session) error {
	for position := range sessions {
		session := &sessions[position]
		var input nullableString
		err := s.db.QueryRowContext(ctx, `
			SELECT o.input FROM observations o
			WHERE o.trace_id IN (SELECT id FROM traces WHERE session_id = ?) AND o.type = 'GENERATION' AND o.input IS NOT NULL
			ORDER BY o.start_time ASC LIMIT 1`, session.ID).Scan(&input)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read session title: %w", err)
		}
		if err == nil {
			session.Title = clipTitle(parseLLMExchange(rawJSON(input), nil).UserText)
		}
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM observations
			WHERE trace_id IN (SELECT id FROM traces WHERE session_id = ?) AND `+errorLevelCondition, session.ID).Scan(&session.ErrorCount); err != nil {
			return fmt.Errorf("count session errors: %w", err)
		}
	}
	return nil
}
