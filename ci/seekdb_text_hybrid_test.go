//go:build integration

package oceanbase

import (
	"context"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

// TestSeekDBLexicalHybridContributions checks the adapter's default SQL
// full-text-first fusion, not the optional DBMS_HYBRID_SEARCH implementation.
// Identical lexical documents make vector-driven ranking changes deterministic
// without depending on the database's absolute BM25 score or lexical tie order.
func TestSeekDBLexicalHybridContributions(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_OCEANBASE_HOST") == "" {
		t.Fatal("RAGFLOW_TEST_OCEANBASE_HOST is required for the real-database regression")
	}
	if os.Getenv("RAGFLOW_TEST_OCEANBASE_ENGINE_TYPE") != "seekdb" {
		t.Fatal("this regression requires the seekdb backend")
	}
	t.Setenv("ENABLE_FULLTEXT_SEARCH", "true")
	t.Setenv("ENABLE_HYBRID_SEARCH", "false")
	t.Setenv("SEARCH_ORIGINAL_CONTENT", "true")
	t.Setenv("USE_FULLTEXT_FIRST_FUSION_SEARCH", "true")
	port, err := strconv.Atoi(envOr("RAGFLOW_TEST_OCEANBASE_PORT", "2881"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine("seekdb", config.OceanBaseConnectionConfig{
		DBName:   envOr("RAGFLOW_TEST_OCEANBASE_DBNAME", "test"),
		User:     envOr("RAGFLOW_TEST_OCEANBASE_USER", "root"),
		Password: os.Getenv("RAGFLOW_TEST_OCEANBASE_PASSWORD"),
		Host:     os.Getenv("RAGFLOW_TEST_OCEANBASE_HOST"), Port: port, MaxConnections: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	table := "ragflow_text_hybrid_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	const kb = "hybrid-allowed"
	defer cleanupChunkStore(t, engine, table, "")
	if err := engine.CreateChunkStore(ctx, table, kb, 2, "naive", ""); err != nil {
		t.Fatal(err)
	}
	rows := []map[string]interface{}{
		{"id": "lexical-y", "kb_id": kb, "doc_id": "doc-y", "content_with_weight": "tangerine orchard", "content_ltks": "tangerine orchard", "q_2_vec": []float64{0.6, 0.8}},
		{"id": "vector-x-only", "kb_id": kb, "doc_id": "doc-vx", "content_with_weight": "granite mountain", "content_ltks": "granite mountain", "q_2_vec": []float64{1, 0}},
		{"id": "lexical-x", "kb_id": kb, "doc_id": "doc-x", "content_with_weight": "tangerine orchard", "content_ltks": "tangerine orchard", "q_2_vec": []float64{0.8, 0.6}},
		{"id": "vector-y-only", "kb_id": kb, "doc_id": "doc-vy", "content_with_weight": "granite mountain", "content_ltks": "granite mountain", "q_2_vec": []float64{0, 1}},
		{"id": "foreign-match", "kb_id": "hybrid-foreign", "doc_id": "doc-foreign", "content_with_weight": "tangerine orchard", "content_ltks": "tangerine orchard", "q_2_vec": []float64{1, 0}},
	}
	if _, err := engine.InsertChunks(ctx, rows, table, kb, ""); err != nil {
		t.Fatal(err)
	}
	searchIDs := func(expressions ...interface{}) []string {
		t.Helper()
		result, err := engine.Search(ctx, &types.SearchRequest{
			IndexNames: []string{table}, KbIDs: []string{kb}, Limit: 10,
			SelectFields: []string{"id", "kb_id"}, MatchExprs: expressions,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(result.Chunks))
		for _, chunk := range result.Chunks {
			if chunk["kb_id"] != kb {
				t.Fatalf("dataset isolation failed: %#v", chunk)
			}
			id, ok := chunk["id"].(string)
			if !ok {
				t.Fatalf("missing/non-string chunk identity: %#v", chunk)
			}
			ids = append(ids, id)
		}
		return ids
	}
	text := func(query string) *types.MatchTextExpr {
		return &types.MatchTextExpr{MatchingText: query, TopN: 10}
	}
	dense := func(vector []float64) *types.MatchDenseExpr {
		return &types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: vector,
			EmbeddingDataType: "float", TopN: 10, ExtraOptions: map[string]interface{}{"similarity": 0.1}}
	}
	fusion := &types.FusionExpr{Method: "weighted_sum", TopN: 10,
		FusionParams: map[string]interface{}{"weights": "0.5,0.5"}}
	assertIDs := func(got, want []string) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got IDs %v, want %v", got, want)
		}
	}

	// Lexical ties are compared as sets, but exact membership/count is required.
	lexicalIDs := searchIDs(text("tangerine"))
	sort.Strings(lexicalIDs)
	assertIDs(lexicalIDs, []string{"lexical-x", "lexical-y"})
	assertIDs(searchIDs(text("absentlexicaltoken")), []string{})

	// Establish that a vector-only fallback would return the distractors first.
	assertIDs(searchIDs(dense([]float64{1, 0})), []string{"vector-x-only", "lexical-x", "lexical-y"})
	assertIDs(searchIDs(dense([]float64{0, 1})), []string{"vector-y-only", "lexical-y", "lexical-x"})

	// Both matching rows have identical lexical scores and pagerank defaults.
	// Changing only the vector must invert their strict order; the text predicate
	// must still exclude both vector-perfect, nonmatching distractors.
	assertIDs(searchIDs(text("tangerine"), dense([]float64{1, 0}), fusion), []string{"lexical-x", "lexical-y"})
	assertIDs(searchIDs(text("tangerine"), dense([]float64{0, 1}), fusion), []string{"lexical-y", "lexical-x"})
	assertIDs(searchIDs(text("absentlexicaltoken"), dense([]float64{1, 0}), fusion), []string{})
}
