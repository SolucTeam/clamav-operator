/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
	clamavv1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testNodeScan(name, nodeName string, phase clamavv1beta1.NodeScanPhase, scanned, infected int64) *clamavv1beta1.NodeScan {
	return &clamavv1beta1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "clamav-system",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)),
		},
		Spec: clamavv1beta1.NodeScanSpec{NodeName: nodeName},
		Status: clamavv1beta1.NodeScanStatus{
			Phase:         phase,
			FilesScanned:  scanned,
			FilesInfected: infected,
			StartTime:     &metav1.Time{Time: time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC)},
		},
	}
}

func testInfectedNodeScan(name, nodeName string, files ...clamavv1beta1.InfectedFile) *clamavv1beta1.NodeScan {
	s := testNodeScan(name, nodeName, clamavv1beta1.NodeScanPhaseCompleted, 1000, int64(len(files)))
	s.Status.InfectedFiles = files
	return s
}

func newFakeServer(t *testing.T, fixedNamespace string, objs ...client.Object) *Server {
	t.Helper()
	scheme := Scheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		builder.WithObjects(o)
	}
	return &Server{client: builder.Build(), fixedNamespace: fixedNamespace}
}

func TestListScans_Cluster(t *testing.T) {
	running := &clamavv1beta1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{Name: "cs-running", Namespace: "clamav-system"},
		Status: clamavv1beta1.ClusterScanStatus{
			Phase:              clamavv1beta1.ClusterScanPhaseRunning,
			TotalNodes:         3,
			CompletedNodes:     1,
			TotalFilesScanned:  5000,
			TotalFilesInfected: 2,
		},
	}
	done := &clamavv1beta1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{Name: "cs-done", Namespace: "other-ns"},
		Status:     clamavv1beta1.ClusterScanStatus{Phase: clamavv1beta1.ClusterScanPhaseCompleted},
	}

	s := newFakeServer(t, "", running, done)
	_, out, err := s.listScans(context.Background(), nil, listScansIn{Kind: "cluster"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Count)
}

func TestListScans_PhaseAndNamespaceFilter(t *testing.T) {
	running := testNodeScan("ns-1", "node-a", clamavv1beta1.NodeScanPhaseRunning, 100, 0)
	done := testNodeScan("ns-2", "node-b", clamavv1beta1.NodeScanPhaseCompleted, 200, 1)
	done.Namespace = "other-ns"

	s := newFakeServer(t, "", running, done)
	_, out, err := s.listScans(context.Background(), nil, listScansIn{Kind: "node", Phase: "Running"})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Count)
	assert.Equal(t, "ns-1", out.Scans[0].Name)
	assert.Equal(t, "node-a", out.Scans[0].Node)

	_, out, err = s.listScans(context.Background(), nil, listScansIn{Kind: "node", Namespace: "clamav-system"})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Count)

	_, out, err = s.listScans(context.Background(), nil, listScansIn{Kind: "bogus"})
	require.Error(t, err)

	_, out, err = s.listScans(context.Background(), nil, listScansIn{Kind: "node", Phase: "Bogus"})
	require.Error(t, err)
	_ = out
}

func TestListScans_FixedNamespaceOverridesRequest(t *testing.T) {
	a := testNodeScan("in-fixed", "node-a", clamavv1beta1.NodeScanPhaseRunning, 10, 0)
	b := &clamavv1beta1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{Name: "in-other", Namespace: "other"},
		Spec:       clamavv1beta1.NodeScanSpec{NodeName: "node-b"},
		Status:     clamavv1beta1.NodeScanStatus{Phase: clamavv1beta1.NodeScanPhaseRunning},
	}

	s := newFakeServer(t, "clamav-system", a, b)
	// Requesting another namespace must NOT escape the fixed one.
	_, out, err := s.listScans(context.Background(), nil, listScansIn{Kind: "node", Namespace: "other"})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Count)
	assert.Equal(t, "in-fixed", out.Scans[0].Name)
}

