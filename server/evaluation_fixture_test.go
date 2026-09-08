package main

import (
	"context"
	"conway/server/db"
	"conway/server/planning"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type evaluationFixture struct{ Plan, Reference, Initial, Training, Later string }

// Shared by real HTTP browser acceptance and API access-boundary tests.
func seedEvaluationFixture(database *db.DB, pool *pgxpool.Pool, owner string) evaluationFixture {
	ctx := context.Background()
	encode := func(v any) []byte { b, err := json.Marshal(v); Expect(err).NotTo(HaveOccurred()); return b }
	now := time.Now().UTC().Truncate(24 * time.Hour)
	f := evaluationFixture{Plan: newID(), Reference: "evaluation-training", Initial: newID(), Training: newID(), Later: newID()}
	plan := db.PlanRow{ID: f.Plan, Owner: owner, Name: "Atlas model evaluation", HorizonWeeks: 12, CreatedAt: now.Unix(), UpdatedAt: now.Unix()}
	Expect(database.CreatePlan(plan)).To(Succeed())
	DeferCleanup(func() { Expect(database.DeletePlan(f.Plan)).To(Succeed()) })
	sourceID := newID()
	_, err := pool.Exec(ctx, `INSERT INTO evidence_sources(id,owner,config,credential,next_at) VALUES($1,$2,'{}','',0)`, sourceID, owner)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		_, err := pool.Exec(ctx, `DELETE FROM evidence_sources WHERE id=$1`, sourceID)
		Expect(err).NotTo(HaveOccurred())
	})
	initial := []planning.ExecutionIssue{}
	completed := []db.IssueRow{}
	predictions := []planning.ForecastPrediction{}
	for i, name := range []string{"Beacon", "Atlas release"} {
		key := []string{"PROJ-101", "PROJ-201"}[i]
		issued := now.AddDate(0, 0, -90+i*60)
		finish := issued.AddDate(0, 0, 14)
		it := planning.Initiative{Name: name, EpicKeys: []string{key}, KitPct: 1, Work: map[string]planning.TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}}
		inputs := planning.NewBaselineInputs([]planning.Team{{Name: "Atlas", Tracks: 1}}, []planning.Initiative{it}, planning.Params{HorizonWeeks: 12}, planning.SchedulingParams{PeriodStart: issued.Format("2006-01-02"), WipModel: "strict", EstimateModel: "effort"})
		forecast, err := planning.ComputeForecast(inputs, planning.ForecastSettings{LowerFactor: .8, UpperFactor: 1.3, Disruption: .1})
		Expect(err).NotTo(HaveOccurred())
		scope := []planning.ExecutionIssue{{Key: key, Type: "Epic", Pod: "Atlas", StatusCategory: "indeterminate"}, {Key: key + "-child", ParentKey: key, Type: "Story", Pod: "Atlas", StatusCategory: "indeterminate"}}
		initial = append(initial, scope...)
		completed = append(completed, db.IssueRow{Key: key, IssueType: "Epic", Pod: "Atlas", StatusCat: "indeterminate"}, db.IssueRow{Key: key + "-child", ParentKey: key, IssueType: "Story", Pod: "Atlas", StatusCat: "done", Resolved: &finish})
		predictions = append(predictions, planning.ForecastPrediction{ID: []string{f.Reference, "evaluation-test"}[i], Name: name + " prediction", IssuedAt: issued.Unix(), Inputs: inputs, Forecast: forecast, Issues: scope})
	}
	for i, id := range []string{f.Initial, f.Training, f.Later} {
		at := now.AddDate(0, 0, []int{-100, -60, -1}[i]).Unix()
		rows := []db.IssueRow{}
		switch i {
		case 0:
			for _, r := range initial {
				rows = append(rows, db.IssueRow{Key: r.Key, ParentKey: r.ParentKey, IssueType: r.Type, Pod: r.Pod, StatusCat: r.StatusCategory})
			}
		case 1:
			rows = completed[:2]
		default:
			rows = completed
		}
		Expect(database.CreateSnapshotWithData(db.SnapshotRow{ID: id, Owner: owner, Name: []string{"Initial scope", "Earlier training evidence", "Later test evidence"}[i], Source: "jira", CreatedAt: at + 60}, db.SnapshotData{Issues: rows, Pods: []db.PodRow{{Name: "Atlas", Streams: 1}}})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(id)).To(Succeed()) })
		_, err = pool.Exec(ctx, `INSERT INTO evidence_runs(id,source_id,status,started_at,finished_at,snapshot_id,config,version) VALUES($1,$2,'succeeded',$3,$4,$5,'{}',1)`, newID(), sourceID, at, at+60, id)
		Expect(err).NotTo(HaveOccurred())
	}
	source, err := database.PredictionSnapshotSource(ctx, f.Initial)
	Expect(err).NotTo(HaveOccurred())
	// The current plan intentionally differs; model evaluation uses archived inputs.
	Expect(database.SavePlanTeams(f.Plan, encode(predictions[1].Inputs.Teams), plan.UpdatedAt)).To(Succeed())
	Expect(database.SavePlanInitiatives(f.Plan, encode(predictions[1].Inputs.Initiatives), plan.UpdatedAt)).To(Succeed())
	Expect(database.SavePlanScheduling(f.Plan, encode(predictions[1].Inputs.Scheduling), plan.UpdatedAt)).To(Succeed())
	saved, err := database.GetPlan(f.Plan)
	Expect(err).NotTo(HaveOccurred())
	for _, p := range predictions {
		p.Evidence = planning.PredictionEvidence{SnapshotID: f.Initial, SourceID: sourceID, ConfigFingerprint: source.Fingerprint, StartedAt: now.AddDate(0, 0, -100).Unix(), CapturedAt: now.AddDate(0, 0, -100).Unix() + 60}
		inserted, err := database.SavePrediction(ctx, saved, db.PredictionRow{ID: p.ID, Name: p.Name, IssuedAt: p.IssuedAt, SnapshotID: f.Initial, RequestHash: p.ID, Data: encode(p)})
		Expect(err).NotTo(HaveOccurred())
		Expect(inserted).To(BeTrue())
	}
	return f
}
