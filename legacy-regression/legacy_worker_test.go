//go:build integration

package api

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// This intentionally runs in its own fresh tenant and process. SQL constructs an
// old-index/new-source fixture; it does not fake a successful parse or queue.
func TestSeekDBLegacyTableForcedReparseWorker22082(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_API_KEY") != "" || !strings.HasPrefix(os.Getenv("RAGFLOW_TEST_EMAIL"), "legacy-") {
		t.Fatal("run alone with a fresh legacy- test email and no externally supplied API key")
	}
	fixture := os.Getenv("SEEKDB_LEGACY_FIXTURE")
	if fixture == "" {
		t.Fatal("SEEKDB_LEGACY_FIXTURE is required")
	}
	dataset := createRESTDataset(t, apiObject{"parse_type": 1, "parser_id": "general"})
	kb := dataset["id"].(string)
	source := "Quasarrenewal coriander botanical archive describes the NEW source revision with fresh emerald leaves.\n"
	neighborSource := "Neighborpreserved basil botanical archive must remain unchanged throughout this experiment.\n"
	docs := uploadRESTDocuments(t, TestConfig, kb, []documentUpload{
		{Name: "rerun.txt", Content: []byte(source)}, {Name: "neighbor.txt", Content: []byte(neighborSource)},
	})
	if len(docs) != 2 {
		t.Fatalf("upload=%v", docs)
	}
	// Match by filename rather than assuming the upload response order.
	ids := map[string]string{}
	for _, v := range docs {
		d := object(t, v)
		ids[d["name"].(string)] = d["id"].(string)
	}
	target, neighbor := ids["rerun.txt"], ids["neighbor.txt"]
	if target == "" || neighbor == "" {
		t.Fatalf("missing fixture docs: %v", ids)
	}
	started := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, documentRESTPath(kb)+"/parse", apiObject{"document_ids": []string{target, neighbor}}, nil))["data"])
	if started["success_count"] != float64(2) {
		t.Fatalf("initial parse not accepted: %v", started)
	}
	waitRESTDocumentStatus(t, kb, []string{target, neighbor}, "COMPLETED")
	beforeEvent := getRESTDocument(t, kb, target)["latest_ingestion_event"]
	snapshot := func(doc string) map[string]string {
		t.Helper()
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, documentRESTPath(kb)+"/"+doc+"/chunks", nil, apiObject{"page_size": 100}))["data"])
		result := map[string]string{}
		for _, v := range array(t, data["chunks"]) {
			c := object(t, v)
			id, _ := c["id"].(string)
			content, _ := c["content_with_weight"].(string)
			if id == "" || content == "" {
				t.Fatalf("bad chunk: %v", c)
			}
			result[id] = content
		}
		if len(result) == 0 {
			t.Fatalf("document %s has no parsed chunks", doc)
		}
		return result
	}
	original := snapshot(target)
	neighborBefore := snapshot(neighbor)
	runFixture := func(action string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()
		output, err := exec.CommandContext(ctx, "bash", fixture, action, kb, target, neighbor).CombinedOutput()
		t.Logf("fixture %s: %s", action, output)
		if err != nil {
			t.Fatalf("fixture %s: %v", action, err)
		}
	}
	runFixture("downgrade")
	// Unlike /documents/parse (a no-op for COMPLETED), this invokes the service's
	// forced RerunWithDelete path. The worker must read the original uploaded file.
	restOK(t, restCall(t, TestConfig, http.MethodPost, "/datasets/"+kb+"/chunks", apiObject{"document_ids": []string{target}}, nil))
	waitRESTDocumentStatus(t, kb, []string{target}, "COMPLETED")
	after := getRESTDocument(t, kb, target)
	if reflect.DeepEqual(beforeEvent, after["latest_ingestion_event"]) {
		t.Fatal("no new ingestion event; forced parse did not execute")
	}
	fresh := snapshot(target)
	for id, content := range fresh {
		if strings.HasPrefix(id, "legacy_old_") || strings.Contains(content, "Obsoletelegacyrevision") {
			t.Fatalf("legacy chunk survived: %s %s", id, content)
		}
		if !strings.Contains(content, "Quasarrenewal") {
			t.Fatalf("worker did not restore current source: %s", content)
		}
	}
	texts := func(chunks map[string]string) []string {
		result := make([]string, 0, len(chunks))
		for _, text := range chunks {
			result = append(result, text)
		}
		slices.Sort(result)
		return result
	}
	if !slices.Equal(texts(original), texts(fresh)) {
		t.Fatalf("new worker content differs from deterministic source: initial=%v new=%v", original, fresh)
	}
	if got := snapshot(neighbor); !reflect.DeepEqual(neighborBefore, got) {
		t.Fatalf("neighbor mutated: before=%v after=%v", neighborBefore, got)
	}
	runFixture("verify")
	expected := make([]string, 0, len(fresh))
	for id := range fresh {
		expected = append(expected, id)
	}
	slices.Sort(expected)
	retrieved := waitRESTRetrievalIDs(t, apiObject{"dataset_ids": []string{kb}, "document_ids": []string{target}, "question": source, "similarity_threshold": 0, "vector_similarity_weight": 1, "top_k": 100, "page_size": 100, "include_knowledge_compilation": false}, expected)
	for _, chunk := range retrieved {
		if !strings.Contains(chunk["content"].(string), "Quasarrenewal") {
			t.Fatalf("retrieval did not return new source: %v", chunk)
		}
	}
}