func TestGetScan_NodeNotFound(t *testing.T) {
	s := newFakeServer(t, "")
	_, _, err := s.getScan(context.Background(), nil, getScanIn{Kind: "node", Namespace: "clamav-system", Name: "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_, _, err = s.getScan(context.Background(), nil, getScanIn{Name: ""})
	require.Error(t, err)
}

func TestGetScan_NodeWithFailure(t *testing.T) {
	n := testNodeScan("ns-fail", "node-x", clamavv1beta1.NodeScanPhaseFailed, 50, 0)
	n.Status.FailureReason = "OOMKilled"
	n.Status.ExitCode = 137
	s := newFakeServer(t, "clamav-system", n)

	_, out, err := s.getScan(context.Background(), nil, getScanIn{Kind: "node", Name: "ns-fail"})
	require.NoError(t, err)
	assert.Equal(t, "Failed", out.Phase)
	assert.Equal(t, "node-x", out.Node)
	assert.Equal(t, "OOMKilled", out.FailureReason)
	assert.EqualValues(t, 137, out.ExitCode)
}

func TestGetScan_ClusterWithNodeScans(t *testing.T) {
	cs := &clamavv1beta1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{Name: "cs-1", Namespace: "clamav-system"},
		Status: clamavv1beta1.ClusterScanStatus{
			Phase:      clamavv1beta1.ClusterScanPhasePartiallyComplete,
			TotalNodes: 2,
			NodeScans: []clamavv1beta1.NodeScanReference{
				{Name: "ns-a", NodeName: "node-a", Phase: clamavv1beta1.NodeScanPhaseCompleted, FilesScanned: 10, FilesInfected: 1},
				{Name: "ns-b", NodeName: "node-b", Phase: clamavv1beta1.NodeScanPhaseFailed},
			},
		},
	}
	s := newFakeServer(t, "clamav-system", cs)
	_, out, err := s.getScan(context.Background(), nil, getScanIn{Name: "cs-1"})
	require.NoError(t, err)
	assert.Equal(t, "PartiallyCompleted", out.Phase)
	require.Len(t, out.NodeScans, 2)
	assert.Equal(t, "node-a", out.NodeScans[0].Node)
	assert.EqualValues(t, 1, out.NodeScans[0].FilesInfected)
}

func TestListInfections(t *testing.T) {
	files := []clamavv1beta1.InfectedFile{
		{Path: "/tmp/evil1", Viruses: []string{"EICAR-Test-File", "Win.Test.EICAR_HDB-1"}, Size: 68},
		{Path: "/tmp/evil2", Viruses: []string{"Unix.Malware.Agent-123"}},
	}
	a := testInfectedNodeScan("scan-a", "node-a", files...)
	b := testInfectedNodeScan("scan-b", "node-b",
		clamavv1beta1.InfectedFile{Path: "/var/bad", Viruses: []string{"EICAR-Test-File"}})
	clean := testNodeScan("scan-clean", "node-c", clamavv1beta1.NodeScanPhaseCompleted, 999, 0)

	s := newFakeServer(t, "clamav-system", a, b, clean)
	_, out, err := s.listInfections(context.Background(), nil, listInfectionsIn{})
	require.NoError(t, err)
	assert.Equal(t, 3, out.Count)
	assert.Equal(t, "node-a", out.Infections[0].Node) // sorted by node

	// Filter by virus (case-insensitive substring).
	_, out, err = s.listInfections(context.Background(), nil, listInfectionsIn{Virus: "eicar"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Count)

	// Filter by node.
	_, out, err = s.listInfections(context.Background(), nil, listInfectionsIn{NodeName: "node-b"})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Count)
	assert.Equal(t, "/var/bad", out.Infections[0].Path)

	// Filter by scan.
	_, out, err = s.listInfections(context.Background(), nil, listInfectionsIn{ScanName: "scan-a"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Count)
}

func TestListInfections_TruncatedFlag(t *testing.T) {
	n := testInfectedNodeScan("scan-t", "node-a",
		clamavv1beta1.InfectedFile{Path: "/tmp/x", Viruses: []string{"V"}})
	n.Status.InfectedFilesTruncated = true
	n.Status.FilesInfected = 250

	s := newFakeServer(t, "clamav-system", n)
	_, out, err := s.listInfections(context.Background(), nil, listInfectionsIn{})
	require.NoError(t, err)
	assert.True(t, out.Truncated)
}

func TestGetClusterStatus(t *testing.T) {
	completed := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	older := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	infected := testInfectedNodeScan("scan-a", "node-a",
		clamavv1beta1.InfectedFile{Path: "/tmp/x", Viruses: []string{"V"}})
	running := testNodeScan("scan-b", "node-b", clamavv1beta1.NodeScanPhaseRunning, 100, 0)

	csDone := &clamavv1beta1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{Name: "cs-old", Namespace: "clamav-system"},
		Status: clamavv1beta1.ClusterScanStatus{
			Phase:          clamavv1beta1.ClusterScanPhaseCompleted,
			CompletionTime: &metav1.Time{Time: older},
		},
	}
	csRunning := &clamavv1beta1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{Name: "cs-live", Namespace: "clamav-system"},
		Status: clamavv1beta1.ClusterScanStatus{
			Phase:          clamavv1beta1.ClusterScanPhaseRunning,
			CompletionTime: &metav1.Time{Time: completed},
		},
	}
	sched := &clamavv1beta1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "daily", Namespace: "clamav-system"},
		Spec:       clamavv1beta1.ScanScheduleSpec{Schedule: "0 2 * * *"},
	}
	suspended := &clamavv1beta1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "paused", Namespace: "clamav-system"},
		Spec:       clamavv1beta1.ScanScheduleSpec{Schedule: "0 3 * * *", Suspend: true},
	}

	s := newFakeServer(t, "clamav-system", infected, running, csDone, csRunning, sched, suspended)
	_, out, err := s.getClusterStatus(context.Background(), nil, getClusterStatusIn{})
	require.NoError(t, err)

	assert.Equal(t, 2, out.TotalNodeScans)
	assert.Equal(t, 2, out.TotalClusterScans)
	assert.Equal(t, []string{"cs-live"}, out.RunningClusterScans)
	assert.Equal(t, 1, out.ActiveSchedules)
	assert.Equal(t, 1, out.SuspendedSchedules)
	assert.Equal(t, 1, out.InfectedNodes)
	assert.Equal(t, []string{"node-a"}, out.InfectedNodesList)
	require.NotNil(t, out.LastScheduleTime)
	assert.Equal(t, "2026-03-01T12:00:00Z", *out.LastScheduleTime)
}

