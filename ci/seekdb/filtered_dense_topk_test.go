//go:build integration

package oceanbase

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

// TestFilteredDenseTopK catches both cross-dataset leakage and applying the
// dataset filter only after global top-K truncation. It runs against a real DB;
// an absent connection configuration is a failure rather than a passing skip.
func TestFilteredDenseTopK(t *testing.T) {
	host := os.Getenv("RAGFLOW_TEST_OCEANBASE_HOST")
	if host == "" {
		t.Fatal("RAGFLOW_TEST_OCEANBASE_HOST must be set for this real-database regression")
	}
	port, err := strconv.Atoi(envOr("RAGFLOW_TEST_OCEANBASE_PORT", "2881"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(envOr("RAGFLOW_TEST_OCEANBASE_ENGINE_TYPE", "oceanbase"), config.OceanBaseConnectionConfig{
		DBName:   envOr("RAGFLOW_TEST_OCEANBASE_DBNAME", "test"),
		User:     envOr("RAGFLOW_TEST_OCEANBASE_USER", "root@test"),
		Password: os.Getenv("RAGFLOW_TEST_OCEANBASE_PASSWORD"),
		Host:     host, Port: port, MaxConnections: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	// Time only isolates the table name, never the vectors or ranking oracle.
	table := "ragflow_filtered_topk_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	const allowedKB, foreignKB = "allowed-kb", "foreign-kb"
	defer cleanupChunkStore(t, engine, table, "")
	if err := engine.CreateChunkStore(ctx, table, allowedKB, 2, "naive", ""); err != nil {
		t.Fatal(err)
	}
	// All vectors have norm 1. With query [1,0], cosine similarities are
	// foreign=1, near=.8, next=.6, tail=.28. No ties or L2 assumptions.
	// The same physical table must hold both KBs to exercise WHERE kb_id.
	rows := []map[string]interface{}{
		{"id": "allowed-tail", "kb_id": allowedKB, "doc_id": "doc-tail", "content_with_weight": "tail", "q_2_vec": []float64{0.28, 0.96}},
		{"id": "foreign-nearest", "kb_id": foreignKB, "doc_id": "doc-foreign", "content_with_weight": "foreign", "q_2_vec": []float64{1, 0}},
		{"id": "allowed-next", "kb_id": allowedKB, "doc_id": "doc-next", "content_with_weight": "next", "q_2_vec": []float64{0.6, 0.8}},
		{"id": "allowed-near", "kb_id": allowedKB, "doc_id": "doc-near", "content_with_weight": "near", "q_2_vec": []float64{0.8, 0.6}},
	}
	if _, err := engine.InsertChunks(ctx, rows, table, allowedKB, ""); err != nil {
		t.Fatal(err)
	}

	search := func(kbs []string, wantIDs, wantKBs []string) {
		t.Helper()
		result, err := engine.Search(ctx, &types.SearchRequest{
			IndexNames: []string{table}, KbIDs: kbs, Limit: 2,
			SelectFields: []string{"id", "kb_id"},
			MatchExprs: []interface{}{&types.MatchDenseExpr{
				VectorColumnName: "q_2_vec", EmbeddingData: []float64{1, 0},
				EmbeddingDataType: "float", TopN: 2,
				ExtraOptions: map[string]interface{}{"similarity": 0.1},
			}},
		})
		if err != nil {
			t.Fatalf("dense search KBs=%v: %v", kbs, err)
		}
		ids, actualKBs := make([]string, 0, len(result.Chunks)), make([]string, 0, len(result.Chunks))
		for _, chunk := range result.Chunks {
			id, idOK := chunk["id"].(string)
			kb, kbOK := chunk["kb_id"].(string)
			if !idOK || !kbOK {
				t.Fatalf("dense search returned missing/non-string identity: %#v", chunk)
			}
			ids, actualKBs = append(ids, id), append(actualKBs, kb)
		}
		if !reflect.DeepEqual(ids, wantIDs) || !reflect.DeepEqual(actualKBs, wantKBs) {
			t.Fatalf("KBs=%v got IDs=%v KBs=%v; want ordered IDs=%v KBs=%v", kbs, ids, actualKBs, wantIDs, wantKBs)
		}
	}

	// Establish that the foreign row really would displace an allowed result
	// if top-K were calculated globally before applying the dataset filter.
	search(nil, []string{"foreign-nearest", "allowed-near"}, []string{foreignKB, allowedKB})
	search([]string{allowedKB}, []string{"allowed-near", "allowed-next"}, []string{allowedKB, allowedKB})
	// Reverse isolation and a filter miss must not fall back to unfiltered hits.
	search([]string{foreignKB}, []string{"foreign-nearest"}, []string{foreignKB})
	search([]string{"absent-kb"}, []string{}, []string{})
}
