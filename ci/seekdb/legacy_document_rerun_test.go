//go:build integration

package document

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"ragflow/internal/common"
	"ragflow/internal/engine/oceanbase"
	"ragflow/internal/entity"
	"ragflow/internal/server/config"
)

// TestLegacyTableReparseDelete22082 exercises the real service entry and SQL
// cleanup on the pre-#22121 table layout. Metadata uses the existing SQLite
// fixture and the queue uses recordingTaskPublisher; no worker is run.
func TestLegacyTableReparseDelete22082(t *testing.T) {
	host := os.Getenv("RAGFLOW_TEST_OCEANBASE_HOST")
	if host == "" {
		t.Fatal("RAGFLOW_TEST_OCEANBASE_HOST is required for integration tests")
	}
	env := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	port, err := strconv.Atoi(env("RAGFLOW_TEST_OCEANBASE_PORT", "2881"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.OceanBaseConnectionConfig{Host: host, Port: port,
		DBName:   env("RAGFLOW_TEST_OCEANBASE_DBNAME", "test"),
		User:     env("RAGFLOW_TEST_OCEANBASE_USER", "root@test"),
		Password: os.Getenv("RAGFLOW_TEST_OCEANBASE_PASSWORD"), MaxConnections: 4}
	eng, err := oceanbase.NewEngine(env("RAGFLOW_TEST_OCEANBASE_ENGINE_TYPE", "oceanbase"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	mc := mysql.NewConfig()
	mc.Net, mc.Addr, mc.User, mc.Passwd, mc.DBName = "tcp", fmt.Sprintf("%s:%d", host, port), cfg.User, cfg.Password, cfg.DBName
	mc.ParseTime = true
	raw, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	tenant := "legacy_ci_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	table := "ragflow_" + tenant
	// Fixed historical columns, independent of the implementation under test.
	// Source: #22121 first parent 03b0c0ae04ab30e44796b927a3c0229b1a440358.
	ddl := "CREATE TABLE `" + table + "` (" + legacy22082Columns + ")"
	if _, err := raw.ExecContext(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		if _, err := raw.ExecContext(cleanup, "DROP TABLE IF EXISTS `"+table+"`"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	// Add the engine's required indices without replacing the historical table.
	if err := eng.CreateChunkStore(ctx, table, "kb-1", 2, "naive", ""); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME IN ('compile_kwd','compilation_template_kind_kwd')", cfg.DBName, table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("fixture is not a legacy table: optional columns are present")
	}
	for _, row := range []struct{ id, doc, content string }{{"old-1", "doc-1", "old source one"}, {"old-2", "doc-1", "old source two"}, {"neighbor", "doc-2", "untouched neighbor"}} {
		// Empty compiler metadata is intentional: source chunks need no knowledge-
		// compilation deletion event. The service must still project missing columns.
		if _, err := raw.ExecContext(ctx, "INSERT INTO `"+table+"` (id,kb_id,doc_id,content_with_weight,extra) VALUES (?,?,?,?,?)", row.id, "kb-1", row.doc, row.content, `{"compile_kwd":"","compilation_template_kind_kwd":"","custom_payload":{"legacy":true}}`); err != nil {
			t.Fatal(err)
		}
	}
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", tenant, 2, 30, 3)
	insertTestDoc(t, "doc-1", "kb-1", 20, 2)
	insertTestDoc(t, "doc-2", "kb-1", 10, 1)
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("location", "loc-1").Error; err != nil {
		t.Fatal(err)
	}
	publisher := &recordingTaskPublisher{}
	svc := testDocumentService(t)
	svc.docEngine = eng
	svc.ingestionTaskSvc.SetTaskPublisher(publisher)
	svc.ingestionTaskSvc.SetTierResolver(func(context.Context, string) (common.Tier, error) { return common.TierDefault, nil })
	ctx = documentTestActorContext(ctx, "user-1", tenant)
	doc, err := svc.documentDAO.GetByID(ctx, db, "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	kb, err := svc.kbDAO.GetByID(ctx, db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.StartParseDocuments(ctx, doc, kb, StartParseOptions{RerunWithDelete: true}); err != nil {
		t.Fatalf("legacy reparse delete: %v", err)
	}
	if len(publisher.messages) != 1 || publisher.messages[0].TaskID == "" {
		t.Fatalf("expected exactly one enqueued task: %#v", publisher.messages)
	}
	task, err := svc.ingestionTaskDAO.GetByID(ctx, db, publisher.messages[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.DocumentID != "doc-1" || task.DatasetID != "kb-1" || task.Status != common.SCHEDULED {
		t.Fatalf("wrong enqueued task: %#v", task)
	}
	if err := raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"` WHERE doc_id = ?", "doc-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("old document still has %d chunks", count)
	}
	var content string
	if err := raw.QueryRowContext(ctx, "SELECT content_with_weight FROM `"+table+"` WHERE id = ? AND kb_id = ? AND doc_id = ?", "neighbor", "kb-1", "doc-2").Scan(&content); err != nil {
		t.Fatalf("neighbor missing: %v", err)
	}
	if content != "untouched neighbor" {
		t.Fatalf("neighbor changed: %q", content)
	}
}

const legacy22082Columns = "`id` VARCHAR(256) NOT NULL PRIMARY KEY,`kb_id` VARCHAR(256) NOT NULL,`doc_id` VARCHAR(256) NULL,`docnm_kwd` VARCHAR(256) NULL,`doc_type_kwd` VARCHAR(256) NULL,`title_tks` VARCHAR(256) NULL,`title_sm_tks` VARCHAR(256) NULL,`content_with_weight` LONGTEXT NULL,`content_ltks` LONGTEXT NULL,`content_sm_ltks` LONGTEXT NULL,`pagerank_fea` INTEGER NULL,`important_kwd` ARRAY(VARCHAR(256)) NULL,`important_tks` TEXT NULL,`question_kwd` ARRAY(VARCHAR(1024)) NULL,`question_tks` TEXT NULL,`tag_kwd` ARRAY(VARCHAR(256)) NULL,`tag_feas` JSON NULL,`available_int` INTEGER NOT NULL DEFAULT 1,`create_time` VARCHAR(19) NULL,`create_timestamp_flt` DOUBLE NULL,`img_id` VARCHAR(128) NULL,`position_int` ARRAY(ARRAY(INTEGER)) NULL,`page_num_int` ARRAY(INTEGER) NULL,`top_int` ARRAY(INTEGER) NULL,`knowledge_graph_kwd` VARCHAR(256) NULL,`type_kwd` VARCHAR(256) NULL,`source_id` ARRAY(VARCHAR(256)) NULL,`entity_kwd` VARCHAR(256) NULL,`entity_type_kwd` VARCHAR(256) NULL,`from_entity_kwd` VARCHAR(256) NULL,`to_entity_kwd` VARCHAR(256) NULL,`weight_int` INTEGER NULL,`weight_flt` DOUBLE NULL,`entities_kwd` ARRAY(VARCHAR(256)) NULL,`rank_flt` DOUBLE NULL,`n_hop_with_weight` LONGTEXT NULL,`removed_kwd` VARCHAR(256) NULL DEFAULT 'N',`raptor_kwd` VARCHAR(256) NULL,`raptor_layer_int` INTEGER NULL,`chunk_data` JSON NULL,`metadata` JSON NULL,`extra` JSON NULL,`_order_id` INTEGER NULL,`group_id` VARCHAR(256) NULL,`mom_id` VARCHAR(256) NULL"