func TestListSchedules(t *testing.T) {
	sched := &clamavv1beta1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "daily", Namespace: "clamav-system"},
		Spec: clamavv1beta1.ScanScheduleSpec{
			Schedule:          "0 2 * * *",
			ConcurrencyPolicy: "Forbid",
			ClusterScan:       clamavv1beta1.ClusterScanSpec{ScanPolicy: "default"},
		},
		Status: clamavv1beta1.ScanScheduleStatus{
			NextScheduleTime: &metav1.Time{Time: time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC)},
		},
	}
	s := newFakeServer(t, "clamav-system", sched)
	_, out, err := s.listSchedules(context.Background(), nil, listSchedulesIn{})
	require.NoError(t, err)
	require.Len(t, out.Schedules, 1)
	got := out.Schedules[0]
	assert.Equal(t, "0 2 * * *", got.Cron)
	assert.False(t, got.Suspended)
	assert.Equal(t, "default", got.ScanPolicy)
	assert.Equal(t, "2026-04-01T02:00:00Z", *got.NextScheduleTime)
}

func TestListPolicies(t *testing.T) {
	policy := &clamavv1alpha1.ScanPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "full", Namespace: "clamav-system"},
		Spec: clamavv1alpha1.ScanPolicySpec{
			Paths:           []string{"/etc", "/usr"},
			ExcludePatterns: []string{`\.log$`},
			MaxConcurrent:   8,
			MaxFileSize:     52428800,
			Quarantine:      &clamavv1alpha1.QuarantineConfig{Enabled: true, Action: "move", QuarantineDir: "/quarantine"},
			Notifications: &clamavv1alpha1.NotificationConfig{
				Slack:   &clamavv1alpha1.SlackConfig{Enabled: true},
				Webhook: &clamavv1alpha1.WebhookConfig{Enabled: true},
				Email:   &clamavv1alpha1.EmailConfig{Enabled: false},
			},
		},
		Status: clamavv1alpha1.ScanPolicyStatus{UsageCount: 42},
	}
	s := newFakeServer(t, "clamav-system", policy)
	_, out, err := s.listPolicies(context.Background(), nil, listPoliciesIn{})
	require.NoError(t, err)
	require.Len(t, out.Policies, 1)
	got := out.Policies[0]
	assert.Equal(t, []string{"/etc", "/usr"}, got.Paths)
	assert.EqualValues(t, 8, got.MaxConcurrent)
	assert.InDelta(t, 50, got.MaxFileSizeMB, 0.01)
	assert.Equal(t, "move (dir: /quarantine)", got.Quarantine)
	assert.Equal(t, []string{"slack", "webhook"}, got.Notifications)
	assert.EqualValues(t, 42, got.UsageCount)
}

