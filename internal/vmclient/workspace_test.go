package vmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceFiltersEveryReadDeleteAndWrite(t *testing.T) {
	for _, namespace := range []string{"", "alice"} {
		t.Run("namespace="+namespace, func(t *testing.T) {
			writes, reads := 0, 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/import" {
					var m Metric
					if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
						t.Error(err)
					}
					if m.Metric[workspaceLabel] != namespace {
						t.Error("agent-supplied workspace was not overwritten")
					}
					writes++
					w.WriteHeader(204)
					return
				}
				want := `{yuanlook_workspace="` + namespace + `"}`
				if r.URL.Query().Get("extra_filters[]") != want {
					t.Errorf("%s: missing workspace scope", r.URL.Path)
				}
				reads++
				if strings.Contains(r.URL.Path, "/label/") {
					w.Write([]byte(`{"status":"success","data":[]}`))
				} else {
					w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
				}
			}))
			defer s.Close()
			c := NewVMClient(s.URL, time.Second, time.Second)
			c.SetWorkspace(namespace)
			ctx := context.Background()
			labels := map[string]string{"__name__": "test", workspaceLabel: "forged"}
			if err := c.Write(ctx, []Metric{{Metric: labels, Values: []float64{1}, Timestamps: []int64{time.Now().Add(-time.Minute).UnixMilli()}}}); err != nil {
				t.Fatal(err)
			}
			if labels[workspaceLabel] != "forged" {
				t.Fatal("mutated caller labels")
			}
			if _, err := c.Query(ctx, "sum(test)"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.QueryRange(ctx, "test", time.Now().Add(-time.Minute), time.Now(), 0); err != nil {
				t.Fatal(err)
			}
			if _, err := c.GetLabelValues(ctx, "agent_id", nil); err != nil {
				t.Fatal(err)
			}
			if err := c.DeleteSeries(ctx, []string{`{agent_id="same"}`}); err != nil {
				t.Fatal(err)
			}
			if writes != 1 || reads != 4 {
				t.Fatalf("incomplete coverage %d/%d", writes, reads)
			}
		})
	}
}

// Run only against a DISPOSABLE VictoriaMetrics instance. It creates and deletes
// a dedicated test metric to verify server-side semantics, not just query strings.
func TestWorkspaceIsolationWithVictoriaMetrics(t *testing.T) {
	endpoint := os.Getenv("YUANLOOK_TEST_VM_URL")
	if endpoint == "" {
		t.Skip("set YUANLOOK_TEST_VM_URL to a disposable VM")
	}
	admin := NewVMClient(endpoint, time.Second*10, time.Second*10)
	admin.SetWorkspace("")
	alice := NewVMClient(endpoint, time.Second*10, time.Second*10)
	alice.SetWorkspace("alice-test")
	ctx := context.Background()
	metric := "yuanlook_workspace_isolation_test"
	for i, c := range []*VMClient{admin, alice} {
		err := c.Write(ctx, []Metric{{Metric: map[string]string{"__name__": metric, "agent_id": "same", workspaceLabel: "forged"}, Values: []float64{float64(i + 1)}, Timestamps: []int64{time.Now().Add(-time.Minute).UnixMilli()}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []*VMClient{admin, alice} {
		deadline := time.Now().Add(45 * time.Second)
		for {
			r, err := c.Query(ctx, metric)
			if err == nil && len(r.Data.Result) == 1 && r.Data.Result[0].Metric[workspaceLabel] == *c.workspace {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("scoped query failed: %+v %v", r, err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	// Deleting Alice's series with the same agent ID must retain the old series.
	if err := alice.DeleteSeries(ctx, []string{`{__name__="` + metric + `",agent_id="same"}`}); err != nil {
		t.Fatal(err)
	}
	ids, err := admin.GetLabelValues(ctx, "agent_id", []string{metric})
	if err != nil || len(ids) != 1 || ids[0] != "same" {
		t.Fatalf("legacy metric deleted: %v %v", ids, err)
	}
	ids, err = alice.GetLabelValues(ctx, "agent_id", []string{metric})
	if err != nil || len(ids) != 0 {
		t.Fatalf("alice metric survived delete: %v %v", ids, err)
	}
	if err := admin.DeleteSeries(ctx, []string{metric}); err != nil {
		t.Fatal(err)
	}
}
