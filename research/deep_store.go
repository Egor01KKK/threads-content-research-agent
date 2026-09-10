package research

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// startOrResumeDeepRun creates the durable deep-run envelope or reopens an
// incomplete run. Each unit of work is checkpointed independently below, so a
// process interruption does not require repeating completed profile fetches.
func (s *Store) startOrResumeDeepRun(topic string, resumeID, anchorCount int64) (int64, time.Time, bool, error) {
	if resumeID > 0 {
		var storedTopic, startedAt, deepStatus string
		err := s.db.QueryRow(`
SELECT r.topic, r.started_at, d.status
FROM research_runs r JOIN research_deep_runs d ON d.run_id = r.id
WHERE r.id = ?`, resumeID).Scan(&storedTopic, &startedAt, &deepStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, time.Time{}, false, fmt.Errorf("deep run %d was not found", resumeID)
		}
		if err != nil {
			return 0, time.Time{}, false, fmt.Errorf("load deep run %d: %w", resumeID, err)
		}
		if normalizeSpace(storedTopic) != normalizeSpace(topic) {
			return 0, time.Time{}, false, fmt.Errorf("deep run %d belongs to topic %q, not %q", resumeID, storedTopic, topic)
		}
		// A terminal run may be reopened as a deterministic re-materialization
		// (for example after changing only the export writer). Completed unit
		// checkpoints remain idempotent and are not fetched again.
		if _, err := s.db.Exec(`
UPDATE research_runs SET completed_at = NULL, status = ?, error_message = NULL WHERE id = ?;
UPDATE research_deep_runs SET status = ?, stage = 'resume', updated_at = ? WHERE run_id = ?`,
			RunStatusRunning, RunStatusRunning, formatTime(time.Now().UTC()), resumeID); err != nil {
			return 0, time.Time{}, false, fmt.Errorf("resume deep run %d: %w", resumeID, err)
		}
		return resumeID, parseStoredTime(startedAt), true, nil
	}

	startedAt := time.Now().UTC()
	runID, err := s.StartRun(topic, startedAt, int(anchorCount))
	if err != nil {
		return 0, time.Time{}, false, err
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_runs (run_id, mode, objective, anonymous, stage, status, checkpoint_json, updated_at)
VALUES (?, ?, ?, 1, 'anchors', ?, '{}', ?)`,
		runID, ModeDeep, topic, RunStatusRunning, formatTime(startedAt))
	if err != nil {
		_ = s.FinishRun(runID, time.Now().UTC(), RunStatusFailed, err.Error())
		return 0, time.Time{}, false, fmt.Errorf("create deep run envelope: %w", err)
	}
	return runID, startedAt, false, nil
}

// addOrGetQuery makes deep query checkpoints idempotent on resume.
func (s *Store) addOrGetQuery(runID int64, query string, position int, searchedAt time.Time) (int64, error) {
	_, err := s.db.Exec(`
INSERT INTO research_queries (run_id, query, position, searched_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(run_id, query) DO NOTHING`, runID, query, position, formatTime(searchedAt))
	if err != nil {
		return 0, fmt.Errorf("add deep research query %q: %w", query, err)
	}
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM research_queries WHERE run_id = ? AND query = ?`, runID, query).Scan(&id); err != nil {
		return 0, fmt.Errorf("read deep research query %q: %w", query, err)
	}
	return id, nil
}

func (s *Store) saveDeepAnchorProbe(runID int64, probe DeepAnchorProbe) error {
	payload, err := json.Marshal(probe)
	if err != nil {
		return fmt.Errorf("marshal deep anchor probe %q: %w", probe.Anchor.Query, err)
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_anchor_probes (run_id, query, wave, position, status, payload_json, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id, query, wave) DO UPDATE SET
    position = excluded.position,
    status = excluded.status,
    payload_json = excluded.payload_json,
    updated_at = excluded.updated_at`,
		runID, probe.Anchor.Query, probe.Wave, probe.Anchor.Position, probe.Status, string(payload), formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("save deep anchor probe %q: %w", probe.Anchor.Query, err)
	}
	return nil
}

func (s *Store) loadDeepAnchorProbes(runID int64) (map[string]DeepAnchorProbe, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM research_deep_anchor_probes WHERE run_id = ? ORDER BY position, query`, runID)
	if err != nil {
		return nil, fmt.Errorf("load deep anchor probes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]DeepAnchorProbe{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan deep anchor probe: %w", err)
		}
		var probe DeepAnchorProbe
		if err := json.Unmarshal([]byte(raw), &probe); err != nil {
			return nil, fmt.Errorf("decode deep anchor probe: %w", err)
		}
		out[deepRecordKey(probe.Wave, probe.Anchor.Query)] = probe
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deep anchor probes: %w", err)
	}
	return out, nil
}