func TestHelpers(t *testing.T) {
	// timeStr
	assert.Nil(t, timeStr(nil))
	assert.Nil(t, timeStr(&metav1.Time{}))
	ts := timeStr(&metav1.Time{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	require.NotNil(t, ts)
	assert.Equal(t, "2026-01-02T03:04:05Z", *ts)

	// humanizeDuration
	assert.Equal(t, "45s", humanizeDuration(45*time.Second))
	assert.Equal(t, "5m0s", humanizeDuration(5*time.Minute))
	assert.Equal(t, "2h", humanizeDuration(2*time.Hour))
	assert.Equal(t, "2h30m", humanizeDuration(2*time.Hour+30*time.Minute))
	assert.Equal(t, "3d4h", humanizeDuration(76*time.Hour))

	// validPhase
	assert.True(t, validPhase("Running"))
	assert.False(t, validPhase("running"))
	assert.False(t, validPhase(""))

	// matchesVirus
	assert.True(t, matchesVirus([]string{"Win.Test.EICAR_HDB-1"}, "eicar"))
	assert.False(t, matchesVirus([]string{"Foo"}, "eicar"))
	assert.True(t, matchesVirus(nil, ""))

	// toPhaseCounts ordering
	counts := toPhaseCounts(map[string]int{"Failed": 1, "Pending": 2, "Running": 3, "Completed": 4})
	var phases []string
	for _, pc := range counts {
		phases = append(phases, pc.Phase)
	}
	assert.Equal(t, []string{"Pending", "Running", "Completed", "Failed"}, phases)
}

func TestResolveNamespace(t *testing.T) {
	s := &Server{}
	assert.Equal(t, "req", s.resolveNamespace("req"))
	assert.Equal(t, "", s.resolveNamespace(""))

	sFixed := &Server{fixedNamespace: "fixed"}
	assert.Equal(t, "fixed", sFixed.resolveNamespace("req"))
	assert.Equal(t, "fixed", sFixed.resolveNamespace(""))
}

func TestTTLPointerAvailability(t *testing.T) {
	// Keeps the k8s.io/utils dependency import honest for ptr usage in tests.
	assert.NotNil(t, ptr.To("x"))
}