func (s *Store) saveDeepProfile(runID int64, profile DeepSeedProfile) error {
	profile.Username = normalizeAuthor(profile.Username)
	if profile.Username == "" {
		return errors.New("deep profile username is required")
	}
	payload, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal deep profile @%s: %w", profile.Username, err)
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_profiles (run_id, username, status, payload_json, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(run_id, username) DO UPDATE SET
    status = excluded.status,
    payload_json = excluded.payload_json,
    updated_at = excluded.updated_at`,
		runID, profile.Username, profile.Status, string(payload), formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("save deep profile @%s: %w", profile.Username, err)
	}
	return nil
}

func (s *Store) loadDeepProfiles(runID int64) (map[string]DeepSeedProfile, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM research_deep_profiles WHERE run_id = ? ORDER BY username`, runID)
	if err != nil {
		return nil, fmt.Errorf("load deep profiles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]DeepSeedProfile{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan deep profile: %w", err)
		}
		var profile DeepSeedProfile
		if err := json.Unmarshal([]byte(raw), &profile); err != nil {
			return nil, fmt.Errorf("decode deep profile: %w", err)
		}
		out[normalizeAuthor(profile.Username)] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deep profiles: %w", err)
	}
	return out, nil
}

func (s *Store) saveDeepPost(runID int64, record DeepPostRecord) error {
	if strings.TrimSpace(record.Post.ID) == "" {
		return errors.New("deep post id is required")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal deep post %s: %w", record.Post.ID, err)
	}
	stage := ""
	if len(record.Stages) > 0 {
		stage = record.Stages[0]
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_posts (run_id, post_id, stage, payload_json, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(run_id, post_id) DO UPDATE SET
    stage = excluded.stage,
    payload_json = excluded.payload_json,
    updated_at = excluded.updated_at`,
		runID, record.Post.ID, stage, string(payload), formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("save deep post %s: %w", record.Post.ID, err)
	}
	return nil
}

func (s *Store) loadDeepPosts(runID int64) (map[string]DeepPostRecord, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM research_deep_posts WHERE run_id = ? ORDER BY post_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("load deep posts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]DeepPostRecord{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan deep post: %w", err)
		}
		var record DeepPostRecord
		if err := json.Unmarshal([]byte(raw), &record); err != nil {
			return nil, fmt.Errorf("decode deep post: %w", err)
		}
		out[record.Post.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deep posts: %w", err)
	}
	return out, nil
}

func (s *Store) saveDeepProvenance(runID int64, edge DeepProvenance) error {
	if strings.TrimSpace(edge.PostID) == "" {
		return errors.New("deep provenance post id is required")
	}
	payload, err := json.Marshal(edge)
	if err != nil {
		return fmt.Errorf("marshal provenance %s: %w", edge.PostID, err)
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_provenance (run_id, post_id, edge_key, payload_json, observed_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(run_id, post_id, edge_key) DO NOTHING`,
		runID, edge.PostID, deepProvenanceKey(edge), string(payload), formatTime(edge.ObservedAt))
	if err != nil {
		return fmt.Errorf("save provenance %s: %w", edge.PostID, err)
	}
	return nil
}

func (s *Store) loadDeepProvenance(runID int64) ([]DeepProvenance, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM research_deep_provenance WHERE run_id = ? ORDER BY observed_at, post_id, edge_key`, runID)
	if err != nil {
		return nil, fmt.Errorf("load deep provenance: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeepProvenance
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan deep provenance: %w", err)
		}
		var edge DeepProvenance
		if err := json.Unmarshal([]byte(raw), &edge); err != nil {
			return nil, fmt.Errorf("decode deep provenance: %w", err)
		}
		out = append(out, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deep provenance: %w", err)
	}
	return out, nil
}

func (s *Store) saveDeepSecondaryAnchor(runID int64, anchor DeepSecondaryAnchor) error {
	payload, err := json.Marshal(anchor)
	if err != nil {
		return fmt.Errorf("marshal secondary anchor %q: %w", anchor.Phrase, err)
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_secondary_anchors (run_id, phrase, payload_json, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(run_id, phrase) DO UPDATE SET payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		runID, anchor.Phrase, string(payload), formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("save secondary anchor %q: %w", anchor.Phrase, err)
	}
	return nil
}

func (s *Store) loadDeepSecondaryAnchors(runID int64) (map[string]DeepSecondaryAnchor, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM research_deep_secondary_anchors WHERE run_id = ? ORDER BY phrase`, runID)
	if err != nil {
		return nil, fmt.Errorf("load secondary anchors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]DeepSecondaryAnchor{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan secondary anchor: %w", err)
		}
		var anchor DeepSecondaryAnchor
		if err := json.Unmarshal([]byte(raw), &anchor); err != nil {
			return nil, fmt.Errorf("decode secondary anchor: %w", err)
		}
		out[anchor.Phrase] = anchor
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate secondary anchors: %w", err)
	}
	return out, nil
}

func (s *Store) saveDeepReply(runID int64, reply DeepReplySignal) error {
	if strings.TrimSpace(reply.ReplyID) == "" {
		return errors.New("deep reply id is required")
	}
	payload, err := json.Marshal(reply)
	if err != nil {
		return fmt.Errorf("marshal deep reply %s: %w", reply.ReplyID, err)
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_replies (run_id, reply_id, payload_json, observed_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(run_id, reply_id) DO NOTHING`,
		runID, reply.ReplyID, string(payload), formatTime(reply.ObservedAt))
	if err != nil {
		return fmt.Errorf("save deep reply %s: %w", reply.ReplyID, err)
	}
	return nil
}

func (s *Store) loadDeepReplies(runID int64) ([]DeepReplySignal, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM research_deep_replies WHERE run_id = ? ORDER BY observed_at, reply_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("load deep replies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeepReplySignal
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan deep reply: %w", err)
		}
		var reply DeepReplySignal
		if err := json.Unmarshal([]byte(raw), &reply); err != nil {
			return nil, fmt.Errorf("decode deep reply: %w", err)
		}
		out = append(out, reply)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deep replies: %w", err)
	}
	return out, nil
}

func (s *Store) saveDeepCheckpoint(runID int64, stage, key string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal deep checkpoint %s/%s: %w", stage, key, err)
	}
	_, err = s.db.Exec(`
INSERT INTO research_deep_checkpoints (run_id, stage, checkpoint_key, payload_json, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(run_id, stage, checkpoint_key) DO UPDATE SET payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		runID, stage, key, string(raw), formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("save deep checkpoint %s/%s: %w", stage, key, err)
	}
	return nil
}

func (s *Store) updateDeepRunStatus(runID int64, stage string, status RunStatus, checkpoint any) error {
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal deep run checkpoint: %w", err)
	}
	_, err = s.db.Exec(`
UPDATE research_deep_runs SET stage = ?, status = ?, checkpoint_json = ?, updated_at = ? WHERE run_id = ?`,
		stage, status, string(raw), formatTime(time.Now().UTC()), runID)
	if err != nil {
		return fmt.Errorf("update deep run status: %w", err)
	}
	return nil
}

func deepRecordKey(wave, query string) string {
	return strings.ToLower(strings.TrimSpace(wave)) + "\x00" + normalizeSpace(query)
}

func deepProvenanceKey(edge DeepProvenance) string {
	return strings.Join([]string{
		edge.SourceStage, edge.AnchorQuery, edge.AnchorCategory, edge.SeedPostID,
		edge.ProfileUsername, edge.RootPostID, edge.ExtractedPhrase, edge.SecondaryAnchor,
		fmt.Sprint(edge.Depth),
	}, "\x00")
}
